package pod

import (
	"fmt"
	"math"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
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
	inodesGaugeVec                     *prometheus.GaugeVec
	inodesFreeGaugeVec                 *prometheus.GaugeVec
	inodesUsedGaugeVec                 *prometheus.GaugeVec
)

type FsStats struct {
	AvailableBytes float64 `json:"availableBytes"`
	CapacityBytes  float64 `json:"capacityBytes"`
	UsedBytes      float64 `json:"usedBytes"`
	Inodes         float64 `json:"inodes"`
	InodesFree     float64 `json:"inodesFree"`
	InodesUsed     float64 `json:"inodesUsed"`
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
	cr.lookupMutex.Lock()
	podResult, okPodResult := (*cr.lookup)[podName]
	if !okPodResult {
		// To ensure we can garbage collect pod metrics we need to make sure all are stored in lookup
		(*cr.lookup)[podName] = pod{}
	}
	cr.lookupMutex.Unlock()

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
			containerRootfsUsedBytesVec.With(labels).Set(c.Rootfs.UsedBytes)
			containerRootfsAvailableBytesVec.With(labels).Set(c.Rootfs.AvailableBytes)
			containerRootfsCapacityBytesVec.With(labels).Set(c.Rootfs.CapacityBytes)
		}
	}

	if cr.containerLogsUsage {
		for _, c := range containers {
			labels := prometheus.Labels{"pod_namespace": podNamespace,
				"pod_name": podName, "node_name": nodeName, "container": c.Name}
			containerLogsUsedBytesVec.With(labels).Set(c.Logs.UsedBytes)
			containerLogsAvailableBytesVec.With(labels).Set(c.Logs.AvailableBytes)
			containerLogsCapacityBytesVec.With(labels).Set(c.Logs.CapacityBytes)
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

	// TODO: Look into removing this for loop and delete by pod_name
	// e.g. containerVolumeUsageVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	for _, c := range p.Spec.Containers {
		containerVolumeUsageVec.DeletePartialMatch(prometheus.Labels{"container": c.Name})
		containerPercentageLimitsVec.DeletePartialMatch(prometheus.Labels{"container": c.Name})
		containerPercentageVolumeLimitsVec.DeletePartialMatch(prometheus.Labels{"container": c.Name})
	}
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
}
