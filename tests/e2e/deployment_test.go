package e2e

import (
	"fmt"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var httpClient *http.Client

func requestPrometheusString() string {

	var resp *http.Response

	req, err := http.NewRequest("GET", "http://127.0.0.1:9100/metrics", nil)
	if err != nil {
		panic(err)
	}
	for {
		// Send the request
		resp, err = httpClient.Do(req)
		if err != nil {
			time.Sleep(time.Second * 1)
			continue
		}
		break

	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	output := string(body)
	err = resp.Body.Close()
	if err != nil {
		panic(err)
	}
	return output
}

func CheckValues(ifFound map[string]bool) int {
	var status int
	for _, value := range ifFound {
		if value {
			status = 1
			continue
		}
		status = 0
	}
	return status
}

func checkPrometheus(checkSlice []string) {
	var status int
	timeout := time.Second * 180
	startTime := time.Now()
	ifFound := make(map[string]bool)

	// Add values to IFound Map
	for _, a := range checkSlice {
		ifFound[a] = false
	}

	for {
		elapsed := time.Since(startTime)
		if elapsed >= timeout {
			break
		}
		status = 0
		time.Sleep(1 * time.Second)

		output := requestPrometheusString()

		// Print the response
		for _, a := range checkSlice {
			if ifFound[a] {
				continue
			}
			if strings.Contains(output, a) {
				ifFound[a] = true
				status = CheckValues(ifFound)
			}
		}

		if status == 1 {
			break
		}

	}
	for key, value := range ifFound {
		if value {
			ginkgo.GinkgoWriter.Printf("\nFound value: [ %v ] in prometheus exporter\n", key)
			continue
		}
		ginkgo.GinkgoWriter.Printf("\nDid not find value: [ %v ] in prometheus exporter\n", key)
	}

	gomega.Expect(status).Should(gomega.Equal(1))

}

func WatchNodePercentage() {
	status := 0
	re := regexp.MustCompile(`ephemeral_storage_node_percentage\{node_name="ephemeral-metrics-cluster-control-plane"}\s+(.+)`)
	output := requestPrometheusString()
	match := re.FindAllStringSubmatch(output, -1)
	floatValue, _ := strconv.ParseFloat(match[0][1], 64)
	if floatValue < 100.0 {
		status = 1
	}
	gomega.Expect(status).Should(gomega.Equal(1))

}
func WatchPollingRate(pollRateUpper float64, pollingRateLower float64, timeout time.Duration) {
	status := 0
	startTime := time.Now()
	re := regexp.MustCompile(`ephemeral_storage_adjusted_polling_rate\{node_name="ephemeral-metrics-cluster-control-plane"}\s+(.+)`)
	for {
		elapsed := time.Since(startTime)
		if elapsed >= timeout {
			ginkgo.GinkgoWriter.Printf("Watch for rate polling timed out")
			break
		}
		output := requestPrometheusString()
		match := re.FindAllStringSubmatch(output, -1)
		floatValue, _ := strconv.ParseFloat(match[0][1], 64)
		if pollRateUpper >= floatValue && pollingRateLower <= floatValue {
			status = 1
			break
		}
		time.Sleep(5 * time.Second)
	}

	gomega.Expect(status).Should(gomega.Equal(1))

}

func WatchEphemeralPodSize(podname string, sizeChange float64, timeout time.Duration) {
	// Watch Prometheus Metrics until the ephemeral storage shrinks or grows to a certain sizeChange.
	startTime := time.Now()
	status := 0
	var initSize float64
	for {
		elapsed := time.Since(startTime)
		if elapsed >= timeout {
			ginkgo.GinkgoWriter.Printf("Watch for metrics has timed out for pod %s", podname)
			break
		}
		output := requestPrometheusString()
		re := regexp.MustCompile(fmt.Sprintf(`ephemeral_storage_pod_usage.+pod_name="%s.+\}\s(.+)`, podname))
		match := re.FindAllStringSubmatch(output, 2)
		floatValue, _ := strconv.ParseFloat(match[0][1], 64)
		if initSize == 0.0 {
			initSize = floatValue

		}
		if strings.Contains(podname, "grow") {
			if floatValue-initSize >= sizeChange {
				ginkgo.GinkgoWriter.Printf("\nSuccess: \n\tPod Name: %s \n\tTargetSize: %f \n\tPodSize: %f", podname, initSize+sizeChange, floatValue)
				status = 1
				break
			}
			ginkgo.GinkgoWriter.Printf("\nPending: \n\tPod Name: %s \n\tTargetSize: %f \n\tPodSize: %f", podname, initSize+sizeChange, floatValue)
			time.Sleep(time.Second * 5)
		} else if strings.Contains(podname, "shrink") {
			if initSize-floatValue >= sizeChange {
				ginkgo.GinkgoWriter.Printf("\nSuccess: \n\tPod Name: %s \n\tTargetSize: %f \n\tPodSize: %f", podname, initSize-sizeChange, floatValue)
				status = 1
				break
			}
			ginkgo.GinkgoWriter.Printf("\nPending: \n\tPod Name: %s \n\tTargetSize: %f \n\tPodSize: %f", podname, initSize-sizeChange, floatValue)
			time.Sleep(time.Second * 5)
		}

	}
	gomega.Expect(status).Should(gomega.Equal(1))

}

var _ = ginkgo.Describe("Test Metrics\n", func() {

	ginkgo.Context("Observe labels\n", func() {
		ginkgo.Specify("\nReturn A Record IP addresses and Proxy IP address", func() {
			var checkSlice []string
			checkSlice = append(checkSlice, "ephemeral_storage_pod_usage",
				"ephemeral_storage_node_available",
				"ephemeral_storage_node_capacity",
				"ephemeral_storage_node_percentage",
				"pod_name=\"k8s-ephemeral-storage", "ephemeral_storage_adjusted_polling_rate",
				"node_name=\"ephemeral-metrics-cluster-worker", "node_name=\"ephemeral-metrics-cluster-control-plane")
			checkPrometheus(checkSlice)
		})
	})
	ginkgo.Context("Observe change in storage metrics\n", func() {
		ginkgo.Specify("\nWatch Pod grow to 100000 Bytes", func() {
			WatchEphemeralPodSize("grow-test", 100000, time.Second*90)
		})
		ginkgo.Specify("\nWatch Pod shrink to 100000 Bytes", func() {
			WatchEphemeralPodSize("shrink-test", 100000, time.Second*90)
		})
	})
	ginkgo.Context("Test Polling speed\n", func() {
		ginkgo.Specify("\nMake sure Adjusted Poll rate is between 5000 - 4000 ms", func() {
			WatchPollingRate(5000.0, 4000.0, time.Second*90)
		})
	})
	ginkgo.Context("Test ephemeral_storage_node_percentage\n", func() {
		ginkgo.Specify("\nMake sure percentage is not over 100", func() {
			WatchNodePercentage()
		})
	})
})

func TestDeployments(t *testing.T) {
	// https://onsi.github.io/ginkgo/#ginkgo-and-gomega-patterns
	httpClient = &http.Client{}

	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Test Metrics")
}
