package pod

import (
	"strconv"
	"sync"

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
	containerVolumeLimitsPercentage, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_LIMITS_PERCENTAGE", "false"))
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

	if containerLimitsPercentage || containerVolumeLimitsPercentage {
		waitGroup.Add(1)
		go c.initGetPodsData()
		go c.podWatch()
	}

	return c
}
