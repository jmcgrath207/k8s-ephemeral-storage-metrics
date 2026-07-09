package pod

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	podGaugeVec                        *prometheus.GaugeVec
	containerVolumeUsageVec            *prometheus.GaugeVec
	containerPercentageLimitsVec       *prometheus.GaugeVec
	containerPercentageVolumeLimitsVec *prometheus.GaugeVec
	containerRootfsUsedBytesVec        *prometheus.GaugeVec
	containerRootfsAvailableBytesVec   *prometheus.GaugeVec
	containerRootfsCapacityBytesVec    *prometheus.GaugeVec
	containerLogsUsedBytesVec          *prometheus.GaugeVec
	containerLogsAvailableBytesVec     *prometheus.GaugeVec
	containerLogsCapacityBytesVec      *prometheus.GaugeVec
	containerRootfsUsagePercentageVec  *prometheus.GaugeVec
	containerLogsUsagePercentageVec    *prometheus.GaugeVec
	containerRootfsInodesVec           *prometheus.GaugeVec
	containerRootfsInodesFreeVec       *prometheus.GaugeVec
	containerRootfsInodesUsedVec       *prometheus.GaugeVec
	containerLogsInodesVec             *prometheus.GaugeVec
	containerLogsInodesFreeVec         *prometheus.GaugeVec
	containerLogsInodesUsedVec         *prometheus.GaugeVec
	inodesGaugeVec                     *prometheus.GaugeVec
	inodesFreeGaugeVec                 *prometheus.GaugeVec
	inodesUsedGaugeVec                 *prometheus.GaugeVec

	// nodeTrackers holds per-node scrape-driven eviction state.
	// Keyed by nodeName; value is *podTracker.
	nodeTrackers sync.Map

	// scrapeMissTolerance is the number of consecutive scrapes a pod
	// can be missing from the stats summary before its metrics are evicted.
	// Set in NewCollector from the SCRAPE_MISS_TOLERANCE env var.
	scrapeMissTolerance int
)

// podTracker tracks which pods were seen on a node across scrapes.
// On each scrape, pods present in the stats summary reset their miss count
// to 0. Pods absent from the stats summary increment their miss count; when
// it reaches scrapeMissTolerance, the pod's metrics are evicted.
type podTracker struct {
	mu       sync.Mutex
	lastSeen map[string]int
}

type FsStats struct {
	AvailableBytes int64 `json:"availableBytes"`
	CapacityBytes  int64 `json:"capacityBytes"`
	UsedBytes      int   `json:"usedBytes"`
	Inodes         int64 `json:"inodes"`
	InodesFree     int64 `json:"inodesFree"`
	InodesUsed     int64 `json:"inodesUsed"`
}

type Volume struct {
	AvailableBytes int64  `json:"availableBytes"`
	CapacityBytes  int64  `json:"capacityBytes"`
	UsedBytes      int    `json:"usedBytes"`
	Name           string `json:"name"`
}

type ContainerStats struct {
	Name   string  `json:"name"`
	Rootfs FsStats `json:"rootfs"`
	Logs   FsStats `json:"logs"`
}

func (cr Collector) createMetrics() {

	podGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_pod_usage",
		Help: "Current ephemeral byte usage of pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
		},
	)

	prometheus.MustRegister(podGaugeVec)

	containerVolumeUsageVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_volume_usage",
		Help: "Current ephemeral storage used by a container's volume in a pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
			// Name of container
			"container",
			// Name of Volume
			"volume_name",
			// Name of Mount Path
			"mount_path",
		},
	)

	prometheus.MustRegister(containerVolumeUsageVec)

	containerPercentageLimitsVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_limit_percentage",
		Help: "Percentage of ephemeral storage used by a container in a pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
			// Name of container
			"container",
			// Source of the limit (either "container" for pod.spec.containers.resources.limits or "node")
			"source",
		},
	)

	prometheus.MustRegister(containerPercentageLimitsVec)

	containerPercentageVolumeLimitsVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_volume_limit_percentage",
		Help: "Percentage of ephemeral storage used by a container's volume in a pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
			// Name of container
			"container",
			// Name of Volume
			"volume_name",
			// Name of Mount Path
			"mount_path",
		},
	)

	prometheus.MustRegister(containerPercentageVolumeLimitsVec)

	containerRootfsUsedBytesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_rootfs_used_bytes",
		Help: "Current rootfs bytes used by a container in a pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
			// Name of container
			"container",
		},
	)
	prometheus.MustRegister(containerRootfsUsedBytesVec)

	containerRootfsAvailableBytesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_rootfs_available_bytes",
		Help: "Current rootfs bytes available to a container in a pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
			// Name of container
			"container",
		},
	)
	prometheus.MustRegister(containerRootfsAvailableBytesVec)

	containerRootfsCapacityBytesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_rootfs_capacity_bytes",
		Help: "Current rootfs bytes capacity for a container in a pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
			// Name of container
			"container",
		},
	)
	prometheus.MustRegister(containerRootfsCapacityBytesVec)

	containerLogsUsedBytesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_logs_used_bytes",
		Help: "Current logs bytes used by a container in a pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
			// Name of container
			"container",
		},
	)
	prometheus.MustRegister(containerLogsUsedBytesVec)

	containerLogsAvailableBytesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_logs_available_bytes",
		Help: "Current logs bytes available to a container in a pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
			// Name of container
			"container",
		},
	)
	prometheus.MustRegister(containerLogsAvailableBytesVec)

	containerLogsCapacityBytesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_logs_capacity_bytes",
		Help: "Current logs bytes capacity for a container in a pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
			// Name of container
			"container",
		},
	)
	prometheus.MustRegister(containerLogsCapacityBytesVec)

	containerRootfsUsagePercentageVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_rootfs_usage_percentage",
		Help: "Percentage of rootfs capacity used by a container in a pod",
	},
		[]string{
			"pod_name",
			"pod_namespace",
			"node_name",
			"container",
		},
	)
	prometheus.MustRegister(containerRootfsUsagePercentageVec)

	containerLogsUsagePercentageVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_logs_usage_percentage",
		Help: "Percentage of logs capacity used by a container in a pod",
	},
		[]string{
			"pod_name",
			"pod_namespace",
			"node_name",
			"container",
		},
	)
	prometheus.MustRegister(containerLogsUsagePercentageVec)

	containerRootfsInodesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_rootfs_inodes",
		Help: "Maximum number of inodes in the container rootfs",
	},
		[]string{
			"pod_name",
			"pod_namespace",
			"node_name",
			"container",
		},
	)
	prometheus.MustRegister(containerRootfsInodesVec)

	containerRootfsInodesFreeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_rootfs_inodes_free",
		Help: "Number of free inodes in the container rootfs",
	},
		[]string{
			"pod_name",
			"pod_namespace",
			"node_name",
			"container",
		},
	)
	prometheus.MustRegister(containerRootfsInodesFreeVec)

	containerRootfsInodesUsedVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_rootfs_inodes_used",
		Help: "Number of used inodes in the container rootfs",
	},
		[]string{
			"pod_name",
			"pod_namespace",
			"node_name",
			"container",
		},
	)
	prometheus.MustRegister(containerRootfsInodesUsedVec)

	containerLogsInodesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_logs_inodes",
		Help: "Maximum number of inodes in the container logs",
	},
		[]string{
			"pod_name",
			"pod_namespace",
			"node_name",
			"container",
		},
	)
	prometheus.MustRegister(containerLogsInodesVec)

	containerLogsInodesFreeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_logs_inodes_free",
		Help: "Number of free inodes in the container logs",
	},
		[]string{
			"pod_name",
			"pod_namespace",
			"node_name",
			"container",
		},
	)
	prometheus.MustRegister(containerLogsInodesFreeVec)

	containerLogsInodesUsedVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_container_logs_inodes_used",
		Help: "Number of used inodes in the container logs",
	},
		[]string{
			"pod_name",
			"pod_namespace",
			"node_name",
			"container",
		},
	)
	prometheus.MustRegister(containerLogsInodesUsedVec)

	inodesGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_inodes",
		Help: "Maximum number of inodes in the pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
		},
	)

	prometheus.MustRegister(inodesGaugeVec)

	inodesFreeGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_inodes_free",
		Help: "Number of free inodes in the pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
		},
	)

	prometheus.MustRegister(inodesFreeGaugeVec)

	inodesUsedGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_inodes_used",
		Help: "Number of used inodes in the pod",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// namespace of pod for Ephemeral Storage
			"pod_namespace",
			// Name of Node where pod is placed.
			"node_name",
		},
	)

	prometheus.MustRegister(inodesUsedGaugeVec)
}

func (cr Collector) SetMetrics(podName string, podNamespace string, nodeName string, usedBytes float64, availableBytes float64, capacityBytes float64, inodes float64, inodesFree float64, inodesUsed float64, volumes []Volume, containers []ContainerStats) {

	var setValue float64
	cr.lookupMutex.RLock()
	podResult, okPodResult := (*cr.lookup)[podName]
	cr.lookupMutex.RUnlock()

	// TODO: something seems wrong about the metrics.
	//		the volume capacityBytes is not reflected in this query
	// 		kubectl get --raw "/api/v1/nodes/ephemeral-metrics-cluster-worker/proxy/stats/summary"
	// 		need to source it from the pod spec
	// 		make issue upstream with CA advisor
	if cr.containerVolumeUsage {
		// TODO: what a mess...need to figure out a better way.
		if okPodResult {
			for _, c := range podResult.containers {
				if c.emptyDirVolumes != nil {
					for _, edv := range c.emptyDirVolumes {
						for _, v := range volumes {
							if edv.name == v.Name {
								labels := prometheus.Labels{"pod_namespace": podNamespace,
									"pod_name": podName, "node_name": nodeName, "container": c.name, "volume_name": v.Name,
									"mount_path": edv.mountPath}
								containerVolumeUsageVec.With(labels).Set(float64(v.UsedBytes))
								log.Debug().Msg(fmt.Sprintf("pod %s/%s/%s  on %s with usedBytes: %f", podNamespace, podName, c.name, nodeName, usedBytes))
							}
						}
					}
				}
			}
		}
	}

	if cr.containerVolumeLimitsPercentage {
		// TODO: what a mess...need to figure out a better way.
		if okPodResult {
			for _, c := range podResult.containers {
				if c.emptyDirVolumes != nil {
					for _, edv := range c.emptyDirVolumes {
						if edv.sizeLimit != 0 {
							for _, v := range volumes {
								if edv.name == v.Name {
									labels := prometheus.Labels{"pod_namespace": podNamespace,
										"pod_name": podName, "node_name": nodeName, "container": c.name, "volume_name": v.Name,
										"mount_path": edv.mountPath}
									// Convert used bytes to *bibyte since. Since the volume limit in the pod manifest is in *bibyte, but the
									// Used bytes from the Kube API is not.
									// multiply the digital storage value by 1.024
									// https://stackoverflow.com/a/50805048/3263650
									usedBiBytes := float64(v.UsedBytes) * 1.024
									setValue = math.Min((usedBiBytes/edv.sizeLimit)*100.0, 100.0)
									containerPercentageVolumeLimitsVec.With(labels).Set(setValue)
								}
							}
						}
					}
				}
			}
		}
	}

	if cr.containerLimitsPercentage {
		if okPodResult {
			for _, c := range podResult.containers {
				labels := prometheus.Labels{"pod_namespace": podNamespace,
					"pod_name": podName, "node_name": nodeName, "container": c.name, "source": "node"}
				if c.limit != 0 {
					// Use limit if found.
					// Convert used bytes to *bibyte since. Since the limit in the pod manifest is in *bibyte, but the
					// Used bytes from the Kube API is not.
					// multiply the digital storage value by 1.024
					// https://stackoverflow.com/a/50805048/3263650
					usedBiBytes := usedBytes * 1.024
					setValue = math.Min((usedBiBytes/c.limit)*100.0, 100.0)
					labels["source"] = "container"
				} else if capacityBytes > 0. {
					// Default to Node Used Ephemeral Storage
					setValue = math.Max(capacityBytes-availableBytes, 0.) * 100.0 / capacityBytes
				} else {
					setValue = math.NaN()
				}
				containerPercentageLimitsVec.With(labels).Set(setValue)
			}
		}
	}

	if cr.containerRootfsUsage {
		for _, c := range containers {
			labels := prometheus.Labels{"pod_namespace": podNamespace,
				"pod_name": podName, "node_name": nodeName, "container": c.Name}
			containerRootfsUsedBytesVec.With(labels).Set(float64(c.Rootfs.UsedBytes))
			containerRootfsAvailableBytesVec.With(labels).Set(float64(c.Rootfs.AvailableBytes))
			containerRootfsCapacityBytesVec.With(labels).Set(float64(c.Rootfs.CapacityBytes))
			if c.Rootfs.CapacityBytes > 0 {
				containerRootfsUsagePercentageVec.With(labels).Set(float64(c.Rootfs.UsedBytes) / float64(c.Rootfs.CapacityBytes) * 100.0)
			}
			if cr.inodes {
				containerRootfsInodesVec.With(labels).Set(float64(c.Rootfs.Inodes))
				containerRootfsInodesFreeVec.With(labels).Set(float64(c.Rootfs.InodesFree))
				containerRootfsInodesUsedVec.With(labels).Set(float64(c.Rootfs.InodesUsed))
			}
		}
	}

	if cr.containerLogsUsage {
		for _, c := range containers {
			labels := prometheus.Labels{"pod_namespace": podNamespace,
				"pod_name": podName, "node_name": nodeName, "container": c.Name}
			containerLogsUsedBytesVec.With(labels).Set(float64(c.Logs.UsedBytes))
			containerLogsAvailableBytesVec.With(labels).Set(float64(c.Logs.AvailableBytes))
			containerLogsCapacityBytesVec.With(labels).Set(float64(c.Logs.CapacityBytes))
			if c.Logs.CapacityBytes > 0 {
				containerLogsUsagePercentageVec.With(labels).Set(float64(c.Logs.UsedBytes) / float64(c.Logs.CapacityBytes) * 100.0)
			}
			if cr.inodes {
				containerLogsInodesVec.With(labels).Set(float64(c.Logs.Inodes))
				containerLogsInodesFreeVec.With(labels).Set(float64(c.Logs.InodesFree))
				containerLogsInodesUsedVec.With(labels).Set(float64(c.Logs.InodesUsed))
			}
		}
	}

	if cr.podUsage {
		labels := prometheus.Labels{"pod_namespace": podNamespace,
			"pod_name": podName, "node_name": nodeName}
		podGaugeVec.With(labels).Set(usedBytes)
		log.Debug().Msg(fmt.Sprintf("pod %s/%s on %s with usedBytes: %f", podNamespace, podName, nodeName, usedBytes))
	}

	if cr.inodes {
		labels := prometheus.Labels{"pod_namespace": podNamespace,
			"pod_name": podName, "node_name": nodeName}
		inodesGaugeVec.With(labels).Set(inodes)
		inodesFreeGaugeVec.With(labels).Set(inodesFree)
		inodesUsedGaugeVec.With(labels).Set(inodesUsed)
		log.Debug().Msg(fmt.Sprintf("pod %s/%s on %s with inodes: %f, inodesFree: %f, inodesUsed: %f", podNamespace, podName, nodeName, inodes, inodesFree, inodesUsed))
	}
}

// Evicts exporter metrics by pod and container name
func evictPodByName(p v1.Pod) {
	start := time.Now()
	podGaugeVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	inodesGaugeVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	inodesFreeGaugeVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	inodesUsedGaugeVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerRootfsUsedBytesVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerRootfsAvailableBytesVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerRootfsCapacityBytesVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerLogsUsedBytesVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerLogsAvailableBytesVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerLogsCapacityBytesVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerRootfsUsagePercentageVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerLogsUsagePercentageVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerRootfsInodesVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerRootfsInodesFreeVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerRootfsInodesUsedVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerLogsInodesVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerLogsInodesFreeVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerLogsInodesUsedVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})

	containerVolumeUsageVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerPercentageLimitsVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	containerPercentageVolumeLimitsVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	duration := time.Since(start)
	if duration > 100*time.Millisecond {
		log.Warn().
			Str("pod", fmt.Sprintf("%s/%s", p.Namespace, p.Name)).
			Dur("duration", duration).
			Msg("Pod metrics eviction took longer than 100ms")
	}
}

// EvictPodByNode Evicts exporter metrics by Node
func EvictPodByNode(deleteLabel *prometheus.Labels) {
	if nodeName, ok := (*deleteLabel)["node_name"]; ok {
		nodeTrackers.Delete(nodeName)
	}
	podGaugeVec.DeletePartialMatch(*deleteLabel)
	containerVolumeUsageVec.DeletePartialMatch(*deleteLabel)
	containerPercentageLimitsVec.DeletePartialMatch(*deleteLabel)
	containerPercentageVolumeLimitsVec.DeletePartialMatch(*deleteLabel)
	containerRootfsUsedBytesVec.DeletePartialMatch(*deleteLabel)
	containerRootfsAvailableBytesVec.DeletePartialMatch(*deleteLabel)
	containerRootfsCapacityBytesVec.DeletePartialMatch(*deleteLabel)
	containerLogsUsedBytesVec.DeletePartialMatch(*deleteLabel)
	containerLogsAvailableBytesVec.DeletePartialMatch(*deleteLabel)
	containerLogsCapacityBytesVec.DeletePartialMatch(*deleteLabel)
	containerRootfsUsagePercentageVec.DeletePartialMatch(*deleteLabel)
	containerLogsUsagePercentageVec.DeletePartialMatch(*deleteLabel)
	containerRootfsInodesVec.DeletePartialMatch(*deleteLabel)
	containerRootfsInodesFreeVec.DeletePartialMatch(*deleteLabel)
	containerRootfsInodesUsedVec.DeletePartialMatch(*deleteLabel)
	containerLogsInodesVec.DeletePartialMatch(*deleteLabel)
	containerLogsInodesFreeVec.DeletePartialMatch(*deleteLabel)
	containerLogsInodesUsedVec.DeletePartialMatch(*deleteLabel)
}

// EvictStalePods evicts metrics for pods on nodeName that have been absent
// from the kubelet stats summary for scrapeMissTolerance consecutive scrapes.
//
// Each scrape passes the current set of pod names from the stats summary.
// Pods present in the summary reset their miss count to 0. Pods absent
// increment their miss count; when it reaches scrapeMissTolerance, the pod's
// metrics are evicted and the pod is removed from the tracker.
//
// Query failures (node unreachable) do not call this function — the caller
// returns early on error, so miss counts are not incremented spuriously.
func EvictStalePods(nodeName string, currentPods []string) {
	t, _ := nodeTrackers.LoadOrStore(nodeName, &podTracker{lastSeen: make(map[string]int)})
	tracker := t.(*podTracker)

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	currentSet := make(map[string]struct{}, len(currentPods))
	for _, name := range currentPods {
		currentSet[name] = struct{}{}
		tracker.lastSeen[name] = 0
	}

	for podName, misses := range tracker.lastSeen {
		if _, exists := currentSet[podName]; !exists {
			misses++
			if misses >= scrapeMissTolerance {
				log.Info().Msgf("Scrape-driven eviction: pod %s on node %s missing %d scrapes, evicting", podName, nodeName, misses)
				evictPodByName(v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName}})
				delete(tracker.lastSeen, podName)
			} else {
				tracker.lastSeen[podName] = misses
			}
		}
	}
}
