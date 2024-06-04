package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/node"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/pod"
	"github.com/panjf2000/ants/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"os"
	"strconv"
	"time"
)

var (
	sampleInterval     int64
	sampleIntervalMill int64
	Node               node.Node
	Pod                pod.Collector
	clientset          *kubernetes.Clientset
)

func getMonitoredNamespaces(clientset *kubernetes.Clientset) ([]string, error) {
	labelSelector := os.Getenv("EPHEMERAL_STORAGE_LABEL")

	if labelSelector == "" {
		labelSelector = "ephemeral-storage-monitoring=enabled"
	}

	namespaceList, err := clientset.CoreV1().Namespaces().List(context.TODO(), v1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	var namespaces []string
	for _, ns := range namespaceList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	return namespaces, nil
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

		Volumes []pod.Volume `json:"volume,omitempty"`
	}
}

func setMetrics(clientset *kubernetes.Clientset, nodeName string, monitoredNamespaces map[string]bool) {
	if clientset == nil {
		log.Error().Msg("Kubernetes clientset is nil")
		return
	}

	var data ephemeralStorageMetrics

	start := time.Now()

	content, err := Node.Query(nodeName)
	if err != nil {
		log.Warn().Msgf("Could not query node %s for ephemeral storage", nodeName)
		return
	}

	log.Debug().Msg(fmt.Sprintf("Fetched proxy stats from node : %s", nodeName))
	err = json.Unmarshal(content, &data)
	if err != nil {
		log.Error().Msgf("Failed to unmarshal content: %v", err)
		return
	}

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

		if !monitoredNamespaces[podNamespace] {
			log.Debug().Msg(fmt.Sprintf("Skipping pod %s/%s as it does not meet label requirements", podName, podNamespace))
			continue
		}

		Node.SetMetrics(nodeName, availableBytes, capacityBytes)
		Pod.SetMetrics(podName, podNamespace, nodeName, usedBytes, availableBytes, capacityBytes, p.Volumes)
	}

	//adjustTime := sampleIntervalMill - time.Now().Sub(start).Milliseconds()
	adjustTime := sampleIntervalMill - time.Since(start).Milliseconds()
	if adjustTime <= 0.0 {
		log.Error().Msgf("Node %s: Polling Rate could not keep up. Adjust your Interval to a higher number than %d seconds", nodeName, sampleInterval)
	}
	if Node.AdjustedPollingRate {
		node.AdjustedPollingRateGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(float64(adjustTime))
	}

}

func getMetrics(clientset *kubernetes.Clientset) {

	Node.WaitGroup.Wait()
	Pod.WaitGroup.Wait()

	p, _ := ants.NewPoolWithFunc(Node.MaxNodeQueryConcurrency, func(node interface{}) {
		monitoredNamespacesList, err := getMonitoredNamespaces(clientset)
		if err != nil {
			log.Error().Msgf("Failed to fetch monitored namespaces: %v", err)
			return
		}

		monitoredNamespaces := make(map[string]bool)
		for _, ns := range monitoredNamespacesList {
			monitoredNamespaces[ns] = true
		}
		setMetrics(clientset, node.(string), monitoredNamespaces)
	}, ants.WithExpiryDuration(time.Duration(sampleInterval)*time.Second))

	defer p.Release()

	for {
		nodeSlice := Node.Set.ToSlice()

		for _, node := range nodeSlice {
			_ = p.Invoke(node)
		}

		time.Sleep(time.Duration(sampleInterval) * time.Second)
	}
}

func main() {
	flag.Parse()
	port := dev.GetEnv("METRICS_PORT", "9100")

	pprofEnabled, _ := strconv.ParseBool(dev.GetEnv("PPROF", "false"))
	sampleInterval, _ = strconv.ParseInt(dev.GetEnv("SCRAPE_INTERVAL", "15"), 10, 64)
	sampleIntervalMill = sampleInterval * 1000

	dev.SetLogger()
	dev.SetK8sClient()
	Node = node.NewCollector(sampleInterval)
	Pod = pod.NewCollector(sampleInterval)

	var err error
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Error().Msgf("Failed to create in-cluster config: %v", err)
		return
	}

	config.QPS = 20.0
	config.Burst = 40
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Error().Msgf("Failed to create Kubernetes clientset: %v", err)
		return
	}

	if pprofEnabled {
		go dev.EnablePprof()
	}
	go Node.Get()
	go Node.Watch()

	go func() {
		Node.WaitGroup.Wait()
		Pod.WaitGroup.Wait()

		p, _ := ants.NewPoolWithFunc(Node.MaxNodeQueryConcurrency, func(node interface{}) {
			monitoredNamespacesList, err := getMonitoredNamespaces(clientset)
			if err != nil {
				log.Error().Msgf("Failed to fetch monitored namespaces: %v", err)
				return
			}
			monitoredNamespaces := make(map[string]bool)
			for _, ns := range monitoredNamespacesList {
				monitoredNamespaces[ns] = true
			}
			setMetrics(clientset, node.(string), monitoredNamespaces)
		}, ants.WithExpiryDuration(time.Duration(sampleInterval)*time.Second))

		defer p.Release()

		for {
			nodeSlice := Node.Set.ToSlice()

			for _, node := range nodeSlice {
				_ = p.Invoke(node)
			}

			time.Sleep(time.Duration(sampleInterval) * time.Second)
		}
	}()

	go getMetrics(clientset)

	http.Handle("/metrics", promhttp.Handler())
	log.Info().Msg(fmt.Sprintf("Starting server listening on :%s", port))
	err = http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Listener Failed : %s\n", err.Error()))
		panic(err.Error())
	}
}
