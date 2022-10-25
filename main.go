package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
)

var inCluster string
var clientset *kubernetes.Clientset
var reg *prometheus.Registry
var currentNode string

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

	log.Debug().Msg(fmt.Sprintf("getMetrics has been invoked"))
	currentNode = getEnv("CURRENT_NODE_NAME", "")
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

		pod_name := element.(map[string]interface{})["podRef"].(map[string]interface{})["name"].(string)

		opsQueued := promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name:        "ephemeral_storage_pod_usage",
			Help:        "Used to expose Ephemeral Storage metrics for pod ",
			ConstLabels: prometheus.Labels{"pod_name": pod_name, "node_name": nodeName},
		})

		// ERROR
		// 2022/10/25 04:22:23 http: panic serving 127.0.0.1:47698: duplicate metrics collector registration attempted
		// TODO: https://github.com/prometheus/client_golang/issues/716
		prometheus.MustRegister(opsQueued)

		usedBytes := element.(map[string]interface{})["ephemeral-storage"].(map[string]interface{})["usedBytes"].(float64)
		opsQueued.Set(usedBytes)

	}
}

func prometheusMiddleware(next http.Handler) http.Handler {
	log.Debug().Msg("Invoked prometheusMiddleware")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getMetrics()
		next.ServeHTTP(w, r)
	})
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
	reg = prometheus.NewRegistry()
	r := mux.NewRouter()
	r.Use(prometheusMiddleware)
	r.Path("/metrics").Handler(promhttp.Handler())
	port := getEnv("METRICS_PORT", "9100")
	srv := &http.Server{Addr: fmt.Sprintf("localhost:%v", port), Handler: r}
	err := srv.ListenAndServe()
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Listener Falied : %s\n", err.Error()))
		panic(err.Error())
	}

}
