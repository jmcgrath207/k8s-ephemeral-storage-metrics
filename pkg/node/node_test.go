package node

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/pod"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

var (
	nodeSetupOnce sync.Once
	testNode      Node
)

func setupNodeEnv() {
	nodeSetupOnce.Do(func() {
		os.Setenv("ADJUSTED_POLLING_RATE", "true")
		os.Setenv("EPHEMERAL_STORAGE_NODE_AVAILABLE", "true")
		os.Setenv("EPHEMERAL_STORAGE_NODE_CAPACITY", "true")
		os.Setenv("EPHEMERAL_STORAGE_NODE_PERCENTAGE", "true")
		os.Setenv("CURRENT_NODE_NAME", "test-node")
		testNode = NewCollector(15)
		// Initialize pod metrics so node.evict doesn't nil-pointer
		//pod.NewCollector(15)
	})
}

func TestNewCollector_Defaults(t *testing.T) {
	setupNodeEnv()
	if !testNode.AdjustedPollingRate {
		t.Error("expected AdjustedPollingRate to be true")
	}
	if testNode.deployType != "DaemonSet" {
		t.Errorf("expected deployType 'DaemonSet', got %q", testNode.deployType)
	}
	if testNode.MaxNodeQueryConcurrency != 10 {
		t.Errorf("expected MaxNodeQueryConcurrency 10, got %d", testNode.MaxNodeQueryConcurrency)
	}
	if !testNode.nodeAvailable {
		t.Error("expected nodeAvailable to be true")
	}
	if !testNode.nodeCapacity {
		t.Error("expected nodeCapacity to be true")
	}
	if !testNode.nodePercentage {
		t.Error("expected nodePercentage to be true")
	}
	if testNode.Set == nil {
		t.Error("expected Set to be initialized")
	}
	if !testNode.Set.Contains("test-node") {
		t.Error("expected Set to contain test-node")
	}
}

func TestNewCollector_DaemonSet_NoMetrics(t *testing.T) {
	os.Setenv("ADJUSTED_POLLING_RATE", "false")
	os.Setenv("EPHEMERAL_STORAGE_NODE_AVAILABLE", "false")
	os.Setenv("EPHEMERAL_STORAGE_NODE_CAPACITY", "false")
	os.Setenv("EPHEMERAL_STORAGE_NODE_PERCENTAGE", "false")
	os.Setenv("CURRENT_NODE_NAME", "minimal-node")
	defer func() {
		os.Setenv("ADJUSTED_POLLING_RATE", "true")
		os.Setenv("EPHEMERAL_STORAGE_NODE_AVAILABLE", "true")
		os.Setenv("EPHEMERAL_STORAGE_NODE_CAPACITY", "true")
		os.Setenv("EPHEMERAL_STORAGE_NODE_PERCENTAGE", "true")
		os.Setenv("CURRENT_NODE_NAME", "test-node")
	}()

	n := NewCollector(15)
	if n.AdjustedPollingRate {
		t.Error("expected AdjustedPollingRate to be false")
	}
	if n.nodeAvailable {
		t.Error("expected nodeAvailable to be false")
	}
	if n.nodeCapacity {
		t.Error("expected nodeCapacity to be false")
	}
	if n.nodePercentage {
		t.Error("expected nodePercentage to be false")
	}
	if !n.Set.Contains("minimal-node") {
		t.Error("expected Set to contain minimal-node")
	}
}

func TestNewCollector_ScrapeFromKubelet(t *testing.T) {
	os.Setenv("SCRAPE_FROM_KUBELET", "true")
	os.Setenv("KUBELET_READONLY_PORT", "10255")
	defer func() {
		os.Setenv("SCRAPE_FROM_KUBELET", "false")
		os.Unsetenv("KUBELET_READONLY_PORT")
	}()

	n := NewCollector(15)
	if !n.scrapeFromKubelet {
		t.Error("expected scrapeFromKubelet to be true")
	}
	if n.kubeletReadOnlyPort != 10255 {
		t.Errorf("expected kubeletReadOnlyPort 10255, got %d", n.kubeletReadOnlyPort)
	}
}

func TestNewCollector_GcEnabled(t *testing.T) {
	os.Setenv("GC_ENABLED", "true")
	defer os.Setenv("GC_ENABLED", "false")

	// ponytail: starts gcMetrics goroutine with 5-minute ticker, leaks but no CPU burn
	n := NewCollector(15)
	if n.Set == nil {
		t.Error("expected Set to be initialized")
	}
}

func TestGetKubeletEndpoint_ReadOnlyPort(t *testing.T) {
	n := &Node{kubeletReadOnlyPort: 10255}
	node := &v1.Node{
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.0.0.1"},
			},
		},
	}
	ep := n.getKubeletEndpoint(node)
	if ep != "http://10.0.0.1:10255" {
		t.Errorf("expected 'http://10.0.0.1:10255', got %q", ep)
	}
}

func TestGetKubeletEndpoint_DefaultPort(t *testing.T) {
	n := &Node{kubeletReadOnlyPort: 0}
	node := &v1.Node{
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "192.168.1.1"},
			},
			DaemonEndpoints: v1.NodeDaemonEndpoints{
				KubeletEndpoint: v1.DaemonEndpoint{Port: 10250},
			},
		},
	}
	ep := n.getKubeletEndpoint(node)
	if ep != "https://192.168.1.1:10250" {
		t.Errorf("expected 'https://192.168.1.1:10250', got %q", ep)
	}
}

func TestGetKubeletEndpoint_NoInternalIP(t *testing.T) {
	n := &Node{}
	node := &v1.Node{
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: "1.2.3.4"},
			},
		},
	}
	ep := n.getKubeletEndpoint(node)
	if ep != "" {
		t.Errorf("expected empty string, got %q", ep)
	}
}

func TestCheckKubeletStatus_Ready(t *testing.T) {
	conditions := &[]v1.NodeCondition{
		{Reason: "KubeletReady", Status: v1.ConditionTrue},
	}
	if !checkKubeletStatus(conditions) {
		t.Error("expected true for KubeletReady condition")
	}
}

func TestCheckKubeletStatus_NotReady(t *testing.T) {
	conditions := &[]v1.NodeCondition{
		{Reason: "SomeOtherReason", Status: v1.ConditionFalse},
	}
	if checkKubeletStatus(conditions) {
		t.Error("expected false when no KubeletReady condition")
	}
}

func TestCheckKubeletStatus_Empty(t *testing.T) {
	conditions := &[]v1.NodeCondition{}
	if checkKubeletStatus(conditions) {
		t.Error("expected false for empty conditions")
	}
}

func TestSetMetrics_Available(t *testing.T) {
	setupNodeEnv()
	nodeAvailableGaugeVec.Reset()
	testNode.SetMetrics("node1", 1000, 2000)

	m := &dto.Metric{}
	err := nodeAvailableGaugeVec.With(prometheus.Labels{"node_name": "node1"}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Gauge.GetValue() != 1000 {
		t.Errorf("expected node available 1000, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_Capacity(t *testing.T) {
	setupNodeEnv()
	nodeCapacityGaugeVec.Reset()
	testNode.SetMetrics("node2", 500, 1000)

	m := &dto.Metric{}
	err := nodeCapacityGaugeVec.With(prometheus.Labels{"node_name": "node2"}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Gauge.GetValue() != 1000 {
		t.Errorf("expected node capacity 1000, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_Percentage(t *testing.T) {
	setupNodeEnv()
	nodePercentageGaugeVec.Reset()
	testNode.SetMetrics("node3", 500, 1000)

	m := &dto.Metric{}
	err := nodePercentageGaugeVec.With(prometheus.Labels{"node_name": "node3"}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	// available=500, capacity=1000 → used=500 → 50%
	if m.Gauge.GetValue() != 50.0 {
		t.Errorf("expected node percentage 50, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_Percentage_ZeroCapacity(t *testing.T) {
	setupNodeEnv()
	nodePercentageGaugeVec.Reset()
	testNode.SetMetrics("node4", 500, 0)

	m := &dto.Metric{}
	err := nodePercentageGaugeVec.With(prometheus.Labels{"node_name": "node4"}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if !isNaN(m.Gauge.GetValue()) {
		t.Errorf("expected NaN for zero capacity, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_Percentage_AvailableExceedsCapacity(t *testing.T) {
	setupNodeEnv()
	nodePercentageGaugeVec.Reset()
	testNode.SetMetrics("node5", 2000, 1000)
	// available > capacity → used = max(1000-2000, 0) = 0 → 0%
	m := &dto.Metric{}
	err := nodePercentageGaugeVec.With(prometheus.Labels{"node_name": "node5"}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Gauge.GetValue() != 0.0 {
		t.Errorf("expected node percentage 0, got %f", m.Gauge.GetValue())
	}
}

func TestAdjustedPollingRate_Set(t *testing.T) {
	setupNodeEnv()
	AdjustedPollingRateGaugeVec.Reset()
	AdjustedPollingRateGaugeVec.With(prometheus.Labels{"node_name": "n1"}).Set(1234.0)

	m := &dto.Metric{}
	err := AdjustedPollingRateGaugeVec.With(prometheus.Labels{"node_name": "n1"}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Gauge.GetValue() != 1234.0 {
		t.Errorf("expected 1234, got %f", m.Gauge.GetValue())
	}
}

func TestEvict(t *testing.T) {
	setupNodeEnv()
	// Init pod metrics so EvictPodByNode doesn't nil-pointer
	pod.NewCollector(15)

	n := NewCollector(15)
	n.Set.Add("node-to-evict")
	n.SetMetrics("node-to-evict", 100, 200)
	nodeAvailableGaugeVec.With(prometheus.Labels{"node_name": "node-to-evict"}).Set(100)

	n.evict("node-to-evict")
	if n.Set.Contains("node-to-evict") {
		t.Error("expected node-to-evict to be removed from set")
	}
}

func TestQuery_FakeClient_FailsTypeAssertion(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient
	defer func() { dev.Clientset = origClient }()

	n := &Node{
		scrapeFromKubelet: false,
		deployType:        "Deployment",
		sampleInterval:    1,
		Set:               mapset.NewSet[string](),
	}

	_, err := n.Query("test-node")
	if err == nil {
		t.Error("expected error from type assertion failure with fake client")
	}
}

func TestQuery_KubeletEndpointNotFound(t *testing.T) {
	n := &Node{
		scrapeFromKubelet: true,
		deployType:        "Deployment",
		sampleInterval:    1,
		KubeletEndpoint:   &sync.Map{},
		Set:               mapset.NewSet[string](),
	}

	_, err := n.Query("test-node")
	if err == nil {
		t.Error("expected error when kubelet endpoint not found")
	}
}

func TestQuery_NonDeployment_UsesApiProxy(t *testing.T) {
	// When deployType != "Deployment", uses k8s API proxy path (same type assertion issue)
	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient
	defer func() { dev.Clientset = origClient }()

	n := &Node{
		scrapeFromKubelet: true,
		deployType:        "DaemonSet",
		sampleInterval:    1,
		Set:               mapset.NewSet[string](),
	}

	_, err := n.Query("test-node")
	if err == nil {
		t.Error("expected error from API proxy path with fake client")
	}
}

func TestRunGC_EvictsDeletedNode(t *testing.T) {
	setupNodeEnv()
	pod.NewCollector(15) // init pod metrics

	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient
	defer func() { dev.Clientset = origClient }()

	n := &Node{
		AdjustedPollingRate: false,
		nodeAvailable:       false,
		nodeCapacity:        false,
		nodePercentage:      false,
		Set:                 mapset.NewSet[string](),
		KubeletEndpoint:     &sync.Map{},
	}
	// Add a node that no longer exists in the cluster
	n.Set.Add("deleted-node")

	n.runGC(500)

	if n.Set.Contains("deleted-node") {
		t.Error("expected deleted-node to be evicted after GC")
	}
}

func TestRunGC_KeepsExistingNode(t *testing.T) {
	setupNodeEnv()

	fakeClient := fake.NewSimpleClientset(
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "existing-node"}},
	)
	origClient := dev.Clientset
	dev.Clientset = fakeClient
	defer func() { dev.Clientset = origClient }()

	n := &Node{
		AdjustedPollingRate: false,
		nodeAvailable:       false,
		nodeCapacity:        false,
		nodePercentage:      false,
		Set:                 mapset.NewSet[string](),
		KubeletEndpoint:     &sync.Map{},
	}
	n.Set.Add("existing-node")

	n.runGC(500)

	if !n.Set.Contains("existing-node") {
		t.Error("expected existing-node to remain after GC")
	}
}

func TestQuery_ApiProxy_Success(t *testing.T) {
	// ponytail: httptest server to test Query happy path
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"node":{"nodeName":"test-node"},"pods":[]}`))
	}))
	defer server.Close()

	config := &rest.Config{
		Host:    server.URL,
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &schema.GroupVersion{Group: "", Version: "v1"},
			NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		},
	}
	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		t.Fatal(err)
	}
	clientset := kubernetes.New(restClient)

	origClient := dev.Clientset
	dev.Clientset = clientset
	defer func() { dev.Clientset = origClient }()

	n := &Node{
		scrapeFromKubelet: false,
		deployType:        "Deployment",
		sampleInterval:    15,
		Set:               mapset.NewSet[string](),
	}

	content, err := n.Query("test-node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(content) == 0 {
		t.Error("expected non-empty response")
	}
}

func TestQuery_ScrapeFromKubelet_ReadOnlyPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"node":{"nodeName":"kubelet-node"},"pods":[]}`))
	}))
	defer server.Close()

	origAno := dev.ClientAno
	dev.ClientAno = server.Client()
	defer func() { dev.ClientAno = origAno }()

	n := &Node{
		scrapeFromKubelet:   true,
		deployType:          "Deployment",
		kubeletReadOnlyPort: 10255,
		sampleInterval:      15,
		KubeletEndpoint:     &sync.Map{},
		Set:                 mapset.NewSet[string](),
	}
	n.KubeletEndpoint.Store("kubelet-node", server.URL)

	content, err := n.Query("kubelet-node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(content) == 0 {
		t.Error("expected non-empty response")
	}
}

func TestQuery_ScrapeFromKubelet_DefaultPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"node":{"nodeName":"kubelet-node2"},"pods":[]}`))
	}))
	defer server.Close()

	origRaw := dev.ClientRaw
	dev.ClientRaw = server.Client()
	defer func() { dev.ClientRaw = origRaw }()

	n := &Node{
		scrapeFromKubelet:   true,
		deployType:          "Deployment",
		kubeletReadOnlyPort: 0,
		sampleInterval:      15,
		KubeletEndpoint:     &sync.Map{},
		Set:                 mapset.NewSet[string](),
	}
	n.KubeletEndpoint.Store("kubelet-node2", server.URL)

	content, err := n.Query("kubelet-node2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(content) == 0 {
		t.Error("expected non-empty response")
	}
}

func TestWatch_AddNode_Informers(t *testing.T) {
	// ponytail: test Watch with fake client + informers. Goroutine leak accepted.
	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient

	n := &Node{
		deployType:          "Deployment",
		sampleInterval:      1,
		scrapeFromKubelet:   false,
		Set:                 mapset.NewSet[string](),
		KubeletEndpoint:     &sync.Map{},
		WaitGroup:           &waitGroup,
		nodeAvailable:       false,
		nodeCapacity:        false,
		nodePercentage:      false,
		AdjustedPollingRate: false,
	}

	// Watch runs forever; test only the initial add path
	go n.Watch()

	// Create a ready node
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "watch-test-node"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{Reason: "KubeletReady", Status: v1.ConditionTrue},
			},
		},
	}
	_, err := fakeClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for informer to pick up the add
	for i := 0; i < 20; i++ {
		if n.Set.Contains("watch-test-node") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !n.Set.Contains("watch-test-node") {
		t.Error("expected watch-test-node to be added to Set")
	}

	// ponytail: goroutine leak from Watch. Restore client; process exit cleans up.
	dev.Clientset = origClient
}

func TestWatch_DeleteNode(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient

	// Need pod metrics for evict
	pod.NewCollector(15)

	n := &Node{
		deployType:          "Deployment",
		sampleInterval:      1,
		scrapeFromKubelet:   false,
		Set:                 mapset.NewSet[string](),
		KubeletEndpoint:     &sync.Map{},
		WaitGroup:           &waitGroup,
	}
	n.Set.Add("delete-me")

	go n.Watch()

	// Create and immediately delete a node
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "delete-me"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{Reason: "KubeletReady", Status: v1.ConditionTrue},
			},
		},
	}
	created, _ := fakeClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	time.Sleep(100 * time.Millisecond)

	fakeClient.CoreV1().Nodes().Delete(context.Background(), created.Name, metav1.DeleteOptions{})

	// Wait for informer to process deletion
	for i := 0; i < 20; i++ {
		if !n.Set.Contains("delete-me") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if n.Set.Contains("delete-me") {
		t.Error("expected delete-me to be removed from Set after DeleteFunc")
	}

	dev.Clientset = origClient
}

func TestWatch_UpdateNode(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient

	n := &Node{
		deployType:          "Deployment",
		sampleInterval:      1,
		scrapeFromKubelet:   false,
		Set:                 mapset.NewSet[string](),
		KubeletEndpoint:     &sync.Map{},
		WaitGroup:           &waitGroup,
	}

	go n.Watch()

	// Create node that is not ready
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "update-me"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{Reason: "SomeReason", Status: v1.ConditionFalse},
			},
		},
	}
	created, _ := fakeClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	time.Sleep(100 * time.Millisecond)

	// Should NOT be added since not ready
	if n.Set.Contains("update-me") {
		t.Error("expected update-me NOT in set when not ready")
	}

	// Update to ready
	created.Status.Conditions = []v1.NodeCondition{
		{Reason: "KubeletReady", Status: v1.ConditionTrue},
	}
	fakeClient.CoreV1().Nodes().Update(context.Background(), created, metav1.UpdateOptions{})

	for i := 0; i < 20; i++ {
		if n.Set.Contains("update-me") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !n.Set.Contains("update-me") {
		t.Error("expected update-me to be added after UpdateFunc with ready status")
	}

	dev.Clientset = origClient
}

func TestRunGC_WithScrapeFromKubelet(t *testing.T) {
	setupNodeEnv()
	pod.NewCollector(15)

	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient
	defer func() { dev.Clientset = origClient }()

	n := &Node{
		scrapeFromKubelet: true,
		Set:               mapset.NewSet[string](),
		KubeletEndpoint:   &sync.Map{},
	}
	// Add node that doesn't exist, with kubelet endpoint
	n.Set.Add("gone-node")
	n.KubeletEndpoint.Store("gone-node", "http://10.0.0.1:10255")

	n.runGC(500)

	if n.Set.Contains("gone-node") {
		t.Error("expected gone-node to be evicted")
	}
	if _, ok := n.KubeletEndpoint.Load("gone-node"); ok {
		t.Error("expected kubelet endpoint to be deleted for gone-node")
	}
}

func TestQuery_ScrapeFromKubelet_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`error`))
	}))
	defer server.Close()

	origAno := dev.ClientAno
	dev.ClientAno = server.Client()
	defer func() { dev.ClientAno = origAno }()

	n := &Node{
		scrapeFromKubelet:   true,
		deployType:          "Deployment",
		kubeletReadOnlyPort: 10255,
		sampleInterval:      1,
		KubeletEndpoint:     &sync.Map{},
		Set:                 mapset.NewSet[string](),
	}
	n.KubeletEndpoint.Store("bad-node", server.URL)

	_, err := n.Query("bad-node")
	if err == nil {
		t.Error("expected error for non-200 response")
	}
}

func TestQuery_ApiProxy_Retry(t *testing.T) {
	// ponytail: test backoff retry by having first attempt fail, second succeed
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"node":{"nodeName":"retry-node"},"pods":[]}`))
	}))
	defer server.Close()

	config := &rest.Config{
		Host:    server.URL,
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &schema.GroupVersion{Group: "", Version: "v1"},
			NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		},
	}
	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		t.Fatal(err)
	}
	clientset := kubernetes.New(restClient)

	origClient := dev.Clientset
	dev.Clientset = clientset
	defer func() { dev.Clientset = origClient }()

	n := &Node{
		scrapeFromKubelet: false,
		deployType:        "Deployment",
		sampleInterval:    15,
		Set:               mapset.NewSet[string](),
	}

	content, err := n.Query("retry-node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(content) == 0 {
		t.Error("expected non-empty response")
	}
	if callCount < 2 {
		t.Error("expected at least 2 calls (retry)")
	}
}

func TestWatch_AddNode_WithScrapeFromKubelet(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient

	n := &Node{
		deployType:          "Deployment",
		sampleInterval:      1,
		scrapeFromKubelet:   true,
		kubeletReadOnlyPort: 10255,
		Set:                 mapset.NewSet[string](),
		KubeletEndpoint:     &sync.Map{},
		WaitGroup:           &waitGroup,
	}

	go n.Watch()

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "kubelet-node"},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.0.0.5"},
			},
			Conditions: []v1.NodeCondition{
				{Reason: "KubeletReady", Status: v1.ConditionTrue},
			},
		},
	}
	fakeClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})

	for i := 0; i < 20; i++ {
		if n.Set.Contains("kubelet-node") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !n.Set.Contains("kubelet-node") {
		t.Error("expected kubelet-node to be added to Set")
	}
	if ep, ok := n.KubeletEndpoint.Load("kubelet-node"); !ok || ep == "" {
		t.Error("expected kubelet endpoint to be stored")
	}

	dev.Clientset = origClient
}

func TestWatch_UpdateNode_WithScrapeFromKubelet(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient

	n := &Node{
		deployType:          "Deployment",
		sampleInterval:      1,
		scrapeFromKubelet:   true,
		kubeletReadOnlyPort: 10255,
		Set:                 mapset.NewSet[string](),
		KubeletEndpoint:     &sync.Map{},
		WaitGroup:           &waitGroup,
	}

	go n.Watch()

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "update-kubelet"},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.0.0.6"},
			},
			Conditions: []v1.NodeCondition{
				{Reason: "NotReady", Status: v1.ConditionFalse},
			},
		},
	}
	created, _ := fakeClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	time.Sleep(100 * time.Millisecond)

	if n.Set.Contains("update-kubelet") {
		t.Error("expected update-kubelet NOT in set when not ready")
	}

	created.Status.Conditions = []v1.NodeCondition{
		{Reason: "KubeletReady", Status: v1.ConditionTrue},
	}
	fakeClient.CoreV1().Nodes().Update(context.Background(), created, metav1.UpdateOptions{})

	for i := 0; i < 20; i++ {
		if n.Set.Contains("update-kubelet") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !n.Set.Contains("update-kubelet") {
		t.Error("expected update-kubelet to be added after UpdateFunc")
	}
	if ep, ok := n.KubeletEndpoint.Load("update-kubelet"); !ok || ep == "" {
		t.Error("expected kubelet endpoint to be stored after update")
	}

	dev.Clientset = origClient
}

func TestQuery_Failure_EvictsNode(t *testing.T) {
	setupNodeEnv()
	pod.NewCollector(15)

	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient
	defer func() { dev.Clientset = origClient }()

	n := &Node{
		scrapeFromKubelet: false,
		deployType:        "Deployment",
		sampleInterval:    1,
		Set:               mapset.NewSet[string](),
	}
	// Pre-add node that will be evicted on Query failure
	n.Set.Add("failing-node")

	_, err := n.Query("failing-node")
	if err == nil {
		t.Error("expected error")
	}
	if n.Set.Contains("failing-node") {
		t.Error("expected failing-node to be evicted after Query error")
	}
}

func TestWatch_DeleteNode_WithScrapeFromKubelet(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient

	pod.NewCollector(15)

	n := &Node{
		deployType:          "Deployment",
		sampleInterval:      1,
		scrapeFromKubelet:   true,
		kubeletReadOnlyPort: 10255,
		Set:                 mapset.NewSet[string](),
		KubeletEndpoint:     &sync.Map{},
		WaitGroup:           &waitGroup,
	}
	n.Set.Add("delete-kubelet")
	n.KubeletEndpoint.Store("delete-kubelet", "http://10.0.0.99:10255")

	go n.Watch()

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "delete-kubelet"},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.0.0.99"},
			},
			Conditions: []v1.NodeCondition{
				{Reason: "KubeletReady", Status: v1.ConditionTrue},
			},
		},
	}
	created, _ := fakeClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	time.Sleep(100 * time.Millisecond)
	fakeClient.CoreV1().Nodes().Delete(context.Background(), created.Name, metav1.DeleteOptions{})

	for i := 0; i < 20; i++ {
		if !n.Set.Contains("delete-kubelet") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if n.Set.Contains("delete-kubelet") {
		t.Error("expected delete-kubelet to be removed from Set")
	}
	if _, ok := n.KubeletEndpoint.Load("delete-kubelet"); ok {
		t.Error("expected kubelet endpoint to be deleted for delete-kubelet")
	}

	dev.Clientset = origClient
}

func isNaN(v float64) bool {
	return v != v
}
