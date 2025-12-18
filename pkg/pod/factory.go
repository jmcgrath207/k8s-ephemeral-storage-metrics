package pod

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
)

var (
	lookupMutex sync.RWMutex
	waitGroup   sync.WaitGroup
)

type Collector struct {
	containerVolumeUsage            bool
	containerLimitsPercentage       bool
	containerVolumeLimitsPercentage bool
	inodes                          bool
	lookup                          *map[string]pod
	lookupMutex                     *sync.RWMutex
	podUsage                        bool
	WaitGroup                       *sync.WaitGroup
	sampleInterval                  int64
}

func NewCollector(sampleInterval int64) Collector {
	podUsage, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_POD_USAGE", "false"))
	containerVolumeUsage, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_USAGE", "false"))
	containerLimitsPercentage, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_CONTAINER_LIMIT_PERCENTAGE", "false"))
	containerVolumeLimitsPercentage, _ := strconv.ParseBool(
		dev.GetEnv(
			"EPHEMERAL_STORAGE_CONTAINER_VOLUME_LIMITS_PERCENTAGE", "false",
		),
	)
	inodes, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_INODES", "false"))
	lookup := make(map[string]pod)

	var c = Collector{
		containerVolumeUsage:            containerVolumeUsage,
		containerLimitsPercentage:       containerLimitsPercentage,
		containerVolumeLimitsPercentage: containerVolumeLimitsPercentage,
		inodes:                          inodes,
		lookup:                          &lookup,
		lookupMutex:                     &lookupMutex,
		podUsage:                        podUsage,
		sampleInterval:                  sampleInterval,
		WaitGroup:                       &waitGroup,
	}

	c.createMetrics()

	gcEnabled, _ := strconv.ParseBool(dev.GetEnv("GC_ENABLED", "false"))
	gcInterval, _ := strconv.ParseInt(dev.GetEnv("GC_INTERVAL", "5"), 10, 64)
	gcBatchSize, _ := strconv.ParseInt(dev.GetEnv("GC_BATCH_SIZE", "500"), 10, 64)
	if gcEnabled {
		go c.gcMetrics(gcInterval, gcBatchSize)
	}

	if containerLimitsPercentage || containerVolumeLimitsPercentage {
		waitGroup.Add(1)
		go c.initGetPodsData()
		go c.podWatch()
	}

	return c
}

func (cr Collector) gcMetrics(interval int64, batchSize int64) {
	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			log.Info().Msgf("Starting GC for pods in batches of %d", batchSize)
			paginationContinue := ""
			for {
				pods, err := dev.Clientset.CoreV1().Pods("").List(
					context.Background(),
					metav1.ListOptions{Limit: batchSize, Continue: paginationContinue},
				)
				if err != nil {
					log.Error().Msgf("Error getting pods: %v", err)
					continue
				}

				// Collect current pod names
				podNames := make(map[string]struct{}, len(pods.Items))
				for _, p := range pods.Items {
					podNames[p.Name] = struct{}{}
				}

				// Identify all pods we have metrics for that no longer exist
				deletedPods := make(map[string]struct{})
				cr.lookupMutex.RLock()
				for k := range *cr.lookup {
					if _, ok := podNames[k]; !ok {
						deletedPods[k] = struct{}{}
					}
				}
				cr.lookupMutex.RUnlock()

				// Remove metrics for deleted pods
				cr.lookupMutex.Lock()
				for podName := range deletedPods {
					log.Info().Msgf("Garbage collector removing metrics for deleted pod %s", podName)
					delete(*cr.lookup, podName)
					evictPodByName(
						v1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Name: podName,
							},
						},
					)
				}
				cr.lookupMutex.Unlock()

				if pods.Continue != "" {
					paginationContinue = pods.Continue
				} else {
					// We're done for now, waiting for the next tick
					break
				}
			}
		}
	}
}
