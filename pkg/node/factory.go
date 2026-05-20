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
	nodeLabelSelector       string
	Set                     mapset.Set[string]
	KubeletEndpoint         *sync.Map // key=nodeName val=kubeletEndpoint
	WaitGroup               *sync.WaitGroup
	inFlight                *sync.Map          // tracks nodes currently being queried
	failureCooldown         *sync.Map          // key=nodeName val=time.Time of last failure
	cooldownMultiplier      int64              // multiplier for sampleInterval to compute cooldown duration
	timeNow                 func() time.Time   // injectable clock for testing
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
	nodeLabelSelector := dev.GetEnv("NODE_LABEL_SELECTOR", "")
	cooldownMultiplier, _ := strconv.ParseInt(dev.GetEnv("FAILURE_COOLDOWN_MULTIPLIER", "3"), 10, 64)
	if cooldownMultiplier <= 0 {
		cooldownMultiplier = 3
	}
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
		nodeLabelSelector:       nodeLabelSelector,
		Set:                     set,
		KubeletEndpoint:         mp,
		WaitGroup:               &waitGroup,
		inFlight:                &sync.Map{},
		failureCooldown:         &sync.Map{},
		cooldownMultiplier:      cooldownMultiplier,
		timeNow:                 time.Now,
	}
	node.createMetrics()

	gcEnabled, _ := strconv.ParseBool(dev.GetEnv("GC_ENABLED", "false"))
	gcInterval, _ := strconv.ParseInt(dev.GetEnv("GC_INTERVAL", "5"), 10, 64)
	gcBatchSize, _ := strconv.ParseInt(dev.GetEnv("GC_BATCH_SIZE", "500"), 10, 64)
	if gcEnabled {
		go node.gcMetrics(gcInterval, gcBatchSize)
	}

	if node.deployType != "Deployment" {
		currentNodeName := dev.GetEnv("CURRENT_NODE_NAME", "")
		node.Set.Add(currentNodeName)
		if node.scrapeFromKubelet {
			node.initKubeletEndpoint(currentNodeName)
		}
	} else {
		go node.Watch()
	}

	return node
}

func (n *Node) initKubeletEndpoint(nodeName string) {
	nodeObj, err := dev.Clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		log.Error().Msgf("Failed to get node %s for kubelet endpoint: %v", nodeName, err)
		os.Exit(1)
	}
	ep := n.getKubeletEndpoint(nodeObj)
	if ep == "" {
		log.Error().Msgf("No internal IP found for node %s", nodeName)
		os.Exit(1)
	}
	n.KubeletEndpoint.Store(nodeName, ep)
	log.Info().Msgf("Kubelet endpoint for node %s: %s", nodeName, ep)
}

func (n Node) gcMetrics(interval int64, batchSize int64) {
	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			log.Info().Msgf("Starting GC for nodes in batches of %d", batchSize)
			currentNodes := make(map[string]struct{})
			paginationContinue := ""

			for {
				nodes, err := dev.Clientset.CoreV1().Nodes().List(
					context.Background(),
					metav1.ListOptions{
						Limit:         batchSize,
						Continue:      paginationContinue,
						LabelSelector: n.nodeLabelSelector,
					},
				)
				if err != nil {
					log.Error().Msgf("Error getting nodes: %v", err)
					continue
				}

				// Collect pod names from this batch
				for _, n := range nodes.Items {
					currentNodes[n.Name] = struct{}{}
				}

				if nodes.Continue != "" {
					paginationContinue = nodes.Continue
				} else {
					// All batches processed
					break
				}
			}
			log.Info().Msgf("Found %d current nodes in cluster", len(currentNodes))

			// Identify all nodes we have metrics for that no longer exist
			for nodeName := range n.Set.Iter() {
				if _, ok := currentNodes[nodeName]; !ok {
					log.Info().Msgf("Garbage collector removing metrics for deleted node %s", nodeName)
					n.evict(nodeName)
					n.ClearCooldown(nodeName)
					if n.scrapeFromKubelet {
						n.KubeletEndpoint.Delete(nodeName)
					}
				}
			}

			log.Info().Msgf("Node GC cycle completed")
		}
	}
}
