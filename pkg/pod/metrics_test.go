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
