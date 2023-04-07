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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var inCluster string
var clientset *kubernetes.Clientset
var currentNode string
var currentPod string

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

func getMetrics() {

	opsQueued := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_pod_usage",
		Help: "Used to expose Ephemeral Storage metrics for pod ",
	},
		[]string{
			// name of pod for Ephemeral Storage
			"pod_name",
			// Name of Node where pod is placed.
			"node_name",
		},
	)

	prometheus.MustRegister(opsQueued)

	log.Debug().Msg(fmt.Sprintf("getMetrics has been invoked"))
	currentNode = getEnv("CURRENT_NODE_NAME", "")
	currentPod = getEnv("CURRENT_POD_NAME", "")
	for {
		content, err := clientset.RESTClient().Get().AbsPath(fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", currentNode)).DoRaw(context.Background())
		if err != nil {
			log.Error().Msg(fmt.Sprintf("ErrorBadRequst : %s\n", err.Error()))
			os.Exit(1)
		}
		log.Debug().Msg(fmt.Sprintf("Fetched proxy stats from node : %s", currentNode))
		var raw map[string]interface{}
		_ = json.Unmarshal(content, &raw)

		nodeName := raw["node"].(map[string]interface{})["nodeName"].(string)
		for _, element := range raw["pods"].([]interface{}) {

			podName := element.(map[string]interface{})["podRef"].(map[string]interface{})["name"].(string)
			if currentPod != podName {
				usedBytes := element.(map[string]interface{})["ephemeral-storage"].(map[string]interface{})["usedBytes"].(float64)

				opsQueued.With(prometheus.Labels{"pod_name": podName, "node_name": nodeName}).Set(usedBytes)

				log.Debug().Msg(fmt.Sprintf("pod %s on %s with usedBytes: %s", podName, nodeName, usedBytes))
			}
		}

		time.Sleep(15 * time.Second)
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
	go getMetrics()
	port := getEnv("METRICS_PORT", "9100")
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Listener Falied : %s\n", err.Error()))
		panic(err.Error())
	}

}
