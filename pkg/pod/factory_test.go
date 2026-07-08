package pod

import (
	"sync"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFactory(t *testing.T) {
	t.Run("getPodData_running", func(t *testing.T) {
		lookup := make(map[string]pod)
		cr := Collector{
			containerVolumeUsage: true,
			lookup:               &lookup,
			lookupMutex:          &sync.RWMutex{},
		}
		p := v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Status:     v1.PodStatus{Phase: v1.PodRunning},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Name: "c1"},
				},
			},
		}
		cr.getPodData(p)
		cr.lookupMutex.RLock()
		pd, ok := (*cr.lookup)["test-pod"]
		cr.lookupMutex.RUnlock()
		if !ok {
			t.Fatal("expected pod in lookup")
		}
		if len(pd.containers) != 1 {
			t.Fatalf("expected 1 container, got %d", len(pd.containers))
		}
		if pd.containers[0].name != "c1" {
			t.Fatalf("expected container c1, got %s", pd.containers[0].name)
		}
	})

	t.Run("getPodData_not_running", func(t *testing.T) {
		lookup := make(map[string]pod)
		cr := Collector{
			lookup:      &lookup,
			lookupMutex: &sync.RWMutex{},
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
			t.Fatal("expected no entry for pending pod")
		}
	})

	t.Run("getContainerData_no_flags", func(t *testing.T) {
		cr := Collector{}
		c := v1.Container{Name: "c1"}
		p := v1.Pod{}
		result := cr.getContainerData(c, p)
		if result.name != "c1" {
			t.Fatalf("expected c1, got %s", result.name)
		}
		if result.limit != 0 {
			t.Fatalf("expected limit 0, got %f", result.limit)
		}
		if len(result.emptyDirVolumes) != 0 {
			t.Fatalf("expected 0 volumes, got %d", len(result.emptyDirVolumes))
		}
	})

	t.Run("getContainerData_with_limit", func(t *testing.T) {
		cr := Collector{containerLimitsPercentage: true}
		c := v1.Container{
			Name: "c1",
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceEphemeralStorage: resource.MustParse("2Gi"),
				},
			},
		}
		p := v1.Pod{}
		result := cr.getContainerData(c, p)
		if result.limit != 2*1024*1024*1024 {
			t.Fatalf("expected limit 2147483648, got %f", result.limit)
		}
	})

	t.Run("getContainerData_without_limit", func(t *testing.T) {
		cr := Collector{containerLimitsPercentage: true}
		c := v1.Container{
			Name: "c1",
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU: resource.MustParse("1"),
				},
			},
		}
		p := v1.Pod{}
		result := cr.getContainerData(c, p)
		if result.limit != 0 {
			t.Fatalf("expected limit 0 when no ephemeral-storage limit, got %f", result.limit)
		}
	})

	t.Run("getContainerData_with_volumes", func(t *testing.T) {
		cr := Collector{containerVolumeUsage: true}
		c := v1.Container{
			Name: "c1",
			VolumeMounts: []v1.VolumeMount{
				{Name: "vol1", MountPath: "/data"},
			},
		}
		p := v1.Pod{
			Spec: v1.PodSpec{
				Volumes: []v1.Volume{
					{Name: "vol1", VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{
							SizeLimit: resource.NewQuantity(500*1024*1024, resource.BinarySI),
						},
					}},
				},
			},
		}
		result := cr.getContainerData(c, p)
		if len(result.emptyDirVolumes) != 1 {
			t.Fatalf("expected 1 volume, got %d", len(result.emptyDirVolumes))
		}
		if result.emptyDirVolumes[0].name != "vol1" {
			t.Fatalf("expected vol1, got %s", result.emptyDirVolumes[0].name)
		}
		if result.emptyDirVolumes[0].mountPath != "/data" {
			t.Fatalf("expected /data, got %s", result.emptyDirVolumes[0].mountPath)
		}
		if result.emptyDirVolumes[0].sizeLimit != 500*1024*1024 {
			t.Fatalf("expected 524288000, got %f", result.emptyDirVolumes[0].sizeLimit)
		}
	})

	t.Run("getContainerData_volumes_disabled", func(t *testing.T) {
		cr := Collector{containerVolumeUsage: false}
		c := v1.Container{
			Name: "c1",
			VolumeMounts: []v1.VolumeMount{
				{Name: "vol1", MountPath: "/data"},
			},
		}
		p := v1.Pod{
			Spec: v1.PodSpec{
				Volumes: []v1.Volume{
					{Name: "vol1", VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					}},
				},
			},
		}
		result := cr.getContainerData(c, p)
		if len(result.emptyDirVolumes) != 0 {
			t.Fatalf("expected 0 volumes when containerVolumeUsage disabled, got %d", len(result.emptyDirVolumes))
		}
	})

}
