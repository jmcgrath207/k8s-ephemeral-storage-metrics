package e2e

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var httpClient *http.Client

type getPodSize func(podName string) float64

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

func checkPrometheus(checkSlice []string, inverse bool) {
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
			if inverse && !strings.Contains(output, a) {
				ifFound[a] = true

			} else if !inverse && strings.Contains(output, a) {
				ifFound[a] = true
			}

			status = CheckValues(ifFound)
		}

		if status == 1 {
			break
		}

	}
	for key, value := range ifFound {
		if value {
			if inverse {
				ginkgo.GinkgoWriter.Printf("\nDid not find value: [ %v ] in prometheus exporter\n", key)
			} else {
				ginkgo.GinkgoWriter.Printf("\nFound value: [ %v ] in prometheus exporter\n", key)
			}
			continue
		}
		ginkgo.GinkgoWriter.Printf("\nDid not find value: [ %v ] in prometheus exporter\n", key)
	}

	gomega.Expect(status).Should(gomega.Equal(1))

}

func WatchContainerPercentage() {
	status := 0
	re := regexp.MustCompile(`ephemeral_storage_container_limit_percentage{container="grow-test",node_name="minikube".+,pod_namespace="ephemeral-metrics",source="container"}\s+(.+)`)
	timeout := time.Second * 180
	startTime := time.Now()
	for {
		if time.Since(startTime) >= timeout {
			break
		}
		output := requestPrometheusString()
		match := re.FindAllStringSubmatch(output, -1)
		if match != nil {
			floatValue, _ := strconv.ParseFloat(match[0][1], 64)
			if floatValue < 100.0 {
				status = 1
				break
			}
		}
		time.Sleep(time.Second)
	}
	gomega.Expect(status).Should(gomega.Equal(1))

}

func WatchContainerVolumePercentage() {
	status := 0
	re := regexp.MustCompile(`ephemeral_storage_container_volume_limit_percentage{container="shrink-test",mount_path="\/cache".+volume_name="cache-volume-1"}\s+(.+)`)
	timeout := time.Second * 180
	startTime := time.Now()
	for {
		if time.Since(startTime) >= timeout {
			break
		}
		output := requestPrometheusString()
		match := re.FindAllStringSubmatch(output, -1)
		if match != nil {
			floatValue, _ := strconv.ParseFloat(match[0][1], 64)
			if floatValue < 100.0 {
				status = 1
				break
			}
		}
		time.Sleep(time.Second)
	}
	gomega.Expect(status).Should(gomega.Equal(1))

}

func WatchNodePercentage() {
	status := 0
	re := regexp.MustCompile(`ephemeral_storage_node_percentage\{node_name="minikube"}\s+(.+)`)
	timeout := time.Second * 180
	startTime := time.Now()
	for {
		if time.Since(startTime) >= timeout {
			break
		}
		output := requestPrometheusString()
		match := re.FindAllStringSubmatch(output, -1)
		if match != nil {
			floatValue, _ := strconv.ParseFloat(match[0][1], 64)
			if floatValue < 100.0 {
				status = 1
				break
			}
		}
		time.Sleep(time.Second)
	}
	gomega.Expect(status).Should(gomega.Equal(1))

}
func WatchPollingRate(pollRateUpper float64, pollingRateLower float64, timeout time.Duration) {
	var currentPollRate float64
	status := 0
	startTime := time.Now()
	re := regexp.MustCompile(`ephemeral_storage_adjusted_polling_rate\{node_name="minikube"}\s+(.+)`)
	for {
		elapsed := time.Since(startTime)
		if elapsed >= timeout {
			ginkgo.GinkgoWriter.Printf("\nFailed: \n\tephemeral_storage_adjusted_polling_rate: %f\n", currentPollRate)
			break
		}
		output := requestPrometheusString()
		match := re.FindAllStringSubmatch(output, -1)
		currentPollRate, _ = strconv.ParseFloat(match[0][1], 64)
		if pollRateUpper >= currentPollRate && pollingRateLower <= currentPollRate {
			status = 1
			break
		}
		time.Sleep(5 * time.Second)
	}

	gomega.Expect(status).Should(gomega.Equal(1))
	ginkgo.GinkgoWriter.Printf("\nSuccess: \n\tephemeral_storage_adjusted_polling_rate: %f\n", currentPollRate)

}

func getPodUsageSize(podName string) float64 {
	output := requestPrometheusString()
	re := regexp.MustCompile(fmt.Sprintf(`ephemeral_storage_pod_usage.+pod_name="%s.+\}\s(.+)`, podName))
	match := re.FindAllStringSubmatch(output, 2)
	if match == nil {
		return 0
	}
	currentPodSize, _ := strconv.ParseFloat(match[0][1], 64)
	return currentPodSize
}

func getContainerLimitPercentage(podName string) float64 {
	output := requestPrometheusString()
	re := regexp.MustCompile(fmt.Sprintf(`ephemeral_storage_container_limit_percentage.+pod_name="%s.+\}\s(.+)`, podName))
	match := re.FindAllStringSubmatch(output, 2)
	if match == nil {
		return 0
	}
	currentPodSize, _ := strconv.ParseFloat(match[0][1], 64)
	return currentPodSize
}

func getContainerVolumeLimitPercentage(podName string) float64 {
	output := requestPrometheusString()
	re := regexp.MustCompile(
		fmt.Sprintf(`ephemeral_storage_container_volume_limit_percentage.+container="%s",mount_path="\/cache".+\}\s(.+)`,
			podName))
	match := re.FindAllStringSubmatch(output, 2)
	if match == nil {
		return 0
	}
	currentPodSize, _ := strconv.ParseFloat(match[0][1], 64)
	return currentPodSize
}

func getContainerVolumeUsage(podName string) float64 {
	output := requestPrometheusString()
	re := regexp.MustCompile(
		fmt.Sprintf(`ephemeral_storage_container_volume_usage.+container="%s",mount_path="\/cache".+\}\s(.+)`,
			podName))
	match := re.FindAllStringSubmatch(output, 2)
	if match == nil {
		return 0
	}
	currentPodSize, _ := strconv.ParseFloat(match[0][1], 64)
	return currentPodSize
}

func getContainerRootfsUsedBytes(podName string) float64 {
	output := requestPrometheusString()
	re := regexp.MustCompile(
		fmt.Sprintf(`ephemeral_storage_container_rootfs_used_bytes.+container="%s".+\}\s(.+)`, podName))
	match := re.FindAllStringSubmatch(output, 2)
	if match == nil {
		return 0
	}
	currentPodSize, _ := strconv.ParseFloat(match[0][1], 64)
	return currentPodSize
}

func WatchEphemeralSize(podName string, desiredSizeChange float64, timeout time.Duration, getPodSize getPodSize) {
	// Watch Prometheus Metrics until the ephemeral storage shrinks or grows to a certain desiredSizeChange.
	var currentPodSize float64
	var targetSizeChange float64

	startTime := time.Now()
	status := 0
	initSize := getPodSize(podName)
	if podName == "grow-test" {
		targetSizeChange = initSize + desiredSizeChange
	} else if podName == "shrink-test" {
		targetSizeChange = initSize - desiredSizeChange
	}

	for {
		elapsed := time.Since(startTime)
		if elapsed >= timeout {
			ginkgo.GinkgoWriter.Printf("\nWatch for metrics has timed out for pod %s", podName)
			break
		}
		currentPodSize = getPodSize(podName)

		if podName == "grow-test" && currentPodSize >= targetSizeChange {
			status = 1

		} else if podName == "shrink-test" && currentPodSize <= targetSizeChange {
			status = 1
		}

		if status == 1 {
			ginkgo.GinkgoWriter.Printf("\nSuccess: \n\tPod name: %s \n\tTarget size: %f \n\tCurrent size: %f\n", podName, targetSizeChange, currentPodSize)
			break
		}

		ginkgo.GinkgoWriter.Printf("\nPending: \n\tPod name: %s \n\tTarget size: %f \n\tCurrent size: %f\n", podName, targetSizeChange, currentPodSize)
		time.Sleep(time.Second * 5)

	}
	gomega.Expect(status).Should(gomega.Equal(1))

}

func scaleUp() {
	cmd := exec.Command("make", "minikube_scale_up")
	cmd.Dir = "../.."

	_, err := cmd.Output()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

}

func scaleDown() {
	cmd := exec.Command("make", "minikube_scale_down")
	cmd.Dir = "../.."

	_, err := cmd.Output()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

}

func deployManyPods() {
	cmd := exec.Command("make", "deploy_many_pods")
	cmd.Dir = "../.."

	_, err := cmd.Output()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

}

func destroyManyPods() {
	cmd := exec.Command("make", "destroy_many_pods")
	cmd.Dir = "../.."

	_, err := cmd.Output()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

}

func nodeDisconnect() {
	cmd := exec.Command("make", "minikube_node2_stop")
	cmd.Dir = "../.."

	_, err := cmd.Output()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

}

func nodeReconnect() {
	cmd := exec.Command("make", "minikube_node2_start")
	cmd.Dir = "../.."

	_, err := cmd.Output()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

}

var scalingCheckSlice = []string{
	"node_name=\"minikube-m02",
	"ephemeral_storage_container_limit_percentage{container=\"kube-proxy\",node_name=\"minikube-m02\"",
	"ephemeral_storage_inodes{node_name=\"minikube-m02\"",
	"ephemeral_storage_inodes_free{node_name=\"minikube-m02\"",
	"ephemeral_storage_inodes_used{node_name=\"minikube-m02\"",
}

var _ = ginkgo.BeforeSuite(func() {
	scaleUp()
})

var _ = ginkgo.Describe("Test Metrics\n", func() {

	ginkgo.Context("Observe labels\n", func() {
		ginkgo.Specify("\nMake sure all metrics are in the exporter", func() {
			var checkSlice []string
			checkSlice = append(checkSlice, "ephemeral_storage_pod_usage",
				"pod_name=\"k8s-ephemeral-storage",
				"ephemeral_storage_adjusted_polling_rate",
				"node_name=\"minikube",
				"ephemeral_storage_node_available",
				"ephemeral_storage_node_capacity",
				"ephemeral_storage_node_percentage",
				"ephemeral_storage_container_limit_percentage",
				"ephemeral_storage_container_volume_limit_percentage",
				"ephemeral_storage_container_volume_usage",
				"ephemeral_storage_inodes",
				"ephemeral_storage_inodes_free",
				"ephemeral_storage_inodes_used",
				"ephemeral_storage_container_rootfs_used_bytes",
				"ephemeral_storage_container_rootfs_available_bytes",
				"ephemeral_storage_container_rootfs_capacity_bytes",
				"ephemeral_storage_container_logs_used_bytes",
				"ephemeral_storage_container_logs_available_bytes",
				"ephemeral_storage_container_logs_capacity_bytes",
				"ephemeral_storage_container_rootfs_usage_percentage",
				"ephemeral_storage_container_logs_usage_percentage",
				"ephemeral_storage_container_rootfs_inodes",
				"ephemeral_storage_container_rootfs_inodes_free",
				"ephemeral_storage_container_rootfs_inodes_used",
				"ephemeral_storage_container_logs_inodes",
				"ephemeral_storage_container_logs_inodes_free",
				"ephemeral_storage_container_logs_inodes_used",
			)
			checkPrometheus(checkSlice, false)
		})
	})
	ginkgo.Context("Test Polling speed\n", func() {
		ginkgo.Specify("\nMake sure Adjusted Poll rate is between 5000 - 4000 ms", func() {
			WatchPollingRate(5000.0, 4000.0, time.Second*90)
		})
	})
	ginkgo.Context("Observe change in ephemeral_storage_pod_usage metric\n", func() {
		ginkgo.Specify("\nWatch Pod grow to 100000 Bytes", func() {
			WatchEphemeralSize("grow-test", 100000, time.Second*180, getPodUsageSize)
		})
		ginkgo.Specify("\nWatch Pod shrink to 100000 Bytes", func() {
			// Shrinking of ephemeral_storage reflects slower from Node API up to 5 minutes.
			// Wait until it's reporting correctly, and start testing with the minimum of 11mb of data
			// since the shrink container adds 12mb then decrements 12k a second.
			// Ex. /api/v1/nodes/minikube/proxy/stats/summary
			for {
				currentPodSize := getPodUsageSize("shrink-test")
				if currentPodSize >= 11000000.0 {
					break
				}
				time.Sleep(time.Second * 5)
			}
			WatchEphemeralSize("shrink-test", 100000, time.Second*180, getPodUsageSize)
		})
	})
	ginkgo.Context("Observe change in ephemeral_storage_container_limit_percentage metric\n", func() {
		ginkgo.Specify("\nWatch Pod grow to 0.2 percent", func() {
			WatchEphemeralSize("grow-test", 0.2, time.Second*180, getContainerLimitPercentage)
		})
		ginkgo.Specify("\nWatch Pod shrink to 0.2 percent", func() {
			WatchEphemeralSize("shrink-test", 0.2, time.Second*180, getContainerLimitPercentage)
		})

	})
	ginkgo.Context("Observe change in ephemeral_storage_container_volume_limit_percentage metric\n", func() {
		ginkgo.Specify("\nWatch Pod grow to 0.2 percent", func() {
			WatchEphemeralSize("grow-test", 0.2, time.Second*180, getContainerVolumeLimitPercentage)
		})
		ginkgo.Specify("\nWatch Pod shrink to 0.2 percent", func() {
			WatchEphemeralSize("shrink-test", 0.2, time.Second*180, getContainerVolumeLimitPercentage)
		})
	})
	ginkgo.Context("Observe change in ephemeral_storage_container_volume_usage metric\n", func() {
		ginkgo.Specify("\nWatch Pod grow to 0.2 percent", func() {
			WatchEphemeralSize("grow-test", 100000, time.Second*180, getContainerVolumeUsage)
		})
		ginkgo.Specify("\nWatch Pod shrink to 0.2 percent", func() {
			WatchEphemeralSize("shrink-test", 100000, time.Second*180, getContainerVolumeUsage)
		})
	})
	ginkgo.Context("Observe change in ephemeral_storage_container_rootfs_used_bytes metric\n", func() {
		ginkgo.Specify("\nWatch Pod grow to 0.2 percent", func() {
			WatchEphemeralSize("grow-test", 100000, time.Second*180, getContainerRootfsUsedBytes)
		})
		ginkgo.Specify("\nWatch Pod shrink to 0.2 percent", func() {
			WatchEphemeralSize("shrink-test", 100000, time.Second*180, getContainerRootfsUsedBytes)
		})
	})
	ginkgo.Context("\nMake sure percentage is not over 100", func() {
		ginkgo.Specify("\nTest ephemeral_storage_node_percentage", func() {
			WatchNodePercentage()
		})
		ginkgo.Specify("\nTest ephemeral_storage_container_limit_percentage", func() {
			WatchContainerPercentage()
		})
		ginkgo.Specify("\nTest ephemeral_storage_container_volume_limit_percentage", func() {
			WatchContainerVolumePercentage()
		})
	})
	ginkgo.Context("Test Scaling\n", func() {
		ginkgo.Specify("\nVerify pods and nodes are found after scale up", func() {
			checkPrometheus(scalingCheckSlice, false)
		})
	})
	ginkgo.Context("Test Node Disconnect Inode Leak\n", func() {
		// Reconnect m02 after the disconnect test so subsequent contexts
		// (Scrape-Driven Eviction, Scale Down) see a healthy cluster.
		ginkgo.AfterEach(nodeReconnect)
		ginkgo.Specify("\nStop node m02, assert EvictPodByNode cleans inode metrics (regression for Bug 1)", func() {
			nodeDisconnect()
			// Regression: inode metrics should be ABSENT after node unreachable.
			// Bug 1 caused EvictPodByNode to skip inodesGaugeVec/inodesFreeGaugeVec/inodesUsedGaugeVec,
			// leaking these metrics. Bug 1 is now fixed.
			leakCheckSlice := []string{
				"ephemeral_storage_inodes{node_name=\"minikube-m02\"",
				"ephemeral_storage_inodes_free{node_name=\"minikube-m02\"",
				"ephemeral_storage_inodes_used{node_name=\"minikube-m02\"",
			}
			checkPrometheus(leakCheckSlice, true)
			// Sanity check: node-level metrics ABSENT after EvictPodByNode (proves evict ran).
			evictCheckSlice := []string{
				"ephemeral_storage_node_percentage{node_name=\"minikube-m02\"",
			}
			checkPrometheus(evictCheckSlice, true)
		})
	})
	ginkgo.Context("Test Scrape-Driven Eviction\n", func() {
		ginkgo.Specify("\nPod eviction: Deploy pods, verify metrics exist, delete pods, verify metrics are scraped-evicted", func() {
			// Deploy many pods to create metrics
			deployManyPods()

			// Verify metrics for many-pods namespace appear
			manyPodsCheckSlice := []string{
				"pod_namespace=\"many-pods\"",
			}
			checkPrometheus(manyPodsCheckSlice, false)

			// Delete the many-pods deployment
			destroyManyPods()

			// Wait for scrape-driven eviction: at interval=5s with tolerance=2, pods are evicted after ~15s.
			// Wait 90 seconds for safety margin.
			ginkgo.GinkgoWriter.Printf("\nWaiting 90 seconds for scrape-driven eviction to remove pod metrics...\n")
			time.Sleep(90 * time.Second)

			// Verify metrics for many-pods namespace are removed
			checkPrometheus(manyPodsCheckSlice, true)
		})
	})
	ginkgo.Context("Test Scale Down\n", func() {
		ginkgo.Specify("\nScale Down test to make sure pods and nodes are evicted", func() {
			nodeReconnect()
			scaleDown()
			checkPrometheus(scalingCheckSlice, true)
		})
	})
})

func TestDeployments(t *testing.T) {
	// https://onsi.github.io/ginkgo/#ginkgo-and-gomega-patterns
	httpClient = &http.Client{}

	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Test Metrics")
}
