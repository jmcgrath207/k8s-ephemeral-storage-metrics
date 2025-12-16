package node

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/rs/zerolog/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
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

	gcEnabled, _ := strconv.ParseBool(dev.GetEnv("GC_ENABLED", "false"))
	gcInterval, _ := strconv.ParseInt(dev.GetEnv("GC_INTERVAL", "5"), 10, 64)
	if gcEnabled {
		go node.gcMetrics(gcInterval)
	}

	if node.deployType != "Deployment" {
		node.Set.Add(dev.GetEnv("CURRENT_NODE_NAME", ""))
	} else {
		go node.Watch()
	}

	return node
}

func (n Node) gcMetrics(interval int64) {
	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			log.Info().Msgf("Starting GC for nodes")
			nodes, err := dev.Clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				log.Error().Msgf("Error getting nodes: %v", err)
				continue
			}

			// Create current node names
			nodeNames := map[string]struct{}{}
			for _, n := range nodes.Items {
				nodeNames[n.Name] = struct{}{}
			}

			// Identify all nodes we have metrics for that no longer exist
			for nodeName := range n.Set.Iter() {
				if _, ok := nodeNames[nodeName]; !ok {
					log.Info().Msgf("Garbage collector removing metrics for deleted node %s", nodeName)
					n.evict(nodeName)
					if n.scrapeFromKubelet {
						n.KubeletEndpoint.Delete(nodeName)
					}
				}
			}
		}
	}
}
