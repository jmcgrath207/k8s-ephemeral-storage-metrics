package node

import (
	"fmt"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/rs/zerolog/log"
	"os"
	"strconv"
	"sync"
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
	Set                     mapset.Set[string]
	WaitGroup               *sync.WaitGroup
}

func NewCollector(sampleInterval int64) Node {

	adjustedPollingRate, _ := strconv.ParseBool(dev.GetEnv("ADJUSTED_POLLING_RATE", "false"))
	deployType := dev.GetEnv("DEPLOY_TYPE", "DaemonSet")
	nodeAvailable, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_NODE_AVAILABLE", "false"))
	nodeCapacity, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_NODE_CAPACITY", "false"))
	nodePercentage, _ := strconv.ParseBool(dev.GetEnv("EPHEMERAL_STORAGE_NODE_PERCENTAGE", "false"))
	maxNodeQueryConcurrency, _ := strconv.Atoi(dev.GetEnv("MAX_NODE_CONCURRENCY", "10"))
	set := mapset.NewSet[string]()

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
		Set:                     set,
		WaitGroup:               &waitGroup,
	}
	node.createMetrics()
	return node
}
