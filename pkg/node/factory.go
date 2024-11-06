package node

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/rs/zerolog/log"
)

var (
	waitGroup sync.WaitGroup
)

type Node struct {
	AdjustedPollingRate     bool
	deployType              string
	MaxNodeQueryConcurrency int
	nodeAvailable           bool
	nodeCapacity            bool
	nodePercentage          bool
	sampleInterval          int64
	scrapeFromKubelet       bool
	kubeletReadOnlyPort     int
	Set                     mapset.Set[string]
	KubeletEndpoint         *sync.Map // key=nodeName val=kubeletEndpoint
	WaitGroup               *sync.WaitGroup
}

func NewCollector(sampleInterval int64) Node {

	adjustedPollingRate, _ := strconv.ParseBool(dev.GetEnv("ADJUSTED_POLLING_RATE", "false"))
	deployType := dev.GetEnv("DEPLOY_TYPE", "DaemonSet")
	nodeAvailable, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_NODE_AVAILABLE", "false"))
	nodeCapacity, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_NODE_CAPACITY", "false"))
	nodePercentage, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_NODE_PERCENTAGE", "false"))
	maxNodeQueryConcurrency, _ := strconv.Atoi(dev.GetEnv("MAX_NODE_CONCURRENCY", "10"))
	scrapeFromKubelet, _ := strconv.ParseBool(dev.GetEnv("SCRAPE_FROM_KUBELET", "false"))
	kubeletReadOnlyPort, _ := strconv.Atoi(dev.GetEnv("KUBELET_READONLY_PORT", "0"))
	set := mapset.NewSet[string]()
	mp := &sync.Map{}

	if deployType != "Deployment" && deployType != "DaemonSet" {
		log.Error().Msg(fmt.Sprintf("deployType must be 'Deployment' or 'DaemonSet', got %s", deployType))
		os.Exit(1)
	}

	node := Node{
		AdjustedPollingRate:     adjustedPollingRate,
		deployType:              deployType,
		MaxNodeQueryConcurrency: maxNodeQueryConcurrency,
		nodeAvailable:           nodeAvailable,
		nodeCapacity:            nodeCapacity,
		nodePercentage:          nodePercentage,
		sampleInterval:          sampleInterval,
		scrapeFromKubelet:       scrapeFromKubelet,
		kubeletReadOnlyPort:     kubeletReadOnlyPort,
		Set:                     set,
		KubeletEndpoint:         mp,
		WaitGroup:               &waitGroup,
	}
	node.createMetrics()

	if node.deployType != "Deployment" {
		node.Set.Add(dev.GetEnv("CURRENT_NODE_NAME", ""))
	} else {
		go node.Watch()
	}

	return node
}
