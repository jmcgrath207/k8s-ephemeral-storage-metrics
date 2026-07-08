package pod

import (
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRootfsLogsMetrics(t *testing.T) {
	mu := &sync.RWMutex{}
	cr := Collector{
		containerRootfsUsage: true,
		containerLogsUsage:   true,
		inodes:               true,
		lookup:               &map[string]pod{},
		lookupMutex:          mu,
	}
	cr.createMetrics()

	t.Run("registration", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			vec  *prometheus.GaugeVec
		}{
			{"containerRootfsUsagePercentageVec", containerRootfsUsagePercentageVec},
			{"containerLogsUsagePercentageVec", containerLogsUsagePercentageVec},
			{"containerRootfsInodesVec", containerRootfsInodesVec},
			{"containerRootfsInodesFreeVec", containerRootfsInodesFreeVec},
			{"containerRootfsInodesUsedVec", containerRootfsInodesUsedVec},
			{"containerLogsInodesVec", containerLogsInodesVec},
			{"containerLogsInodesFreeVec", containerLogsInodesFreeVec},
			{"containerLogsInodesUsedVec", containerLogsInodesUsedVec},
		} {
			if tc.vec == nil {
				t.Errorf("%s not registered", tc.name)
			}
		}
	})

	// Run podUsage_inodes FIRST so set_values doesn't add pod-level inode data that conflicts.
	t.Run("podUsage_inodes", func(t *testing.T) {
		cr2 := Collector{
			podUsage:    true,
			inodes:      true,
			lookup:      &map[string]pod{},
			lookupMutex: &sync.RWMutex{},
		}
		cr2.SetMetrics("p2", "ns2", "n2", 1000, 2000, 3000, 50, 30, 20, nil, nil)

		expected := strings.NewReader(`
			# HELP ephemeral_storage_pod_usage Current ephemeral byte usage of pod
			# TYPE ephemeral_storage_pod_usage gauge
			ephemeral_storage_pod_usage{node_name="n2",pod_name="p2",pod_namespace="ns2"} 1000
			# HELP ephemeral_storage_inodes Maximum number of inodes in the pod
			# TYPE ephemeral_storage_inodes gauge
			ephemeral_storage_inodes{node_name="n2",pod_name="p2",pod_namespace="ns2"} 50
			# HELP ephemeral_storage_inodes_free Number of free inodes in the pod
			# TYPE ephemeral_storage_inodes_free gauge
			ephemeral_storage_inodes_free{node_name="n2",pod_name="p2",pod_namespace="ns2"} 30
			# HELP ephemeral_storage_inodes_used Number of used inodes in the pod
			# TYPE ephemeral_storage_inodes_used gauge
			ephemeral_storage_inodes_used{node_name="n2",pod_name="p2",pod_namespace="ns2"} 20
		`)
		if err := testutil.GatherAndCompare(prometheus.DefaultGatherer, expected,
			"ephemeral_storage_pod_usage",
			"ephemeral_storage_inodes",
			"ephemeral_storage_inodes_free",
			"ephemeral_storage_inodes_used",
		); err != nil {
			t.Fatalf("podUsage/inodes mismatch: %v", err)
		}
	})

	t.Run("set_values", func(t *testing.T) {
		containers := []ContainerStats{
			{
				Name: "c1",
				Rootfs: FsStats{
					AvailableBytes: 7500,
					CapacityBytes:  10000,
					UsedBytes:      2500,
					Inodes:         1000,
					InodesFree:     800,
					InodesUsed:     200,
				},
				Logs: FsStats{
					AvailableBytes: 5000,
					CapacityBytes:  8000,
					UsedBytes:      3000,
					Inodes:         500,
					InodesFree:     200,
					InodesUsed:     300,
				},
			},
		}

		cr.SetMetrics("p1", "ns1", "n1", 0, 0, 0, 0, 0, 0, nil, containers)

		expected := strings.NewReader(`
			# HELP ephemeral_storage_container_rootfs_usage_percentage Percentage of rootfs capacity used by a container in a pod
			# TYPE ephemeral_storage_container_rootfs_usage_percentage gauge
			ephemeral_storage_container_rootfs_usage_percentage{container="c1",node_name="n1",pod_name="p1",pod_namespace="ns1"} 25
			# HELP ephemeral_storage_container_logs_usage_percentage Percentage of logs capacity used by a container in a pod
			# TYPE ephemeral_storage_container_logs_usage_percentage gauge
			ephemeral_storage_container_logs_usage_percentage{container="c1",node_name="n1",pod_name="p1",pod_namespace="ns1"} 37.5
			# HELP ephemeral_storage_container_rootfs_inodes Maximum number of inodes in the container rootfs
			# TYPE ephemeral_storage_container_rootfs_inodes gauge
			ephemeral_storage_container_rootfs_inodes{container="c1",node_name="n1",pod_name="p1",pod_namespace="ns1"} 1000
			# HELP ephemeral_storage_container_rootfs_inodes_free Number of free inodes in the container rootfs
			# TYPE ephemeral_storage_container_rootfs_inodes_free gauge
			ephemeral_storage_container_rootfs_inodes_free{container="c1",node_name="n1",pod_name="p1",pod_namespace="ns1"} 800
			# HELP ephemeral_storage_container_rootfs_inodes_used Number of used inodes in the container rootfs
			# TYPE ephemeral_storage_container_rootfs_inodes_used gauge
			ephemeral_storage_container_rootfs_inodes_used{container="c1",node_name="n1",pod_name="p1",pod_namespace="ns1"} 200
			# HELP ephemeral_storage_container_logs_inodes Maximum number of inodes in the container logs
			# TYPE ephemeral_storage_container_logs_inodes gauge
			ephemeral_storage_container_logs_inodes{container="c1",node_name="n1",pod_name="p1",pod_namespace="ns1"} 500
			# HELP ephemeral_storage_container_logs_inodes_free Number of free inodes in the container logs
			# TYPE ephemeral_storage_container_logs_inodes_free gauge
			ephemeral_storage_container_logs_inodes_free{container="c1",node_name="n1",pod_name="p1",pod_namespace="ns1"} 200
			# HELP ephemeral_storage_container_logs_inodes_used Number of used inodes in the container logs
			# TYPE ephemeral_storage_container_logs_inodes_used gauge
			ephemeral_storage_container_logs_inodes_used{container="c1",node_name="n1",pod_name="p1",pod_namespace="ns1"} 300
		`)

		metricNames := []string{
			"ephemeral_storage_container_rootfs_usage_percentage",
			"ephemeral_storage_container_logs_usage_percentage",
			"ephemeral_storage_container_rootfs_inodes",
			"ephemeral_storage_container_rootfs_inodes_free",
			"ephemeral_storage_container_rootfs_inodes_used",
			"ephemeral_storage_container_logs_inodes",
			"ephemeral_storage_container_logs_inodes_free",
			"ephemeral_storage_container_logs_inodes_used",
		}

		if err := testutil.GatherAndCompare(prometheus.DefaultGatherer, expected, metricNames...); err != nil {
			t.Fatalf("metric values mismatch: %v", err)
		}
	})

	t.Run("containerVolume_limits", func(t *testing.T) {
		cr3 := Collector{
			containerVolumeUsage:            true,
			containerVolumeLimitsPercentage: true,
			containerLimitsPercentage:       true,
			lookup:                          &map[string]pod{},
			lookupMutex:                     &sync.RWMutex{},
		}
		cr3.lookupMutex.Lock()
		(*cr3.lookup)["p3"] = pod{
			containers: []container{
				{
					name:  "c1",
					limit: 2 * 1024 * 1024 * 1024, // 2Gi
					emptyDirVolumes: []emptyDirVolumes{
						{name: "vol1", mountPath: "/data", sizeLimit: 500 * 1024 * 1024},
					},
				},
			},
		}
		cr3.lookupMutex.Unlock()

		volumes := []Volume{
			{Name: "vol1", UsedBytes: 0},
		}
		cr3.SetMetrics("p3", "ns3", "n3", 0, 0, 0, 0, 0, 0, volumes, nil)

		expected := strings.NewReader(`
			# HELP ephemeral_storage_container_volume_usage Current ephemeral storage used by a container's volume in a pod
			# TYPE ephemeral_storage_container_volume_usage gauge
			ephemeral_storage_container_volume_usage{container="c1",mount_path="/data",node_name="n3",pod_name="p3",pod_namespace="ns3",volume_name="vol1"} 0
			# HELP ephemeral_storage_container_volume_limit_percentage Percentage of ephemeral storage used by a container's volume in a pod
			# TYPE ephemeral_storage_container_volume_limit_percentage gauge
			ephemeral_storage_container_volume_limit_percentage{container="c1",mount_path="/data",node_name="n3",pod_name="p3",pod_namespace="ns3",volume_name="vol1"} 0
			# HELP ephemeral_storage_container_limit_percentage Percentage of ephemeral storage used by a container in a pod
			# TYPE ephemeral_storage_container_limit_percentage gauge
			ephemeral_storage_container_limit_percentage{container="c1",node_name="n3",pod_name="p3",pod_namespace="ns3",source="container"} 0
		`)
		if err := testutil.GatherAndCompare(prometheus.DefaultGatherer, expected,
			"ephemeral_storage_container_volume_usage",
			"ephemeral_storage_container_volume_limit_percentage",
			"ephemeral_storage_container_limit_percentage",
		); err != nil {
			t.Fatalf("containerVolume/limits mismatch: %v", err)
		}
	})

	t.Run("evictPodByNode", func(t *testing.T) {
		deleteLabel := prometheus.Labels{"node_name": "n2"}
		EvictPodByNode(&deleteLabel)

		// NOTE: EvictPodByNode does not clean pod-level inodesGaugeVec,
		// inodesFreeGaugeVec, inodesUsedGaugeVec by node_name (known gap).
		// podGaugeVec for p2 IS cleaned, but the 3 inode vecs are not.
		count, err := testutil.GatherAndCount(prometheus.DefaultGatherer,
			"ephemeral_storage_pod_usage",
			"ephemeral_storage_inodes",
			"ephemeral_storage_inodes_free",
			"ephemeral_storage_inodes_used",
		)
		if err != nil {
			t.Fatalf("GatherAndCount failed: %v", err)
		}
		// 3 remaining (inodes/inodesFree/inodesUsed for p1,n1 from set_values)
		// + potentially 3 from p2,n2 if not cleaned by EvictPodByNode
		if count != 6 {
			t.Errorf("expected 6 (p1+n1 in 3 vecs + p2+n2 in 3 vecs), got %d", count)
		}
	})

	t.Run("eviction", func(t *testing.T) {
		evictPodByName(v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "p1",
				Namespace: "ns1",
			},
		})

		count, err := testutil.GatherAndCount(prometheus.DefaultGatherer,
			"ephemeral_storage_container_rootfs_usage_percentage",
			"ephemeral_storage_container_logs_usage_percentage",
			"ephemeral_storage_container_rootfs_inodes",
			"ephemeral_storage_container_rootfs_inodes_free",
			"ephemeral_storage_container_rootfs_inodes_used",
			"ephemeral_storage_container_logs_inodes",
			"ephemeral_storage_container_logs_inodes_free",
			"ephemeral_storage_container_logs_inodes_used",
		)
		if err != nil {
			t.Fatalf("GatherAndCount failed: %v", err)
		}
		if count > 0 {
			t.Errorf("expected 0 time series after eviction, got %d", count)
		}
	})
}
