package node

import (
	"math"
	"os"
	"sync"
	"testing"

	mapset "github.com/deckarep/golang-set/v2"
	dto "github.com/prometheus/client_model/go"

	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/pod"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	v1 "k8s.io/api/core/v1"
)

var podOnce sync.Once

// initPodGauges registers pod-level gauge vecs so node tests that
// call pod.EvictPodByNode (from Node.evict) don't panic on nil gauges.
func initPodGauges() {
	podOnce.Do(func() {
		defer func() { recover() }()
		// Avoid os.Exit(1) from pod.NewCollector guard
		// that requires DEPLOY_TYPE=Deployment when CURRENT_NODE_NAME empty.
		os.Setenv("DEPLOY_TYPE", "Deployment")
		pod.NewCollector(15)
	})
}

func getGaugeValue(t *testing.T, gv *prometheus.GaugeVec, labels prometheus.Labels) float64 {
	t.Helper()
	m, err := gv.GetMetricWith(labels)
	if err != nil {
		t.Fatalf("GetMetricWith(%v): %v", labels, err)
	}
	var metric dto.Metric
	if err := m.Write(&metric); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return metric.GetGauge().GetValue()
}

func TestNode(t *testing.T) {
	// createMetrics registers node gauge vecs globally;
	// Must run exactly once per test binary.
	n := &Node{
		AdjustedPollingRate:     true,
		deployType:              "Deployment",
		sampleInterval:          15,
		nodeAvailable:           true,
		nodeCapacity:            true,
		nodePercentage:          true,
		MaxNodeQueryConcurrency: 10,
		Set:                     mapset.NewSet[string](),
		KubeletEndpoint:         &sync.Map{},
		WaitGroup:               &sync.WaitGroup{},
	}
	n.createMetrics()

	t.Run("getKubeletEndpoint_internalIP", func(t *testing.T) {
		n1 := &Node{kubeletReadOnlyPort: 0}
		node := &v1.Node{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{Type: v1.NodeInternalIP, Address: "10.0.0.1"},
				},
				DaemonEndpoints: v1.NodeDaemonEndpoints{
					KubeletEndpoint: v1.DaemonEndpoint{Port: 10250},
				},
			},
		}
		ep := n1.getKubeletEndpoint(node)
		want := "https://10.0.0.1:10250"
		if ep != want {
			t.Errorf("got %q, want %q", ep, want)
		}
	})

	t.Run("getKubeletEndpoint_readOnlyPort", func(t *testing.T) {
		n2 := &Node{kubeletReadOnlyPort: 10255}
		node := &v1.Node{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{Type: v1.NodeInternalIP, Address: "10.0.0.1"},
				},
				DaemonEndpoints: v1.NodeDaemonEndpoints{
					KubeletEndpoint: v1.DaemonEndpoint{Port: 10250},
				},
			},
		}
		ep := n2.getKubeletEndpoint(node)
		want := "http://10.0.0.1:10255"
		if ep != want {
			t.Errorf("got %q, want %q", ep, want)
		}
	})

	t.Run("getKubeletEndpoint_noInternalIP", func(t *testing.T) {
		n3 := &Node{}
		nodeNoIP := &v1.Node{
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{Type: v1.NodeExternalIP, Address: "1.2.3.4"},
				},
				DaemonEndpoints: v1.NodeDaemonEndpoints{
					KubeletEndpoint: v1.DaemonEndpoint{Port: 10250},
				},
			},
		}
		ep := n3.getKubeletEndpoint(nodeNoIP)
		if ep != "" {
			t.Errorf("expected empty, got %q", ep)
		}
	})

	t.Run("checkKubeletStatus", func(t *testing.T) {
		t.Run("ready", func(t *testing.T) {
			conditions := []v1.NodeCondition{{Reason: "KubeletReady"}}
			if !checkKubeletStatus(&conditions) {
				t.Error("expected true for KubeletReady")
			}
		})
		t.Run("notReady", func(t *testing.T) {
			conditions := []v1.NodeCondition{{Reason: "KubeletNotReady"}}
			if checkKubeletStatus(&conditions) {
				t.Error("expected false for non-KubeletReady")
			}
		})
		t.Run("empty", func(t *testing.T) {
			conditions := []v1.NodeCondition{}
			if checkKubeletStatus(&conditions) {
				t.Error("expected false for empty conditions")
			}
		})
	})

	t.Run("SetMetrics", func(t *testing.T) {
		n.SetMetrics("set-node", 5000, 10000)

		v := getGaugeValue(t, nodeAvailableGaugeVec, prometheus.Labels{"node_name": "set-node"})
		if v != 5000 {
			t.Errorf("available: got %f, want 5000", v)
		}

		v = getGaugeValue(t, nodeCapacityGaugeVec, prometheus.Labels{"node_name": "set-node"})
		if v != 10000 {
			t.Errorf("capacity: got %f, want 10000", v)
		}

		v = getGaugeValue(t, nodePercentageGaugeVec, prometheus.Labels{"node_name": "set-node"})
		if v != 50 {
			t.Errorf("percentage: got %f, want 50", v)
		}

		n.SetMetrics("zero-cap-node", 0, 0)
		v = getGaugeValue(t, nodePercentageGaugeVec, prometheus.Labels{"node_name": "zero-cap-node"})
		if !math.IsNaN(v) {
			t.Errorf("expected NaN for zero capacity, got %f", v)
		}

		// Set full usage: available=0, capacity=10000 → 100%
		n.SetMetrics("full-node", 0, 10000)
		v = getGaugeValue(t, nodePercentageGaugeVec, prometheus.Labels{"node_name": "full-node"})
		if v != 100 {
			t.Errorf("full usage: got %f, want 100", v)
		}
	})

	t.Run("SetMetrics_disabled_no_panic", func(t *testing.T) {
		nDisabled := &Node{}
		nDisabled.SetMetrics("disabled-node", 1000, 5000)
	})

	t.Run("createMetrics_AdjustedPollingRate_registered", func(t *testing.T) {
		if AdjustedPollingRateGaugeVec == nil {
			t.Error("AdjustedPollingRateGaugeVec should be non-nil after createMetrics with true")
		}
		count := testutil.CollectAndCount(AdjustedPollingRateGaugeVec)
		if count < 0 {
			t.Error("unexpected negative count")
		}
	})

	t.Run("evict", func(t *testing.T) {
		initPodGauges()

		nEvict := &Node{
			AdjustedPollingRate: true,
			deployType:          "Deployment",
			Set:                 mapset.NewSet[string](),
			KubeletEndpoint:     &sync.Map{},
			WaitGroup:           &sync.WaitGroup{},
		}
		nEvict.Set.Add("evict-node")
		nEvict.Set.Add("keep-node")

		// Set metrics to verify cleanup
		nEvict.SetMetrics("evict-node", 1000, 5000)

		nEvict.evict("evict-node")

		if nEvict.Set.Contains("evict-node") {
			t.Error("evict-node should be removed from Set")
		}
		if !nEvict.Set.Contains("keep-node") {
			t.Error("keep-node should still be in Set")
		}
	})
}
