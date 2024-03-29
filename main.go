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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

var (
	inCluster                                       string
	clientset                                       *kubernetes.Clientset
	sampleInterval                                  int64
	sampleIntervalMill                              int64
	adjustedPollingRate                             bool
	ephemeralStoragePodUsage                        bool
	ephemeralStorageNodeAvailable                   bool
	ephemeralStorageNodeCapacity                    bool
	ephemeralStorageNodePercentage                  bool
	ephemeralStorageContainerLimitsPercentage       bool
	ephemeralStorageContainerVolumeLimitsPercentage bool
	adjustedPollingRateGaugeVec                     *prometheus.GaugeVec
	deployType                                      string
	nodeWaitGroup                                   sync.WaitGroup
	podDataWaitGroup                                sync.WaitGroup
	podGaugeVec                                     *prometheus.GaugeVec
	nodeAvailableGaugeVec                           *prometheus.GaugeVec
	nodeCapacityGaugeVec                            *prometheus.GaugeVec
	nodePercentageGaugeVec                          *prometheus.GaugeVec
	containerPercentageLimitsVec                    *prometheus.GaugeVec
	containerPercentageVolumeLimitsVec              *prometheus.GaugeVec
	nodeSlice                                       []string
	maxNodeConcurrency                              int
	podResourceLookup                               map[string]pod
	podResourceLookupMutex                          sync.RWMutex
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

type volume struct {
	AvailableBytes int64  `json:"availableBytes"`
	CapacityBytes  int64  `json:"capacityBytes"`
	UsedBytes      int    `json:"usedBytes"`
	Name           string `json:"name"`
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

		Volumes []volume `json:"volume,omitempty"`
	}
}

type pod struct {
	containers []container
}

type container struct {
	name            string
	limit           float64
	emptyDirVolumes []emptyDirVolumes
}

type emptyDirVolumes struct {
	name      string
	mountPath string
	sizeLimit float64
}

// Collector for container data
func getContainerData(c v1.Container, p v1.Pod) container {

	setContainer := container{}
	setContainer.name = c.Name
	matchKey := v1.ResourceName("ephemeral-storage")

	if ephemeralStorageContainerVolumeLimitsPercentage && p.Spec.Volumes != nil {
		collectMounts := false

		podMountsMap := make(map[string]float64)
		for _, v := range p.Spec.Volumes {
			if v.VolumeSource.EmptyDir != nil {
				if v.VolumeSource.EmptyDir.SizeLimit != nil {
					podMountsMap[v.Name] = v.VolumeSource.EmptyDir.SizeLimit.AsApproximateFloat64()
					collectMounts = true
				}
			}

		}

		if collectMounts {
			var collectVolumes []emptyDirVolumes

			for _, volumeMount := range c.VolumeMounts {
				size, ok := podMountsMap[volumeMount.Name]
				if ok {
					collectVolumes = append(collectVolumes, emptyDirVolumes{name: volumeMount.Name, mountPath: volumeMount.MountPath, sizeLimit: size})
				}
			}

			setContainer.emptyDirVolumes = collectVolumes

		}

	}
	if ephemeralStorageContainerLimitsPercentage {
		for key, val := range c.Resources.Limits {
			if key == matchKey {
				setContainer.limit = val.AsApproximateFloat64()
			}
		}
	}
	return setContainer
}

// Collector for pod data
func getPodData(p v1.Pod) {
	if p.Status.Phase == "Running" {
		var collectContainers []container

		for _, x := range p.Spec.Containers {
			collectContainers = append(collectContainers, getContainerData(x, p))
		}

		podResourceLookupMutex.Lock()
		podResourceLookup[p.Name] = pod{containers: collectContainers}
		podResourceLookupMutex.Unlock()
	}
}

func initGetPodsData() {
	podResourceLookup = make(map[string]pod)
	// Init Get List of all pods
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Error().Msgf("Error getting pods: %v\n", err)
		os.Exit(1)
	}

	for _, p := range pods.Items {
		getPodData(p)
	}
	podDataWaitGroup.Done()

}

func podWatch() {
	podDataWaitGroup.Wait()
	stopCh := make(chan struct{})
	defer close(stopCh)
	sharedInformerFactory := informers.NewSharedInformerFactory(clientset, time.Duration(sampleInterval)*time.Second)
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

	// Define event handlers for Pod events
	eventHandler := cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			p := newObj.(*v1.Pod)
			getPodData(*p)
		},
		DeleteFunc: func(obj interface{}) {
			p := obj.(*v1.Pod)
			podResourceLookupMutex.Lock()
			delete(podResourceLookup, p.Name)
			podResourceLookupMutex.Unlock()
			evictPodFromMetrics(*p)
		},
	}

	// Register the event handlers with the informer
	_, err := podInformer.AddEventHandler(eventHandler)
	if err != nil {
		log.Err(err)
		os.Exit(1)
	}

	// Start the informer to begin watching for Pod events
	go sharedInformerFactory.Start(stopCh)

	for {
		time.Sleep(time.Duration(sampleInterval) * time.Second)
		select {
		case <-stopCh:
			log.Error().Msg("Watcher podWatch stopped.")
			os.Exit(1)
		}
	}

}

func evictPodFromMetrics(p v1.Pod) {

	podGaugeVec.DeletePartialMatch(prometheus.Labels{"pod_name": p.Name})
	for _, c := range p.Spec.Containers {
		containerPercentageLimitsVec.DeletePartialMatch(prometheus.Labels{"container": c.Name})
		containerPercentageVolumeLimitsVec.DeletePartialMatch(prometheus.Labels{"container": c.Name})
	}
}

func evictNode(node string) {

	nodeAvailableGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": node})
	nodeCapacityGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": node})
	nodePercentageGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": node})
	if adjustedPollingRate {
		adjustedPollingRateGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": node})
	}
	log.Info().Msgf("Node %s does not exist. Removed from monitoring", node)
}

func getNodes() {
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

	// Poll for new nodes
	// TODO: make this more event driven instead of polling
	for {
		nodes, _ := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		for _, node := range nodes.Items {
			nodeSet.Add(node.Name)
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

func createMetricsValues(podName string, podNamespace string, nodeName string, usedBytes float64, availableBytes float64, capacityBytes float64, volumes []volume) {

	var setValue float64
	podResourceLookupMutex.RLock()
	podResult, okPodResult := podResourceLookup[podName]
	podResourceLookupMutex.RUnlock()

	// TODO: something seems wrong about the metrics.
	//		the volume capacityBytes is not reflected in this query
	// 		kubectl get --raw "/api/v1/nodes/ephemeral-metrics-cluster-worker/proxy/stats/summary"
	// 		need to source it from the pod spec
	// 		make issue upstream with CA advisor
	// TODO: need to do a grow and shrink test for this.
	if ephemeralStorageContainerVolumeLimitsPercentage {
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
								containerPercentageVolumeLimitsVec.With(labels).Set((float64(v.UsedBytes) / edv.sizeLimit) * 100.0)
							}
						}
					}
				}
			}
		}
	}

	if ephemeralStorageContainerLimitsPercentage {
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

	if ephemeralStoragePodUsage {
		labels := prometheus.Labels{"pod_namespace": podNamespace,
			"pod_name": podName, "node_name": nodeName}
		podGaugeVec.With(labels).Set(usedBytes)
		log.Debug().Msg(fmt.Sprintf("pod %s/%s on %s with usedBytes: %f", podNamespace, podName, nodeName, usedBytes))
	}
	if ephemeralStorageNodeAvailable {
		nodeAvailableGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(availableBytes)
		log.Debug().Msg(fmt.Sprintf("Node: %s availble bytes: %f", nodeName, availableBytes))
	}

	if ephemeralStorageNodeCapacity {
		nodeCapacityGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(capacityBytes)
		log.Debug().Msg(fmt.Sprintf("Node: %s capacity bytes: %f", nodeName, capacityBytes))
	}

	if ephemeralStorageNodePercentage {
		setValue = (availableBytes / capacityBytes) * 100.0
		nodePercentageGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(setValue)
		log.Debug().Msg(fmt.Sprintf("Node: %s percentage used: %f", nodeName, setValue))
	}

}

func setMetrics(node string) {

	var data ephemeralStorageMetrics

	start := time.Now()

	content, err := queryNode(node)
	if err != nil {
		evictNode(node)
		return
	}

	log.Debug().Msg(fmt.Sprintf("Fetched proxy stats from node : %s", node))
	_ = json.Unmarshal(content, &data)

	nodeName := data.Node.NodeName

	for _, p := range data.Pods {
		podName := p.PodRef.Name
		podNamespace := p.PodRef.Namespace
		usedBytes := p.EphemeralStorage.UsedBytes
		availableBytes := p.EphemeralStorage.AvailableBytes
		capacityBytes := p.EphemeralStorage.CapacityBytes
		if podNamespace == "" || (usedBytes == 0 && availableBytes == 0 && capacityBytes == 0) {
			log.Warn().Msg(fmt.Sprintf("pod %s/%s on %s has no metrics on its ephemeral storage usage", podName, podNamespace, nodeName))
			continue
		}
		createMetricsValues(podName, podNamespace, nodeName, usedBytes,
			availableBytes, capacityBytes, p.Volumes)
	}

	adjustTime := sampleIntervalMill - time.Now().Sub(start).Milliseconds()
	if adjustTime <= 0.0 {
		log.Error().Msgf("Node %s: Polling Rate could not keep up. Adjust your Interval to a higher number than %d seconds", nodeName, sampleInterval)
	}
	if adjustedPollingRate {
		adjustedPollingRateGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(float64(adjustTime))
	}

}

func createMetrics() {

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

	if adjustedPollingRate {
		adjustedPollingRateGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ephemeral_storage_adjusted_polling_rate",
			Help: "AdjustTime polling rate time after a Node API queries in Milliseconds",
		},
			[]string{
				// Name of Node where pod is placed.
				"node_name",
			})

		prometheus.MustRegister(adjustedPollingRateGaugeVec)
	}

}

func getMetrics() {

	nodeWaitGroup.Wait()
	if ephemeralStorageContainerLimitsPercentage || ephemeralStorageContainerVolumeLimitsPercentage {
		podDataWaitGroup.Wait()
	}

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

func enablePprof() {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	err := http.ListenAndServe("localhost:6060", mux)
	if err != nil {
		log.Error().Msgf("Pprof could not start localhost:")
	}
}

func main() {
	flag.Parse()
	port := getEnv("METRICS_PORT", "9100")
	// TODO: move to a configmap.
	adjustedPollingRate, _ = strconv.ParseBool(getEnv("ADJUSTED_POLLING_RATE", "false"))
	ephemeralStoragePodUsage, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_POD_USAGE", "false"))
	ephemeralStorageNodeAvailable, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_NODE_AVAILABLE", "false"))
	ephemeralStorageNodeCapacity, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_NODE_CAPACITY", "false"))
	ephemeralStorageNodePercentage, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_NODE_PERCENTAGE", "false"))
	ephemeralStorageContainerLimitsPercentage, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_CONTAINER_LIMIT_PERCENTAGE", "false"))
	ephemeralStorageContainerVolumeLimitsPercentage, _ = strconv.ParseBool(getEnv("EPHEMERAL_STORAGE_CONTAINER_VOLUME_LIMITS_PERCENTAGE", "false"))
	pprofEnabled, _ := strconv.ParseBool(getEnv("PPROF", "false"))
	deployType = getEnv("DEPLOY_TYPE", "DaemonSet")
	sampleInterval, _ = strconv.ParseInt(getEnv("SCRAPE_INTERVAL", "15"), 10, 64)
	maxNodeConcurrency, _ = strconv.Atoi(getEnv("MAX_NODE_CONCURRENCY", "10"))
	sampleIntervalMill = sampleInterval * 1000

	if pprofEnabled {
		go enablePprof()
	}
	setLogger()
	getK8sClient()
	createMetrics()
	if ephemeralStorageContainerLimitsPercentage || ephemeralStorageContainerVolumeLimitsPercentage {
		podDataWaitGroup.Add(1)
		go initGetPodsData()
		go podWatch()
	}
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
