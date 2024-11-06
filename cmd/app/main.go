package main

import (
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
	"net/http"
	"strconv"
	"time"
)

var (
	sampleInterval     int64
	sampleIntervalMill int64
	Node               node.Node
	Pod                pod.Collector
)

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

func setMetrics(nodeName string) {

	var data ephemeralStorageMetrics

	start := time.Now()

	content, err := Node.Query(nodeName)
	// Skip node query if there is an error.
	if err != nil {
		return
	}

	log.Debug().Msg(fmt.Sprintf("Fetched proxy stats from node : %s", nodeName))
	_ = json.Unmarshal(content, &data)

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
		Node.SetMetrics(nodeName, availableBytes, capacityBytes)
		Pod.SetMetrics(podName, podNamespace, nodeName, usedBytes, availableBytes, capacityBytes, p.Volumes)
	}

	adjustTime := sampleIntervalMill - time.Now().Sub(start).Milliseconds()
	if adjustTime <= 0.0 {
		log.Error().Msgf("Node %s: Polling Rate could not keep up. Adjust your Interval to a higher number than %d seconds", nodeName, sampleInterval)
	}
	if Node.AdjustedPollingRate {
		node.AdjustedPollingRateGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(float64(adjustTime))
	}

}

func getMetrics() {

	Pod.WaitGroup.Wait()

	p, _ := ants.NewPoolWithFunc(Node.MaxNodeQueryConcurrency, func(node interface{}) {
		setMetrics(node.(string))
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

	// Shared Vars
	pprofEnabled, _ := strconv.ParseBool(dev.GetEnv("PPROF", "false"))
	sampleInterval, _ = strconv.ParseInt(dev.GetEnv("SCRAPE_INTERVAL", "15"), 10, 64)
	sampleIntervalMill = sampleInterval * 1000

	dev.SetLogger()
	dev.SetK8sClient()
	Node = node.NewCollector(sampleInterval)
	Pod = pod.NewCollector(sampleInterval)

	if pprofEnabled {
		go dev.EnablePprof()
	}
	go getMetrics()
	http.Handle("/metrics", promhttp.Handler())
	log.Info().Msg(fmt.Sprintf("Starting server listening on :%s", port))
	err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Listener Failed : %s\n", err.Error()))
		panic(err.Error())
	}
}
