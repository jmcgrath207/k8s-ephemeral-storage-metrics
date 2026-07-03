package pod

import (
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var podSetupOnce sync.Once

func setupPodMetrics() {
	podSetupOnce.Do(func() {
		// Need fake k8s client since container limits trigger goroutines
		fakeClient := fake.NewSimpleClientset()
		origClient := dev.Clientset
		dev.Clientset = fakeClient

		os.Setenv("EPHEMERAL_STORAGE_POD_USAGE", "true")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_USAGE", "true")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LIMIT_PERCENTAGE", "true")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_LIMITS_PERCENTAGE", "true")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_ROOTFS_USAGE", "true")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LOGS_USAGE", "true")
		os.Setenv("EPHEMERAL_STORAGE_INODES", "true")
		c := NewCollector(15)

		// Wait for initGetPodsData to finish
		c.WaitGroup.Wait()
		// Give podWatch a moment to capture dev.Clientset
		time.Sleep(50 * time.Millisecond)
		// ponytail: podWatch goroutine leaks; process exit cleans up
		dev.Clientset = origClient
		// Restore env to avoid goroutine triggers in subsequent tests
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LIMIT_PERCENTAGE", "false")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_LIMITS_PERCENTAGE", "false")
	})
}

func TestNewCollector_Defaults(t *testing.T) {
	// Ensure all env vars are false - container limits trigger goroutines
	os.Setenv("EPHEMERAL_STORAGE_POD_USAGE", "false")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_USAGE", "false")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LIMIT_PERCENTAGE", "false")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_LIMITS_PERCENTAGE", "false")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_ROOTFS_USAGE", "false")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LOGS_USAGE", "false")
	os.Setenv("EPHEMERAL_STORAGE_INODES", "false")
	os.Setenv("GC_ENABLED", "false")

	c := NewCollector(15)
	if c.podUsage {
		t.Error("expected podUsage false")
	}
	if c.containerVolumeUsage {
		t.Error("expected containerVolumeUsage false")
	}
	if c.containerLimitsPercentage {
		t.Error("expected containerLimitsPercentage false")
	}
	if c.containerVolumeLimitsPercentage {
		t.Error("expected containerVolumeLimitsPercentage false")
	}
}

func TestNewCollector_AllEnabled(t *testing.T) {
	os.Setenv("EPHEMERAL_STORAGE_POD_USAGE", "true")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_USAGE", "true")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LIMIT_PERCENTAGE", "true")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_LIMITS_PERCENTAGE", "true")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_ROOTFS_USAGE", "true")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LOGS_USAGE", "true")
	os.Setenv("EPHEMERAL_STORAGE_INODES", "true")
	os.Setenv("GC_ENABLED", "false")
	defer func() {
		os.Setenv("EPHEMERAL_STORAGE_POD_USAGE", "false")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_USAGE", "false")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LIMIT_PERCENTAGE", "false")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_LIMITS_PERCENTAGE", "false")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_ROOTFS_USAGE", "false")
		os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LOGS_USAGE", "false")
		os.Setenv("EPHEMERAL_STORAGE_INODES", "false")
	}()

	// Need fake k8s client for initGetPodsData goroutine
	fakeClient := fake.NewSimpleClientset()
	origClient := dev.Clientset
	dev.Clientset = fakeClient

	c := NewCollector(15)
	if !c.podUsage {
		t.Error("expected podUsage true")
	}
	if !c.containerVolumeUsage {
		t.Error("expected containerVolumeUsage true")
	}
	if !c.containerLimitsPercentage {
		t.Error("expected containerLimitsPercentage true")
	}
	if !c.containerVolumeLimitsPercentage {
		t.Error("expected containerVolumeLimitsPercentage true")
	}
	if !c.containerRootfsUsage {
		t.Error("expected containerRootfsUsage true")
	}
	if !c.containerLogsUsage {
		t.Error("expected containerLogsUsage true")
	}
	if !c.inodes {
		t.Error("expected inodes true")
	}

	// Wait for initGetPodsData goroutine to finish before restoring client
	// initGetPodsData calls WaitGroup.Done(), so Wait() blocks until it's done
	c.WaitGroup.Wait()
	// Give podWatch a moment to capture dev.Clientset for its informer
	time.Sleep(50 * time.Millisecond)
	// ponytail: goroutine leak from podWatch accepted; process exit cleans up
	dev.Clientset = origClient
}

func TestNewCollector_GcEnabled(t *testing.T) {
	// Explicitly disable container limits to avoid goroutine triggers
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_LIMIT_PERCENTAGE", "false")
	os.Setenv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_LIMITS_PERCENTAGE", "false")
	os.Setenv("GC_ENABLED", "true")
	defer os.Setenv("GC_ENABLED", "false")

	c := NewCollector(15)
	if c.lookup == nil {
		t.Error("expected lookup to be initialized")
	}
}

func TestGetContainerData_NoLimits_NoVolumes(t *testing.T) {
	cr := Collector{
		containerVolumeUsage:            false,
		containerLimitsPercentage:       false,
		containerVolumeLimitsPercentage: false,
	}
	container := v1.Container{
		Name: "test-container",
	}
	pod := v1.Pod{}

	result := cr.getContainerData(container, pod)
	if result.name != "test-container" {
		t.Errorf("expected name 'test-container', got %q", result.name)
	}
	if result.limit != 0 {
		t.Error("expected limit 0")
	}
	if result.emptyDirVolumes != nil {
		t.Error("expected emptyDirVolumes nil")
	}
}

func TestGetContainerData_WithLimit(t *testing.T) {
	cr := Collector{
		containerLimitsPercentage: true,
	}
	qty := resource.MustParse("100Mi")
	container := v1.Container{
		Name: "limited-container",
		Resources: v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceEphemeralStorage: qty,
			},
		},
	}
	pod := v1.Pod{}

	result := cr.getContainerData(container, pod)
	if result.limit <= 0 {
		t.Error("expected limit > 0")
	}
}

func TestGetContainerData_WithMultipleLimits(t *testing.T) {
	cr := Collector{
		containerLimitsPercentage: true,
	}
	ephQty := resource.MustParse("200Mi")
	cpuQty := resource.MustParse("100m")
	container := v1.Container{
		Name: "multi-limit-container",
		Resources: v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:              cpuQty,
				v1.ResourceEphemeralStorage: ephQty,
			},
		},
	}
	pod := v1.Pod{}

	result := cr.getContainerData(container, pod)
	if result.limit <= 0 {
		t.Error("expected limit > 0 when ephemeral-storage is not first")
	}
}

func TestGetContainerData_WithEmptyDirVolumes(t *testing.T) {
	cr := Collector{
		containerVolumeUsage:            true,
		containerVolumeLimitsPercentage: true,
	}
	sizeLimit := resource.MustParse("50Mi")
	container := v1.Container{
		Name: "vol-container",
		VolumeMounts: []v1.VolumeMount{
			{Name: "cache-vol", MountPath: "/cache"},
		},
	}
	pod := v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "cache-vol",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{
							SizeLimit: &sizeLimit,
						},
					},
				},
			},
		},
	}

	result := cr.getContainerData(container, pod)
	if result.emptyDirVolumes == nil {
		t.Fatal("expected emptyDirVolumes to be set")
	}
	if len(result.emptyDirVolumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(result.emptyDirVolumes))
	}
	if result.emptyDirVolumes[0].name != "cache-vol" {
		t.Errorf("expected volume name 'cache-vol', got %q", result.emptyDirVolumes[0].name)
	}
	if result.emptyDirVolumes[0].mountPath != "/cache" {
		t.Errorf("expected mountPath '/cache', got %q", result.emptyDirVolumes[0].mountPath)
	}
	if result.emptyDirVolumes[0].sizeLimit <= 0 {
		t.Error("expected sizeLimit > 0")
	}
}

func TestGetContainerData_EmptyDirWithoutLimit(t *testing.T) {
	cr := Collector{
		containerVolumeUsage: true,
	}
	container := v1.Container{
		Name: "vol-nolimit",
		VolumeMounts: []v1.VolumeMount{
			{Name: "data-vol", MountPath: "/data"},
		},
	}
	pod := v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "data-vol",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	result := cr.getContainerData(container, pod)
	if len(result.emptyDirVolumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(result.emptyDirVolumes))
	}
	if result.emptyDirVolumes[0].sizeLimit != 0 {
		t.Error("expected sizeLimit 0 for EmptyDir without SizeLimit")
	}
}

func TestGetContainerData_MultipleVolumes(t *testing.T) {
	cr := Collector{
		containerVolumeUsage: true,
	}
	container := v1.Container{
		Name: "multi-vol",
		VolumeMounts: []v1.VolumeMount{
			{Name: "vol-a", MountPath: "/a"},
			{Name: "vol-b", MountPath: "/b"},
		},
	}
	pod := v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "vol-a",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "vol-b",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	result := cr.getContainerData(container, pod)
	if len(result.emptyDirVolumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(result.emptyDirVolumes))
	}
}

func TestGetContainerData_NonEmptyDirVolume(t *testing.T) {
	cr := Collector{
		containerVolumeUsage: true,
	}
	container := v1.Container{
		Name: "skip-vol",
		VolumeMounts: []v1.VolumeMount{
			{Name: "host-vol", MountPath: "/host"},
		},
	}
	pod := v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "host-vol",
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{Path: "/tmp"},
					},
				},
			},
		},
	}

	result := cr.getContainerData(container, pod)
	if len(result.emptyDirVolumes) != 0 {
		t.Error("expected no emptyDirVolumes for non-EmptyDir volume")
	}
}

func TestGetContainerData_VolumeMountNotMatching(t *testing.T) {
	cr := Collector{
		containerVolumeUsage: true,
	}
	container := v1.Container{
		Name: "mismatch",
		VolumeMounts: []v1.VolumeMount{
			{Name: "not-in-pod", MountPath: "/other"},
		},
	}
	pod := v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "different-vol",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	result := cr.getContainerData(container, pod)
	// Volume mount references a volume not in pod spec
	// podMountsMap won't have "not-in-pod", so volume mount is skipped
	if len(result.emptyDirVolumes) != 0 {
		t.Error("expected no emptyDirVolumes when mount doesn't match pod volume")
	}
}

func TestGetPodData_Running(t *testing.T) {
	cr := Collector{
		containerLimitsPercentage: false,
		containerVolumeUsage:      false,
		lookup:                    &map[string]pod{},
		lookupMutex:               &sync.RWMutex{},
	}

	p := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "running-pod"},
		Status:     v1.PodStatus{Phase: v1.PodRunning},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{Name: "c1"},
			},
		},
	}

	cr.getPodData(p)

	cr.lookupMutex.RLock()
	_, ok := (*cr.lookup)["running-pod"]
	cr.lookupMutex.RUnlock()
	if !ok {
		t.Error("expected running-pod in lookup")
	}
}

func TestGetPodData_NotRunning(t *testing.T) {
	cr := Collector{
		containerLimitsPercentage: false,
		lookup:                    &map[string]pod{},
		lookupMutex:               &sync.RWMutex{},
	}

	p := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pending-pod"},
		Status:     v1.PodStatus{Phase: v1.PodPending},
	}

	cr.getPodData(p)

	cr.lookupMutex.RLock()
	_, ok := (*cr.lookup)["pending-pod"]
	cr.lookupMutex.RUnlock()
	if ok {
		t.Error("expected pending-pod not in lookup")
	}
}

func TestInitGetPodsData(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "ns1"},
			Status:     v1.PodStatus{Phase: v1.PodRunning},
			Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "c1"}},
			},
		},
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "ns1"},
			Status:     v1.PodStatus{Phase: v1.PodRunning},
			Spec: v1.PodSpec{
				Containers: []v1.Container{{Name: "c2"}},
			},
		},
	)
	origClient := dev.Clientset
	dev.Clientset = fakeClient
	defer func() { dev.Clientset = origClient }()

	lookup := make(map[string]pod)
	cr := Collector{
		containerLimitsPercentage: false,
		lookup:                    &lookup,
		lookupMutex:               &sync.RWMutex{},
		WaitGroup:                 &sync.WaitGroup{},
	}
	cr.WaitGroup.Add(1)
	go cr.initGetPodsData()
	cr.WaitGroup.Wait()

	cr.lookupMutex.RLock()
	defer cr.lookupMutex.RUnlock()
	if len(lookup) != 2 {
		t.Errorf("expected 2 pods in lookup, got %d", len(lookup))
	}
	if _, ok := lookup["pod-a"]; !ok {
		t.Error("expected pod-a in lookup")
	}
	if _, ok := lookup["pod-b"]; !ok {
		t.Error("expected pod-b in lookup")
	}
}

func TestInitGetPodsData_SkipsNonRunning(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pending-pod", Namespace: "ns1"},
			Status:     v1.PodStatus{Phase: v1.PodPending},
		},
	)
	origClient := dev.Clientset
	dev.Clientset = fakeClient
	defer func() { dev.Clientset = origClient }()

	lookup := make(map[string]pod)
	cr := Collector{
		containerLimitsPercentage: false,
		lookup:                    &lookup,
		lookupMutex:               &sync.RWMutex{},
		WaitGroup:                 &sync.WaitGroup{},
	}
	cr.WaitGroup.Add(1)
	go cr.initGetPodsData()
	cr.WaitGroup.Wait()

	cr.lookupMutex.RLock()
	defer cr.lookupMutex.RUnlock()
	if _, ok := lookup["pending-pod"]; ok {
		t.Error("expected pending-pod NOT in lookup")
	}
}

func TestSetMetrics_PodUsage(t *testing.T) {
	setupPodMetrics()
	podGaugeVec.Reset()

	c := Collector{
		podUsage:                  true,
		containerVolumeUsage:      false,
		containerLimitsPercentage: false,
		containerVolumeLimitsPercentage: false,
		lookup:                    &map[string]pod{},
		lookupMutex:               &sync.RWMutex{},
	}

	c.SetMetrics("test-pod", "ns1", "node1", 1024, 2048, 4096, 100, 50, 50, nil, nil)

	m := &dto.Metric{}
	err := podGaugeVec.With(prometheus.Labels{
		"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1",
	}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Gauge.GetValue() != 1024 {
		t.Errorf("expected pod usage 1024, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_Inodes(t *testing.T) {
	setupPodMetrics()
	inodesGaugeVec.Reset()
	inodesFreeGaugeVec.Reset()
	inodesUsedGaugeVec.Reset()

	c := Collector{
		podUsage:                  false,
		containerVolumeUsage:      false,
		containerLimitsPercentage: false,
		containerVolumeLimitsPercentage: false,
		inodes:                    true,
		lookup:                    &map[string]pod{},
		lookupMutex:               &sync.RWMutex{},
	}

	c.SetMetrics("test-pod", "ns1", "node1", 0, 0, 0, 1000, 500, 500, nil, nil)

	m := &dto.Metric{}
	inodesGaugeVec.With(prometheus.Labels{
		"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1",
	}).Write(m)
	if m.Gauge.GetValue() != 1000 {
		t.Errorf("expected inodes 1000, got %f", m.Gauge.GetValue())
	}

	inodesFreeGaugeVec.With(prometheus.Labels{
		"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1",
	}).Write(m)
	if m.Gauge.GetValue() != 500 {
		t.Errorf("expected inodesFree 500, got %f", m.Gauge.GetValue())
	}

	inodesUsedGaugeVec.With(prometheus.Labels{
		"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1",
	}).Write(m)
	if m.Gauge.GetValue() != 500 {
		t.Errorf("expected inodesUsed 500, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_ContainerVolumeUsage(t *testing.T) {
	setupPodMetrics()
	containerVolumeUsageVec.Reset()

	lookupData := map[string]pod{
		"test-pod": {
			containers: []container{
				{
					name: "c1",
					emptyDirVolumes: []emptyDirVolumes{
						{name: "cache-vol", mountPath: "/cache"},
					},
				},
			},
		},
	}

	c := Collector{
		podUsage:                  false,
		containerVolumeUsage:      true,
		containerLimitsPercentage: false,
		containerVolumeLimitsPercentage: false,
		lookup:                    &lookupData,
		lookupMutex:               &sync.RWMutex{},
	}

	volumes := []Volume{
		{Name: "cache-vol", UsedBytes: 500},
	}

	c.SetMetrics("test-pod", "ns1", "node1", 0, 0, 0, 0, 0, 0, volumes, nil)

	m := &dto.Metric{}
	err := containerVolumeUsageVec.With(prometheus.Labels{
		"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1",
		"container": "c1", "volume_name": "cache-vol", "mount_path": "/cache",
	}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Gauge.GetValue() != 500 {
		t.Errorf("expected volume usage 500, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_ContainerVolumeLimitPercentage(t *testing.T) {
	setupPodMetrics()
	containerPercentageVolumeLimitsVec.Reset()

	sizeLimit := 1000.0
	lookupData := map[string]pod{
		"test-pod": {
			containers: []container{
				{
					name: "c1",
					emptyDirVolumes: []emptyDirVolumes{
						{name: "cache-vol", mountPath: "/cache", sizeLimit: sizeLimit},
					},
				},
			},
		},
	}

	c := Collector{
		podUsage:                        false,
		containerVolumeUsage:            false,
		containerLimitsPercentage:       false,
		containerVolumeLimitsPercentage: true,
		lookup:                          &lookupData,
		lookupMutex:                     &sync.RWMutex{},
	}

	volumes := []Volume{
		{Name: "cache-vol", UsedBytes: 500},
	}

	c.SetMetrics("test-pod", "ns1", "node1", 0, 0, 0, 0, 0, 0, volumes, nil)

	m := &dto.Metric{}
	err := containerPercentageVolumeLimitsVec.With(prometheus.Labels{
		"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1",
		"container": "c1", "volume_name": "cache-vol", "mount_path": "/cache",
	}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	// UsedBytes 500 * 1.024 = 512 biBytes / 1000 ≈ 51.2%, capped at 100%
	expected := (500.0 * 1.024) / 1000.0 * 100.0
	if m.Gauge.GetValue() != expected {
		t.Errorf("expected volume limit percentage %f, got %f", expected, m.Gauge.GetValue())
	}
}

func TestSetMetrics_ContainerLimitPercentage_WithLimit(t *testing.T) {
	setupPodMetrics()
	containerPercentageLimitsVec.Reset()

	limit := 2000.0
	lookupData := map[string]pod{
		"test-pod": {
			containers: []container{
				{name: "c1", limit: limit},
			},
		},
	}

	c := Collector{
		podUsage:                        false,
		containerVolumeUsage:            false,
		containerLimitsPercentage:       true,
		containerVolumeLimitsPercentage: false,
		lookup:                          &lookupData,
		lookupMutex:                     &sync.RWMutex{},
	}

	c.SetMetrics("test-pod", "ns1", "node1", 500, 1000, 2000, 0, 0, 0, nil, nil)

	m := &dto.Metric{}
	err := containerPercentageLimitsVec.With(prometheus.Labels{
		"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1",
		"container": "c1", "source": "container",
	}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	// Used 500 * 1.024 = 512 biBytes / 2000 ≈ 25.6%
	expected := (500.0 * 1.024) / 2000.0 * 100.0
	if m.Gauge.GetValue() != expected {
		t.Errorf("expected container limit percentage %f, got %f", expected, m.Gauge.GetValue())
	}
}

func TestSetMetrics_ContainerLimitPercentage_NodeFallback(t *testing.T) {
	setupPodMetrics()
	containerPercentageLimitsVec.Reset()

	lookupData := map[string]pod{
		"test-pod": {
			containers: []container{
				{name: "c1", limit: 0}, // No limit → falls back to node
			},
		},
	}

	c := Collector{
		podUsage:                        false,
		containerVolumeUsage:            false,
		containerLimitsPercentage:       true,
		containerVolumeLimitsPercentage: false,
		lookup:                          &lookupData,
		lookupMutex:                     &sync.RWMutex{},
	}

	c.SetMetrics("test-pod", "ns1", "node1", 0, 500, 1000, 0, 0, 0, nil, nil)

	m := &dto.Metric{}
	err := containerPercentageLimitsVec.With(prometheus.Labels{
		"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1",
		"container": "c1", "source": "node",
	}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	// Used = capacity - available = 1000 - 500 = 500 → 50%
	if m.Gauge.GetValue() != 50.0 {
		t.Errorf("expected fallback percentage 50, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_ContainerRootfsUsage(t *testing.T) {
	setupPodMetrics()
	containerRootfsUsedBytesVec.Reset()
	containerRootfsAvailableBytesVec.Reset()
	containerRootfsCapacityBytesVec.Reset()

	c := Collector{
		podUsage:                  false,
		containerVolumeUsage:      false,
		containerLimitsPercentage: false,
		containerVolumeLimitsPercentage: false,
		containerRootfsUsage:      true,
		lookup:                    &map[string]pod{},
		lookupMutex:               &sync.RWMutex{},
	}

	containers := []ContainerStats{
		{
			Name: "c1",
			Rootfs: FsStats{
				UsedBytes:      100,
				AvailableBytes: 200,
				CapacityBytes:  300,
			},
		},
	}

	c.SetMetrics("test-pod", "ns1", "node1", 0, 0, 0, 0, 0, 0, nil, containers)

	labels := prometheus.Labels{"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1", "container": "c1"}

	m := &dto.Metric{}
	containerRootfsUsedBytesVec.With(labels).Write(m)
	if m.Gauge.GetValue() != 100 {
		t.Errorf("expected rootfs used 100, got %f", m.Gauge.GetValue())
	}

	containerRootfsAvailableBytesVec.With(labels).Write(m)
	if m.Gauge.GetValue() != 200 {
		t.Errorf("expected rootfs available 200, got %f", m.Gauge.GetValue())
	}

	containerRootfsCapacityBytesVec.With(labels).Write(m)
	if m.Gauge.GetValue() != 300 {
		t.Errorf("expected rootfs capacity 300, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_ContainerLogsUsage(t *testing.T) {
	setupPodMetrics()
	containerLogsUsedBytesVec.Reset()
	containerLogsAvailableBytesVec.Reset()
	containerLogsCapacityBytesVec.Reset()

	c := Collector{
		podUsage:                  false,
		containerVolumeUsage:      false,
		containerLimitsPercentage: false,
		containerVolumeLimitsPercentage: false,
		containerLogsUsage:        true,
		lookup:                    &map[string]pod{},
		lookupMutex:               &sync.RWMutex{},
	}

	containers := []ContainerStats{
		{
			Name: "c1",
			Logs: FsStats{
				UsedBytes:      50,
				AvailableBytes: 150,
				CapacityBytes:  250,
			},
		},
	}

	c.SetMetrics("test-pod", "ns1", "node1", 0, 0, 0, 0, 0, 0, nil, containers)

	labels := prometheus.Labels{"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1", "container": "c1"}

	m := &dto.Metric{}
	containerLogsUsedBytesVec.With(labels).Write(m)
	if m.Gauge.GetValue() != 50 {
		t.Errorf("expected logs used 50, got %f", m.Gauge.GetValue())
	}

	containerLogsAvailableBytesVec.With(labels).Write(m)
	if m.Gauge.GetValue() != 150 {
		t.Errorf("expected logs available 150, got %f", m.Gauge.GetValue())
	}

	containerLogsCapacityBytesVec.With(labels).Write(m)
	if m.Gauge.GetValue() != 250 {
		t.Errorf("expected logs capacity 250, got %f", m.Gauge.GetValue())
	}
}

func TestEvictPodByName(t *testing.T) {
	setupPodMetrics()
	podGaugeVec.Reset()
	inodesGaugeVec.Reset()

	// Set some metrics first
	c := Collector{
		podUsage: true,
		inodes:   true,
		lookup:   &map[string]pod{},
		lookupMutex: &sync.RWMutex{},
	}
	c.SetMetrics("evict-me", "ns1", "node1", 100, 200, 300, 10, 5, 5, nil, nil)

	p := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "evict-me"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "c1"}},
		},
	}
	evictPodByName(p)

	// Verify metrics are deleted by checking that With returns 0 after delete
	m := &dto.Metric{}
	err := podGaugeVec.With(prometheus.Labels{
		"pod_name": "evict-me", "pod_namespace": "ns1", "node_name": "node1",
	}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Gauge.GetValue() != 0 {
		t.Errorf("expected pod usage 0 after evict, got %f", m.Gauge.GetValue())
	}
}

func TestEvictPodByNode(t *testing.T) {
	setupPodMetrics()
	podGaugeVec.Reset()

	// Set metrics for pods on a specific node
	c := Collector{
		podUsage: true,
		lookup:   &map[string]pod{},
		lookupMutex: &sync.RWMutex{},
	}
	c.SetMetrics("pod1", "ns1", "bad-node", 100, 0, 0, 0, 0, 0, nil, nil)
	c.SetMetrics("pod2", "ns1", "bad-node", 200, 0, 0, 0, 0, 0, nil, nil)

	delLabel := prometheus.Labels{"node_name": "bad-node"}
	EvictPodByNode(&delLabel)

	// Verify pod1 is evicted
	m := &dto.Metric{}
	err := podGaugeVec.With(prometheus.Labels{
		"pod_name": "pod1", "pod_namespace": "ns1", "node_name": "bad-node",
	}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Gauge.GetValue() != 0 {
		t.Errorf("expected pod1 usage 0 after node evict, got %f", m.Gauge.GetValue())
	}

	// Verify pod2 is evicted
	err = podGaugeVec.With(prometheus.Labels{
		"pod_name": "pod2", "pod_namespace": "ns1", "node_name": "bad-node",
	}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Gauge.GetValue() != 0 {
		t.Errorf("expected pod2 usage 0 after node evict, got %f", m.Gauge.GetValue())
	}
}

func TestSetMetrics_PodNotInLookup_AddedToLookup(t *testing.T) {
	c := Collector{
		podUsage:                  true,
		containerVolumeUsage:      false,
		containerLimitsPercentage: false,
		containerVolumeLimitsPercentage: false,
		lookup:                    &map[string]pod{},
		lookupMutex:               &sync.RWMutex{},
	}

	c.SetMetrics("new-pod", "ns1", "node1", 100, 0, 0, 0, 0, 0, nil, nil)

	c.lookupMutex.RLock()
	_, ok := (*c.lookup)["new-pod"]
	c.lookupMutex.RUnlock()
	if !ok {
		t.Error("expected new-pod to be added to lookup")
	}
}

func TestSetMetrics_ContainerLimitPercentage_NaN(t *testing.T) {
	setupPodMetrics()
	containerPercentageLimitsVec.Reset()

	lookupData := map[string]pod{
		"test-pod": {
			containers: []container{
				{name: "c1", limit: 0},
			},
		},
	}

	c := Collector{
		podUsage:                        false,
		containerVolumeUsage:            false,
		containerLimitsPercentage:       true,
		containerVolumeLimitsPercentage: false,
		lookup:                          &lookupData,
		lookupMutex:                     &sync.RWMutex{},
	}

	// limit=0 AND capacityBytes=0 → NaN
	c.SetMetrics("test-pod", "ns1", "node1", 0, 0, 0, 0, 0, 0, nil, nil)

	m := &dto.Metric{}
	err := containerPercentageLimitsVec.With(prometheus.Labels{
		"pod_name": "test-pod", "pod_namespace": "ns1", "node_name": "node1",
		"container": "c1", "source": "node",
	}).Write(m)
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsNaN(m.Gauge.GetValue()) {
		t.Errorf("expected NaN, got %f", m.Gauge.GetValue())
	}
}

func TestRunGC_EvictsDeletedPods(t *testing.T) {
	setupPodMetrics()

	fakeClient := fake.NewSimpleClientset(
		&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "keep-pod"}},
	)
	origClient := dev.Clientset
	dev.Clientset = fakeClient
	defer func() { dev.Clientset = origClient }()

	lookup := map[string]pod{
		"keep-pod":    {},
		"deleted-pod": {},
	}
	c := Collector{
		lookup:      &lookup,
		lookupMutex: &sync.RWMutex{},
	}

	c.runGC(500)

	c.lookupMutex.RLock()
	defer c.lookupMutex.RUnlock()
	if _, ok := lookup["keep-pod"]; !ok {
		t.Error("expected keep-pod to remain")
	}
	if _, ok := lookup["deleted-pod"]; ok {
		t.Error("expected deleted-pod to be removed")
	}
}
