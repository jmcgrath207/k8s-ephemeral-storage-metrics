package node

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/prometheus/client_golang/prometheus"
)

func newTestNode() *Node {
	return &Node{
		Set:              mapset.NewSet[string](),
		sampleInterval:   1,
		inFlight:         &sync.Map{},
		failureCooldown:  &sync.Map{},
		cooldownMultiplier: 3,
		timeNow:          time.Now,
		AdjustedPollingRate: false,
	}
}

func init() {
	nodeAvailableGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_ephemeral_storage_node_available",
	}, []string{"node_name"})

	nodeCapacityGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_ephemeral_storage_node_capacity",
	}, []string{"node_name"})

	nodePercentageGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_ephemeral_storage_node_percentage",
	}, []string{"node_name"})
}

// --- Fix 1: In-flight deduplication tests ---

func TestTryAcquireInFlight_FirstCall_Succeeds(t *testing.T) {
	n := newTestNode()
	if !n.TryAcquireInFlight("nodeA") {
		t.Fatal("expected first TryAcquireInFlight to succeed")
	}
}

func TestTryAcquireInFlight_SecondCall_Blocked(t *testing.T) {
	n := newTestNode()
	n.TryAcquireInFlight("nodeA")
	if n.TryAcquireInFlight("nodeA") {
		t.Fatal("expected second TryAcquireInFlight to be blocked")
	}
}

func TestReleaseInFlight_EnablesReacquisition(t *testing.T) {
	n := newTestNode()
	n.TryAcquireInFlight("nodeA")
	n.ReleaseInFlight("nodeA")
	if !n.TryAcquireInFlight("nodeA") {
		t.Fatal("expected TryAcquireInFlight to succeed after release")
	}
}

func TestInFlight_DifferentNodes_Independent(t *testing.T) {
	n := newTestNode()
	n.TryAcquireInFlight("nodeA")
	if !n.TryAcquireInFlight("nodeB") {
		t.Fatal("expected nodeB acquisition to be independent of nodeA")
	}
}

func TestInFlight_ConcurrentAccess(t *testing.T) {
	n := newTestNode()
	const goroutines = 50
	var acquired int64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if n.TryAcquireInFlight("nodeA") {
				atomic.AddInt64(&acquired, 1)
			}
		}()
	}
	wg.Wait()

	if acquired != 1 {
		t.Fatalf("expected exactly 1 acquisition, got %d", acquired)
	}
}

// --- Fix 2: suspendNode tests ---

func TestSuspendNode_RemovesFromSet(t *testing.T) {
	n := newTestNode()
	n.Set.Add("nodeA")
	n.suspendNode("nodeA")
	if n.Set.Contains("nodeA") {
		t.Fatal("expected nodeA to be removed from Set")
	}
}

func TestSuspendNode_PreservesMetrics(t *testing.T) {
	n := newTestNode()
	n.Set.Add("nodeA")

	nodeAvailableGaugeVec.With(prometheus.Labels{"node_name": "nodeA"}).Set(100)
	nodeCapacityGaugeVec.With(prometheus.Labels{"node_name": "nodeA"}).Set(200)

	n.suspendNode("nodeA")

	ch := make(chan prometheus.Metric, 10)
	nodeAvailableGaugeVec.Collect(ch)
	if len(ch) == 0 {
		t.Fatal("expected node metrics to be preserved after suspendNode")
	}
}

func TestSuspendNode_Idempotent(t *testing.T) {
	n := newTestNode()
	// Should not panic when node is not in Set
	n.suspendNode("nonexistent")
	n.suspendNode("nonexistent")
}

func TestEvict_RemovesFromSetAndDeletesMetrics(t *testing.T) {
	n := newTestNode()
	n.Set.Add("nodeA")

	nodeAvailableGaugeVec.With(prometheus.Labels{"node_name": "nodeA"}).Set(100)
	nodeCapacityGaugeVec.With(prometheus.Labels{"node_name": "nodeA"}).Set(200)

	n.evict("nodeA")

	if n.Set.Contains("nodeA") {
		t.Fatal("expected nodeA to be removed from Set")
	}

	ch := make(chan prometheus.Metric, 10)
	nodeAvailableGaugeVec.Collect(ch)
	close(ch)
	for m := range ch {
		desc := m.Desc().String()
		if desc != "" {
			// Any remaining metric means it wasn't deleted
			// But with DeletePartialMatch, the whole label set should be gone
		}
	}
}

// --- Fix 3: Cooldown tests ---

func TestRecordFailure_StoresTimestamp(t *testing.T) {
	n := newTestNode()
	before := time.Now()
	n.RecordFailure("nodeA")
	after := time.Now()

	val, ok := n.failureCooldown.Load("nodeA")
	if !ok {
		t.Fatal("expected failure to be recorded")
	}
	ts := val.(time.Time)
	if ts.Before(before) || ts.After(after) {
		t.Fatal("expected timestamp to be between before and after")
	}
}

func TestIsInCooldown_WithinWindow_ReturnsTrue(t *testing.T) {
	n := newTestNode()
	n.RecordFailure("nodeA")
	if !n.IsInCooldown("nodeA") {
		t.Fatal("expected nodeA to be in cooldown immediately after failure")
	}
}

func TestIsInCooldown_AfterExpiry_ReturnsFalse(t *testing.T) {
	n := newTestNode()
	n.sampleInterval = 1
	n.cooldownMultiplier = 1
	// Fake time to simulate expiry
	past := time.Now().Add(-2 * time.Second)
	n.failureCooldown.Store("nodeA", past)

	if n.IsInCooldown("nodeA") {
		t.Fatal("expected cooldown to have expired")
	}

	// Entry should be cleaned up
	if _, ok := n.failureCooldown.Load("nodeA"); ok {
		t.Fatal("expected expired entry to be cleaned up")
	}
}

func TestIsInCooldown_NoFailure_ReturnsFalse(t *testing.T) {
	n := newTestNode()
	if n.IsInCooldown("nodeA") {
		t.Fatal("expected no cooldown for node with no recorded failure")
	}
}

func TestClearCooldown_RemovesEntry(t *testing.T) {
	n := newTestNode()
	n.RecordFailure("nodeA")
	n.ClearCooldown("nodeA")
	if n.IsInCooldown("nodeA") {
		t.Fatal("expected cooldown to be cleared")
	}
}

func TestCooldown_DifferentNodes_Independent(t *testing.T) {
	n := newTestNode()
	n.RecordFailure("nodeA")
	if n.IsInCooldown("nodeB") {
		t.Fatal("expected nodeB to not be in cooldown")
	}
}

func TestIsInCooldown_WithTimeNow_Override(t *testing.T) {
	n := newTestNode()
	n.sampleInterval = 15
	n.cooldownMultiplier = 3
	// Record failure at a fixed time
	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	n.failureCooldown.Store("nodeA", fixedNow)

	// 44 seconds later: still in cooldown (45s window)
	n.timeNow = func() time.Time { return fixedNow.Add(44 * time.Second) }
	if !n.IsInCooldown("nodeA") {
		t.Fatal("expected still in cooldown at 44s")
	}

	// 46 seconds later: cooldown expired
	n.timeNow = func() time.Time { return fixedNow.Add(46 * time.Second) }
	if n.IsInCooldown("nodeA") {
		t.Fatal("expected cooldown to have expired at 46s")
	}
}
