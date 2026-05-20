package node

import (
	"time"

	"github.com/rs/zerolog/log"
)

func (n *Node) TryAcquireInFlight(nodeName string) bool {
	_, loaded := n.inFlight.LoadOrStore(nodeName, struct{}{})
	return !loaded
}

func (n *Node) ReleaseInFlight(nodeName string) {
	n.inFlight.Delete(nodeName)
}

func (n *Node) RecordFailure(nodeName string) {
	n.failureCooldown.Store(nodeName, n.timeNow())
}

func (n *Node) IsInCooldown(nodeName string) bool {
	val, ok := n.failureCooldown.Load(nodeName)
	if !ok {
		return false
	}
	failedAt := val.(time.Time)
	cooldownDuration := time.Duration(n.cooldownMultiplier*n.sampleInterval) * time.Second
	if n.timeNow().Sub(failedAt) < cooldownDuration {
		return true
	}
	n.failureCooldown.Delete(nodeName)
	return false
}

func (n *Node) ClearCooldown(nodeName string) {
	n.failureCooldown.Delete(nodeName)
}

func (n *Node) logCooldownSkip(nodeName string) {
	log.Debug().Msgf("Node %s still in failure cooldown, skipping re-add", nodeName)
}
