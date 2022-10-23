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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"net/http"
	"os"
	"path/filepath"
)

var inCluster string
var clientset *kubernetes.Clientset
var reg *prometheus.Registry
var currentNode string

// kubectl get --raw "/api/v1/nodes/x861.lab.com/proxy/stats/summary" | less

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getK8sClient() {

	if inCluster == "true" {

		config, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
		// creates the clientset
		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			panic(err.Error())
		}

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

	currentNode = getEnv("CURRENT_NODE_NAME", "")
	content, err := clientset.RESTClient().Get().AbsPath(fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", currentNode)).DoRaw(context.Background())
	if err != nil {
		fmt.Printf("ErrorBadRequst : %s\n", err.Error())
		os.Exit(1)
	}
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

		prometheus.MustRegister(opsQueued)

		usedBytes := element.(map[string]interface{})["ephemeral-storage"].(map[string]interface{})["usedBytes"].(float64)
		opsQueued.Set(usedBytes)

	}
}

func prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		getMetrics()
		next.ServeHTTP(w, r)
	})
}

func main() {
	inCluster = getEnv("IN_CLUSTER", "true")
	getK8sClient()
	reg = prometheus.NewRegistry()
	r := mux.NewRouter()
	r.Use(prometheusMiddleware)
	r.Path("/metrics").Handler(promhttp.Handler())
	port := getEnv("METRICS_PORT", "9100")
	srv := &http.Server{Addr: fmt.Sprintf("localhost:%v", port), Handler: r}
	err := srv.ListenAndServe()
	if err != nil {
		return
	}
	fmt.Printf("asdfasdf", inCluster)

}
