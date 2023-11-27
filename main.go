package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/panjf2000/ants/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

var (
	inCluster                      string
	clientset                      *kubernetes.Clientset
	sampleInterval                 int64
	sampleIntervalMill             int64
	adjustedPollingRate            bool
	ephemeralStoragePodUsage       bool
	ephemeralStorageNodeAvailable  bool
	ephemeralStorageNodeCapacity   bool
	ephemeralStorageNodePercentage bool
	adjustedTimeGaugeVec           *prometheus.GaugeVec
	deployType                     string
	nodeWaitGroup                  sync.WaitGroup
	podGaugeVec                    *prometheus.GaugeVec
	nodeAvailableGaugeVec          *prometheus.GaugeVec
	nodeCapacityGaugeVec           *prometheus.GaugeVec
	nodePercentageGaugeVec         *prometheus.GaugeVec
	nodeSlice                      []string
	maxNodeConcurrency             int
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getK8sClient() {
	inCluster = getEnv("IN_CLUSTER", "true")

	if inCluster == "true" {

		config, err := rest.InClusterConfig()
		if err != nil {
			log.Error().Msg("Failed to get rest config for in cluster client")
			panic(err.Error())
		}
		// creates the clientset
		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			log.Error().Msg("Failed to get client set for in cluster client")
			panic(err.Error())
		}
		log.Debug().Msg("Successful got the in cluster client")

	} else {

		var kubeconfig *string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		flag.Parse()

		// use the current context in kubeconfig
		config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err.Error())
		}

		// create the clientset
		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			panic(err.Error())
		}

	}
}

type ephemeralStorageMetrics struct {
	Node struct {
		NodeName string `json:"nodeName"`
	}
	Pods []struct {
		PodRef struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		}
		EphemeralStorage struct {
			AvailableBytes float64 `json:"availableBytes"`
			CapacityBytes  float64 `json:"capacityBytes"`
			UsedBytes      float64 `json:"usedBytes"`
		} `json:"ephemeral-storage"`
	}
}

func getNodes() {
	oldNodeSet := mapset.NewSet[string]()
	nodeSet := mapset.NewSet[string]()
	nodeWaitGroup.Add(1)
	if deployType != "Deployment" {
		nodeSet.Add(getEnv("CURRENT_NODE_NAME", ""))
		nodeWaitGroup.Done()
		return
	}

	// Init Node slice
	startNodes, _ := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	for _, node := range startNodes.Items {
		nodeSet.Add(node.Name)
	}
	nodeSlice = nodeSet.ToSlice()
	nodeWaitGroup.Done()

	// Poll for new nodes and remove dead ones
	for {
		oldNodeSet = nodeSet.Clone()
		nodeSet.Clear()
		nodes, _ := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		for _, node := range nodes.Items {
			nodeSet.Add(node.Name)
		}
		deadNodesSet := nodeSet.Difference(oldNodeSet)

		// Evict Metrics where the node doesn't exist anymore.
		for _, deadNode := range deadNodesSet.ToSlice() {
			podGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": deadNode})
			log.Info().Msgf("Node %s does not exist. Removing from monitoring", deadNode)
		}

		nodeSlice = nodeSet.ToSlice()
		time.Sleep(1 * time.Minute)
	}

}

func queryNode(node string) ([]byte, error) {
	var content []byte

	bo := backoff.NewExponentialBackOff()
	bo.MaxInterval = 1 * time.Second
	bo.MaxElapsedTime = time.Duration(sampleInterval) * time.Second

	operation := func() error {
		var err error
		content, err = clientset.RESTClient().Get().AbsPath(fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", node)).DoRaw(context.Background())
		if err != nil {
			return err
		}
		return nil
	}

	err := backoff.Retry(operation, bo)

	if err != nil {
		log.Warn().Msg(fmt.Sprintf("Failed fetched proxy stats from node : %s", node))
		return nil, err
	}

	return content, nil

}

type CollectMetric struct {
	value  float64
	name   string
	labels prometheus.Labels
}

func setMetrics(node string) {

	var labelsList []CollectMetric
	var data ephemeralStorageMetrics

	start := time.Now()

	content, err := queryNode(node)
	if err != nil {
		log.Warn().Msg(fmt.Sprintf("Could not query node: %s. Skipping..", node))
		return
	}

	log.Debug().Msg(fmt.Sprintf("Fetched proxy stats from node : %s", node))
	_ = json.Unmarshal(content, &data)

	nodeName := data.Node.NodeName

	for _, pod := range data.Pods {
		podName := pod.PodRef.Name
		podNamespace := pod.PodRef.Namespace
		usedBytes := pod.EphemeralStorage.UsedBytes
		availableBytes := pod.EphemeralStorage.AvailableBytes
		capacityBytes := pod.EphemeralStorage.CapacityBytes
		if podNamespace == "" || (usedBytes == 0 && pod.EphemeralStorage.AvailableBytes == 0 && pod.EphemeralStorage.CapacityBytes == 0) {
			log.Warn().Msg(fmt.Sprintf("pod %s/%s on %s has no metrics on its ephemeral storage usage", podName, podNamespace, nodeName))
			continue
		}

		if ephemeralStoragePodUsage {
			labelsList = append(labelsList, CollectMetric{
				value: usedBytes,
				name:  "ephemeral_storage_pod_usage",
				labels: prometheus.Labels{"pod_namespace": podNamespace,
					"pod_name": podName, "node_name": nodeName},
			})
			log.Debug().Msg(fmt.Sprintf("pod %s/%s on %s with usedBytes: %f", podNamespace, podName, nodeName, usedBytes))
		}
		if ephemeralStorageNodeAvailable {
			labelsList = append(labelsList, CollectMetric{
				value:  availableBytes,
				name:   "ephemeral_storage_node_available",
				labels: prometheus.Labels{"node_name": nodeName}},
			)
			log.Debug().Msg(fmt.Sprintf("Node: %s availble bytes: %f", nodeName, availableBytes))
		}

		if ephemeralStorageNodeCapacity {
			labelsList = append(labelsList, CollectMetric{
				value:  capacityBytes,
				name:   "ephemeral_storage_node_capacity",
				labels: prometheus.Labels{"node_name": nodeName}},
			)
			log.Debug().Msg(fmt.Sprintf("Node: %s capacity bytes: %f", nodeName, capacityBytes))
		}

		if ephemeralStorageNodeCapacity {
			percentage := (availableBytes / capacityBytes) * 100.0
			labelsList = append(labelsList, CollectMetric{
				value:  percentage,
				name:   "ephemeral_storage_node_percentage",
				labels: prometheus.Labels{"node_name": nodeName}},
			)
			log.Debug().Msg(fmt.Sprintf("Node: %s percentage used: %f", nodeName, percentage))
		}

	}

	// Reset Metrics for this Node name to remove dead pods
	podGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": nodeName})

	// Push new metrics to exporter
	for _, x := range labelsList {
		switch x.name {
		case "ephemeral_storage_pod_usage":
			podGaugeVec.With(x.labels).Set(x.value)
		case "ephemeral_storage_node_available":
			nodeAvailableGaugeVec.With(x.labels).Set(x.value)
		case "ephemeral_storage_node_capacity":
			nodeCapacityGaugeVec.With(x.labels).Set(x.value)
		case "ephemeral_storage_node_percentage":
			nodePercentageGaugeVec.With(x.labels).Set(x.value)
		}

	}

	adjustTime := sampleIntervalMill - time.Now().Sub(start).Milliseconds()
	if adjustTime <= 0.0 {
		log.Error().Msgf("Node %s: Polling Rate could not keep up. Adjust your Interval to a higher number than %d seconds", nodeName, sampleInterval)
	}
	if adjustedPollingRate {
		adjustedTimeGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(float64(adjustTime))
	}

}

func createMetrics() {

	if ephemeralStoragePodUsage {
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

	}

	if ephemeralStorageNodeAvailable {
		nodeAvailableGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ephemeral_storage_node_available",
			Help: "Available ephemeral storage for a node",
		},
			[]string{
				// Name of Node where pod is placed.
				"node_name",
			},
		)

		prometheus.MustRegister(nodeAvailableGaugeVec)
	}

	if ephemeralStorageNodeCapacity {
		nodeCapacityGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ephemeral_storage_node_capacity",
			Help: "Capacity of ephemeral storage for a node",
		},
			[]string{
				// Name of Node where pod is placed.
				"node_name",
			},
		)

		prometheus.MustRegister(nodeCapacityGaugeVec)
	}

	if ephemeralStorageNodePercentage {
		nodePercentageGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ephemeral_storage_node_percentage",
			Help: "Percentage of ephemeral storage used on a node",
		},
			[]string{
				// Name of Node where pod is placed.
				"node_name",
			},
		)

		prometheus.MustRegister(nodePercentageGaugeVec)
	}

	if adjustedPollingRate {
		adjustedTimeGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ephemeral_storage_adjusted_polling_rate",
			Help: "AdjustTime polling rate time after a Node API queries in Milliseconds",
		},
			[]string{
				// Name of Node where pod is placed.
				"node_name",
			})

		prometheus.MustRegister(adjustedTimeGaugeVec)
	}

}

func getMetrics() {

	nodeWaitGroup.Wait()

	p, _ := ants.NewPoolWithFunc(maxNodeConcurrency, func(node interface{}) {
		setMetrics(node.(string))
	}, ants.WithExpiryDuration(time.Duration(sampleInterval)*time.Second))

	defer p.Release()

	for {

		for _, node := range nodeSlice {
			_ = p.Invoke(node)
		}

		time.Sleep(time.Duration(sampleInterval) * time.Second)
	}
}

type LineInfoHook struct{}

func (h LineInfoHook) Run(e *zerolog.Event, l zerolog.Level, msg string) {
	_, file, line, ok := runtime.Caller(0)
	if ok {
		e.Str("line", fmt.Sprintf("%s:%d", file, line))
	}
}

func setLogger() {
	logLevel := getEnv("LOG_LEVEL", "info")
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		panic(err.Error())
	}
	zerolog.SetGlobalLevel(level)
	log.Hook(LineInfoHook{})

}

func main() {
	flag.Parse()
	port := getEnv("METRICS_PORT", "9100")
	adjustedPollingRate, _ = strconv.ParseBool(getEnv("ADJUSTED_POLLING_RATE", "false"))
	ephemeralStoragePodUsage, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_POD_USAGE", "false"))
	ephemeralStorageNodeAvailable, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_NODE_AVAILABLE", "false"))
	ephemeralStorageNodeCapacity, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_NODE_CAPACITY", "false"))
	ephemeralStorageNodePercentage, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_NODE_PERCENTAGE", "false"))
	deployType = getEnv("DEPLOY_TYPE", "DaemonSet")
	sampleInterval, _ = strconv.ParseInt(getEnv("SCRAPE_INTERVAL", "15"), 10, 64)
	maxNodeConcurrency, _ = strconv.Atoi(getEnv("MAX_NODE_CONCURRENCY", "10"))
	sampleIntervalMill = sampleInterval * 1000

	setLogger()
	getK8sClient()
	createMetrics()
	go getNodes()
	go getMetrics()
	if deployType != "Deployment" && deployType != "DaemonSet" {
		log.Error().Msg(fmt.Sprintf("deployType must be 'Deployment' or 'DaemonSet', got %s", deployType))
		os.Exit(1)
	}
	http.Handle("/metrics", promhttp.Handler())
	log.Info().Msg(fmt.Sprintf("Starting server listening on :%s", port))
	err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Listener Failed : %s\n", err.Error()))
		panic(err.Error())
	}
}
