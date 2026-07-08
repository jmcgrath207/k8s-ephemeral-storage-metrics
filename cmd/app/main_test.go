package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const sampleStatsSummary = `{
  "node": {"nodeName": "test-node-01"},
  "pods": [
    {
      "podRef": {"name": "pod-a", "namespace": "ns-a"},
      "ephemeral-storage": {
        "availableBytes": 8000000,
        "capacityBytes": 10000000,
        "usedBytes": 2000000,
        "inodes": 1000,
        "inodesFree": 800,
        "inodesUsed": 200
      },
      "volume": [
        {"name": "empty", "availableBytes": 1000, "capacityBytes": 2000, "usedBytes": 500, "inodes": 100, "inodesFree": 50, "inodesUsed": 50}
      ]
    },
    {
      "podRef": {"name": "pod-b", "namespace": "ns-b"},
      "ephemeral-storage": {
        "availableBytes": 0,
        "capacityBytes": 0,
        "usedBytes": 0,
        "inodes": 0,
        "inodesFree": 0,
        "inodesUsed": 0
      }
    }
  ]
}`

func TestEphemeralStorageMetricsUnmarshal(t *testing.T) {
	var data ephemeralStorageMetrics
	if err := json.Unmarshal([]byte(sampleStatsSummary), &data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if data.Node.NodeName != "test-node-01" {
		t.Errorf("node name = %q, want test-node-01", data.Node.NodeName)
	}
	if len(data.Pods) != 2 {
		t.Fatalf("pods len = %d, want 2", len(data.Pods))
	}
	first := data.Pods[0]
	if first.PodRef.Name != "pod-a" || first.PodRef.Namespace != "ns-a" {
		t.Errorf("pod ref = %+v", first.PodRef)
	}
	if first.EphemeralStorage.UsedBytes != 2000000 {
		t.Errorf("used bytes = %v, want 2000000", first.EphemeralStorage.UsedBytes)
	}
	if first.EphemeralStorage.CapacityBytes != 10000000 {
		t.Errorf("capacity bytes = %v, want 10000000", first.EphemeralStorage.CapacityBytes)
	}
	if first.EphemeralStorage.AvailableBytes != 8000000 {
		t.Errorf("available bytes = %v, want 8000000", first.EphemeralStorage.AvailableBytes)
	}
	if first.EphemeralStorage.Inodes != 1000 {
		t.Errorf("inodes = %v, want 1000", first.EphemeralStorage.Inodes)
	}
	if first.EphemeralStorage.InodesFree != 800 {
		t.Errorf("inodes free = %v, want 800", first.EphemeralStorage.InodesFree)
	}
	if first.EphemeralStorage.InodesUsed != 200 {
		t.Errorf("inodes used = %v, want 200", first.EphemeralStorage.InodesUsed)
	}
	if len(first.Volumes) != 1 {
		t.Fatalf("volumes len = %d, want 1", len(first.Volumes))
	}
	if first.Volumes[0].Name != "empty" {
		t.Errorf("volume name = %q, want empty", first.Volumes[0].Name)
	}
	if first.Volumes[0].UsedBytes != 500 {
		t.Errorf("volume used = %v, want 500", first.Volumes[0].UsedBytes)
	}
	second := data.Pods[1]
	if second.PodRef.Name != "pod-b" {
		t.Errorf("second pod name = %q, want pod-b", second.PodRef.Name)
	}
	if second.EphemeralStorage.UsedBytes != 0 {
		t.Errorf("second pod used bytes = %v, want 0", second.EphemeralStorage.UsedBytes)
	}
}

func TestEphemeralStorageMetricsUnmarshalMissingFields(t *testing.T) {
	const minimal = `{"node": {"nodeName": "n1"}, "pods": [{"podRef": {"name": "p", "namespace": "ns"}}]}`
	var data ephemeralStorageMetrics
	if err := json.Unmarshal([]byte(minimal), &data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if data.Node.NodeName != "n1" {
		t.Errorf("node name = %q, want n1", data.Node.NodeName)
	}
	if len(data.Pods) != 1 {
		t.Fatalf("pods len = %d, want 1", len(data.Pods))
	}
	p := data.Pods[0]
	if p.PodRef.Name != "p" || p.PodRef.Namespace != "ns" {
		t.Errorf("pod ref = %+v", p.PodRef)
	}
	if p.EphemeralStorage.UsedBytes != 0 {
		t.Errorf("expected zero ephemeral storage, got %+v", p.EphemeralStorage)
	}
	if len(p.Volumes) != 0 {
		t.Errorf("expected no volumes, got %d", len(p.Volumes))
	}
	if len(p.Containers) != 0 {
		t.Errorf("expected no containers, got %d", len(p.Containers))
	}
}

func TestEphemeralStorageMetricsUnmarshalMalformed(t *testing.T) {
	const bad = `{"node": "not-an-object"}`
	var data ephemeralStorageMetrics
	err := json.Unmarshal([]byte(bad), &data)
	if err == nil {
		t.Fatal("expected unmarshal error for malformed input, got nil")
	}
	if !strings.Contains(err.Error(), "cannot unmarshal") && !strings.Contains(err.Error(), "invalid character") {
		t.Logf("got error: %v (acceptable)", err)
	}
}

func TestEphemeralStorageMetricsUnmarshalEmpty(t *testing.T) {
	var data ephemeralStorageMetrics
	if err := json.Unmarshal([]byte(`{}`), &data); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	if data.Node.NodeName != "" {
		t.Errorf("node name = %q, want empty", data.Node.NodeName)
	}
	if len(data.Pods) != 0 {
		t.Errorf("pods len = %d, want 0", len(data.Pods))
	}
}

func TestEphemeralStorageMetricsUnmarshalContainers(t *testing.T) {
	const withContainers = `{
	  "node": {"nodeName": "n1"},
	  "pods": [{
	    "podRef": {"name": "p", "namespace": "ns"},
	    "ephemeral-storage": {"usedBytes": 100, "availableBytes": 200, "capacityBytes": 300, "inodes": 0, "inodesFree": 0, "inodesUsed": 0},
	    "containers": [
	      {
	        "name": "c1",
	        "rootfs": {"availableBytes": 100, "capacityBytes": 200, "usedBytes": 50, "inodes": 0, "inodesFree": 0, "inodesUsed": 0},
	        "logs": {"availableBytes": 50, "capacityBytes": 100, "usedBytes": 25, "inodes": 0, "inodesFree": 0, "inodesUsed": 0}
	      }
	    ]
	  }]
	}`
	var data ephemeralStorageMetrics
	if err := json.Unmarshal([]byte(withContainers), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(data.Pods) != 1 {
		t.Fatalf("pods = %d, want 1", len(data.Pods))
	}
	containers := data.Pods[0].Containers
	if len(containers) != 1 {
		t.Fatalf("containers = %d, want 1", len(containers))
	}
	c := containers[0]
	if c.Name != "c1" {
		t.Errorf("container name = %q, want c1", c.Name)
	}
	if c.Rootfs.UsedBytes != 50 {
		t.Errorf("rootfs used = %v, want 50", c.Rootfs.UsedBytes)
	}
	if c.Rootfs.CapacityBytes != 200 {
		t.Errorf("rootfs capacity = %v, want 200", c.Rootfs.CapacityBytes)
	}
	if c.Logs.UsedBytes != 25 {
		t.Errorf("logs used = %v, want 25", c.Logs.UsedBytes)
	}
	if c.Logs.AvailableBytes != 50 {
		t.Errorf("logs available = %v, want 50", c.Logs.AvailableBytes)
	}
}
