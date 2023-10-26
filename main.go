package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
	inCluster           string
	clientset           *kubernetes.Clientset
	sampleInterval      int64
	adjustedPollingRate bool
	adjustedTimeGauge   prometheus.Gauge
	deployType          string
	nodeSlice           []string
	nodeWaitGroup       sync.WaitGroup
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
	nodeWaitGroup.Add(1)
	if deployType != "Deployment" {
		nodeSlice = append(nodeSlice, getEnv("CURRENT_NODE_NAME", ""))
		nodeWaitGroup.Done()
		return
	}
	for {
		nodeSlice = nil
		nodes, _ := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		for _, node := range nodes.Items {
			nodeSlice = append(nodeSlice, node.Name)
		}
		nodeWaitGroup.Done()
		time.Sleep(1 * time.Minute)
		nodeWaitGroup.Add(1)
	}

}

type CollectMetric struct {
	usedBytes float64
	labels    prometheus.Labels
}

func getMetrics() {
	nodeWaitGroup.Wait()
	var labelsList []CollectMetric
	opsQueued := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_pod_usage",
		Help: "Used to expose Ephemeral Storage metrics for pod in bytes ",
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

	prometheus.MustRegister(opsQueued)

	log.Debug().Msg(fmt.Sprintf("getMetrics has been invoked"))

	if adjustedPollingRate {
		adjustedTimeGauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ephemeral_storage_adjusted_polling_rate",
			Help: "AdjustTime polling rate time after a Node API queries in Milliseconds",
		})

		prometheus.MustRegister(adjustedTimeGauge)
	}
	sampleInterval, _ = strconv.ParseInt(getEnv("SCRAPE_INTERVAL", "15"), 10, 64)
	sampleInterval = sampleInterval * 1000
	adjustTime := sampleInterval
	for {
		start := time.Now()

		for _, node := range nodeSlice {

			content, err := clientset.RESTClient().Get().AbsPath(fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", node)).DoRaw(context.Background())
			if err != nil {
				log.Error().Msg(fmt.Sprintf("ErrorBadRequst : %s\n", err.Error()))
				os.Exit(1)
			}
			log.Debug().Msg(fmt.Sprintf("Fetched proxy stats from node : %s", node))
			var data ephemeralStorageMetrics
			_ = json.Unmarshal(content, &data)

			nodeName := data.Node.NodeName
			for _, pod := range data.Pods {
				podName := pod.PodRef.Name
				podNamespace := pod.PodRef.Namespace
				usedBytes := pod.EphemeralStorage.UsedBytes
				if podNamespace == "" || (usedBytes == 0 && pod.EphemeralStorage.AvailableBytes == 0 && pod.EphemeralStorage.CapacityBytes == 0) {
					log.Warn().Msg(fmt.Sprintf("pod %s/%s on %s has no metrics on its ephemeral storage usage", podName, podNamespace, nodeName))
					log.Warn().Msg(fmt.Sprintf("raw content %v", content))
				}
				labelsList = append(labelsList, CollectMetric{
					usedBytes,
					prometheus.Labels{"pod_namespace": podNamespace,
						"pod_name": podName, "node_name": nodeName},
				})

				log.Debug().Msg(fmt.Sprintf("pod %s/%s on %s with usedBytes: %f", podNamespace, podName, nodeName, usedBytes))
			}
		}

		// reset this metrics in the Exporter to flush dead pods
		opsQueued.Reset()
		// Push new metrics to exporter
		for _, x := range labelsList {
			opsQueued.With(x.labels).Set(x.usedBytes)
		}
		// Zero out collection list
		labelsList = nil

		elapsedTime := time.Now().Sub(start).Milliseconds()
		adjustTime = sampleInterval - elapsedTime
		if adjustTime <= 0.0 {
			log.Error().Msgf("Adjusted Poll Rate: %d ms", adjustTime)
			log.Error().Msgf("Polling Rate could not keep up. Adjust your Interval to a higher number than %d", sampleInterval)
			os.Exit(1)
		}
		if adjustedPollingRate {
			adjustedTimeGauge.Set(float64(adjustTime))
		}
		log.Debug().Msgf("Adjusted Poll Rate: %d ms", adjustTime)
		time.Sleep(time.Duration(adjustTime) * time.Millisecond)
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
	setLogger()
	getK8sClient()
	go getNodes()
	go getMetrics()
	port := getEnv("METRICS_PORT", "9100")
	adjustedPollingRate, _ = strconv.ParseBool(getEnv("ADJUSTED_POLLING_RATE", "false"))
	deployType = getEnv("DEPLOY_TYPE", "DaemonSet")
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
