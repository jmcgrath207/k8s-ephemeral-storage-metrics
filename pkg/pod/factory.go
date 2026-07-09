package pod

import (
	"os"
	"strconv"
	"sync"

	"github.com/rs/zerolog/log"

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
	containerRootfsUsage            bool
	containerLogsUsage              bool
	inodes                          bool
	lookup                          *map[string]pod
	lookupMutex                     *sync.RWMutex
	podUsage                        bool
	WaitGroup                       *sync.WaitGroup
	sampleInterval                  int64

	listPodsWithCache bool

	deployAsDaemonSet bool
	currentNodeName   string
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
	containerRootfsUsage, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_CONTAINER_ROOTFS_USAGE", "false"))
	containerLogsUsage, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_CONTAINER_LOGS_USAGE", "false"))
	inodes, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_INODES", "false"))
	lookup := make(map[string]pod)

	listPodsWithCache, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_LIST_PODS_WITH_CACHE", "false"))

	deployAsDaemonSet := dev.DeployAsDaemonSet()
	currentNodeName := dev.CurrentNodeName()

	if deployAsDaemonSet && currentNodeName == "" {
		log.Error().Msg("CURRENT_NODE_NAME is not set, but deploy as DaemonSet")
		os.Exit(1)
	}

	var c = Collector{
		containerVolumeUsage:            containerVolumeUsage,
		containerLimitsPercentage:       containerLimitsPercentage,
		containerVolumeLimitsPercentage: containerVolumeLimitsPercentage,
		containerRootfsUsage:            containerRootfsUsage,
		containerLogsUsage:              containerLogsUsage,
		inodes:                          inodes,
		lookup:                          &lookup,
		lookupMutex:                     &lookupMutex,
		podUsage:                        podUsage,
		sampleInterval:                  sampleInterval,
		WaitGroup:                       &waitGroup,

		listPodsWithCache: listPodsWithCache,

		deployAsDaemonSet: deployAsDaemonSet,
		currentNodeName:   currentNodeName,
	}

	c.createMetrics()

	tolerance, _ := strconv.Atoi(dev.GetEnv("SCRAPE_MISS_TOLERANCE", "2"))
	if tolerance < 1 {
		tolerance = 2
	}
	scrapeMissTolerance = tolerance

	if containerLimitsPercentage || containerVolumeLimitsPercentage {
		waitGroup.Add(1)
		go c.initGetPodsData()
		go c.podWatch()
	}

	return c
}
