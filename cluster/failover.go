package cluster

import (
	"log"
	"sync"
	"time"
)

// FailoverManager handles node failures and ring updates
type FailoverManager struct {
	mu              sync.RWMutex
	ring            *HashRing
	heartbeat       *HeartbeatMonitor
	clientPool      *ClientPool
	localNodeID     string
	failedNodes     map[string]time.Time
	onRingChange    func()
	enabled         bool
}

func NewFailoverManager(ring *HashRing, heartbeat *HeartbeatMonitor, clientPool *ClientPool, localNodeID string) *FailoverManager {
	fm := &FailoverManager{
		ring:         ring,
		heartbeat:    heartbeat,
		clientPool:   clientPool,
		localNodeID:  localNodeID,
		failedNodes:  make(map[string]time.Time),
		enabled:      true,
	}

	// Register failure callback with heartbeat monitor
	if heartbeat != nil {
		heartbeat.OnFailure(fm.handleNodeFailure)
	}

	return fm
}

// handleNodeFailure is called when a node is detected as failed
func (fm *FailoverManager) handleNodeFailure(nodeID string) {
	if !fm.enabled {
		return
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Track failed node
	fm.failedNodes[nodeID] = time.Now()

	log.Printf("Handling failure for node %s", nodeID)

	// Remove from ring
	fm.ring.RemoveNode(nodeID)

	// Remove client connection
	nodes := fm.ring.GetNodes()
	for _, node := range nodes {
		if node.ID == nodeID {
			fm.clientPool.RemoveClient(node.Address)
			break
		}
	}

	log.Printf("Node %s removed from ring, %d nodes remaining", nodeID, len(fm.ring.GetNodes()))

	// Trigger ring change callback
	if fm.onRingChange != nil {
		go fm.onRingChange()
	}
}

// OnRingChange sets a callback for when the ring changes
func (fm *FailoverManager) OnRingChange(callback func()) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.onRingChange = callback
}

// GetFailedNodes returns a map of failed nodes and when they failed
func (fm *FailoverManager) GetFailedNodes() map[string]time.Time {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	result := make(map[string]time.Time)
	for id, t := range fm.failedNodes {
		result[id] = t
	}
	return result
}

// RecoverNode attempts to add a previously failed node back to the ring
func (fm *FailoverManager) RecoverNode(node Node) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if _, wasFailed := fm.failedNodes[node.ID]; wasFailed {
		delete(fm.failedNodes, node.ID)
		log.Printf("Recovering node %s", node.ID)
	}

	fm.ring.AddNode(node)

	if fm.onRingChange != nil {
		go fm.onRingChange()
	}
}

// SetEnabled enables or disables failover handling
func (fm *FailoverManager) SetEnabled(enabled bool) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.enabled = enabled
}

// GetActiveNodes returns nodes that are currently active (not failed)
func (fm *FailoverManager) GetActiveNodes() []Node {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	nodes := fm.ring.GetNodes()
	active := make([]Node, 0, len(nodes))

	for _, node := range nodes {
		if _, failed := fm.failedNodes[node.ID]; !failed {
			active = append(active, node)
		}
	}

	return active
}
