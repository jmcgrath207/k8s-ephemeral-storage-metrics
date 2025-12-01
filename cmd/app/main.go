package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/node"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/pod"
	"github.com/panjf2000/ants/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
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
			Inodes         float64 `json:"inodes"`
			InodesFree     float64 `json:"inodesFree"`
			InodesUsed     float64 `json:"inodesUsed"`
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
		inodes := p.EphemeralStorage.Inodes
		inodesFree := p.EphemeralStorage.InodesFree
		inodesUsed := p.EphemeralStorage.InodesUsed
		if podNamespace == "" || (usedBytes == 0 && availableBytes == 0 && capacityBytes == 0 && inodes == 0 && inodesFree == 0 && inodesUsed == 0) {
			log.Warn().Msg(fmt.Sprintf("pod %s/%s on %s has no metrics on its ephemeral storage usage", podName, podNamespace, nodeName))
			continue
		}
		Node.SetMetrics(nodeName, availableBytes, capacityBytes)
		Pod.SetMetrics(podName, podNamespace, nodeName, usedBytes, availableBytes, capacityBytes, inodes, inodesFree, inodesUsed, p.Volumes)
	}

	adjustTime := sampleIntervalMill - time.Since(start).Milliseconds()
	if adjustTime <= 0.0 {
		log.Error().Msgf("Node %s: Polling Rate could not keep up. Adjust your Interval to a higher number than %d seconds", nodeName, sampleInterval)
	}
	if Node.AdjustedPollingRate {
		node.AdjustedPollingRateGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(float64(adjustTime))
	}
}

func getMetrics() {
	// Wait for pod initialization with a timeout to prevent deadlock
	// If initialization takes too long, log a warning and continue anyway
	initTimeout := time.Duration(sampleInterval*2) * time.Second
	initDone := make(chan struct{})

	go func() {
		Pod.WaitGroup.Wait()
		close(initDone)
	}()

	select {
	case <-initDone:
		log.Info().Msg("Pod initialization completed successfully")
	case <-time.After(initTimeout):
		log.Warn().Msgf("Pod initialization timed out after %v, continuing anyway. Metrics may be incomplete.", initTimeout)
	}

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
	readinessTimeoutSeconds, _ := strconv.Atoi(dev.GetEnv("READINESS_PROBE_TIMEOUT_SECONDS", "2"))
	readinessTimeout := time.Duration(readinessTimeoutSeconds) * time.Second

	dev.SetLogger()
	dev.SetK8sClient()
	Node = node.NewCollector(sampleInterval)
	Pod = pod.NewCollector(sampleInterval)

	if pprofEnabled {
		go dev.EnablePprof()
	}
	go getMetrics()

	// Health check endpoint for readiness probe - responds immediately
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Error().Err(err).Msg("Failed to write health check response")
		}
	})

	// Metrics endpoint with timing middleware to diagnose slow responses
	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		promhttp.Handler().ServeHTTP(w, r)
		duration := time.Since(start)

		if duration > readinessTimeout {
			log.Warn().
				Dur("duration", duration).
				Dur("timeout", readinessTimeout).
				Msg("Metrics endpoint took longer than readiness probe timeout")
		} else if duration > readinessTimeout/2 {
			log.Info().
				Dur("duration", duration).
				Dur("timeout", readinessTimeout).
				Msg("Metrics endpoint response time approaching timeout")
		}
	})
	http.Handle("/metrics", metricsHandler)
	log.Info().Msg(fmt.Sprintf("Starting server listening on :%s", port))
	err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	if err != nil {
		log.Error().Msg(fmt.Sprintf("Listener Failed : %s\n", err.Error()))
		panic(err.Error())
	}
}
