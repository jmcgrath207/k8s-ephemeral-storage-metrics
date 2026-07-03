package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/node"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/pod"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

func TestSetMetrics_WithApiServer(t *testing.T) {
	// ponytail: httptest-based integration test for setMetrics
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"node": {"nodeName": "test-node"},
			"pods": [{
				"podRef": {"name": "pod-1", "namespace": "ns-1"},
				"ephemeral-storage": {
					"usedBytes": 100,
					"availableBytes": 900,
					"capacityBytes": 1000,
					"inodes": 1000,
					"inodesFree": 500,
					"inodesUsed": 500
				},
				"containers": [],
				"volume": []
			}]
		}`))
	}))
	defer server.Close()

	config := &rest.Config{
		Host:    server.URL,
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &schema.GroupVersion{Group: "", Version: "v1"},
			NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		},
	}
	restClient, _ := rest.RESTClientFor(config)
	clientset := kubernetes.New(restClient)

	origClient := dev.Clientset
	dev.Clientset = clientset
	defer func() { dev.Clientset = origClient }()

	os.Setenv("CURRENT_NODE_NAME", "test-node")
	os.Setenv("EPHEMERAL_STORAGE_POD_USAGE", "true")
	defer func() {
		os.Unsetenv("CURRENT_NODE_NAME")
		os.Unsetenv("EPHEMERAL_STORAGE_POD_USAGE")
	}()

	Node = node.NewCollector(15)
	Pod = pod.NewCollector(15)

	// setMetrics should succeed without panicking
	setMetrics("test-node")
}
