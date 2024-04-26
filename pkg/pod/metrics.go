package pod

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
)

var (
	podGaugeVec                        *prometheus.GaugeVec
	containerVolumeUsageVec            *prometheus.GaugeVec
	containerPercentageLimitsVec       *prometheus.GaugeVec
	containerPercentageVolumeLimitsVec *prometheus.GaugeVec
)

type Volume struct {
	AvailableBytes int64  `json:"availableBytes"`
	CapacityBytes  int64  `json:"capacityBytes"`
	UsedBytes      int    `json:"usedBytes"`
	Name           string `json:"name"`
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
}

func (cr Collector) SetMetrics(podName string, podNamespace string, nodeName string, usedBytes float64, availableBytes float64, capacityBytes float64, volumes []Volume) {

	var setValue float64
	cr.lookupMutex.RLock()
	podResult, okPodResult := (*cr.lookup)[podName]
	cr.lookupMutex.RUnlock()

	// TODO: something seems wrong about the metrics.
	//		the volume capacityBytes is not reflected in this query
	// 		kubectl get --raw "/api/v1/nodes/ephemeral-metrics-cluster-worker/proxy/stats/summary"
	// 		need to source it from the pod spec
	// 		make issue upstream with CA advisor
	// TODO: need to do a grow and shrink test for this.
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
									containerPercentageVolumeLimitsVec.With(labels).Set((float64(v.UsedBytes) / edv.sizeLimit) * 100.0)
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
					"pod_name": podName, "node_name": nodeName, "container": c.name}
				if c.limit != 0 {
					// Use Limit from Container
					setValue = (usedBytes / c.limit) * 100.0
				} else {
					// Default to Node Available Ephemeral Storage
					setValue = (availableBytes / capacityBytes) * 100.0
				}
				containerPercentageLimitsVec.With(labels).Set(setValue)
			}
		}
	}

	if cr.podUsage {
		labels := prometheus.Labels{"pod_namespace": podNamespace,
			"pod_name": podName, "node_name": nodeName}
		podGaugeVec.With(labels).Set(usedBytes)
		log.Debug().Msg(fmt.Sprintf("pod %s/%s on %s with usedBytes: %f", podNamespace, podName, nodeName, usedBytes))
	}
}

func evictPodFromMetrics(p v1.Pod) {

	podGaugeVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	for _, c := range p.Spec.Containers {
		containerVolumeUsageVec.DeletePartialMatch(prometheus.Labels{"container": c.Name})
		containerPercentageLimitsVec.DeletePartialMatch(prometheus.Labels{"container": c.Name})
		containerPercentageVolumeLimitsVec.DeletePartialMatch(prometheus.Labels{"container": c.Name})
	}
}
