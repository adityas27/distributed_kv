package cluster

import (
	"log"
	"sync"
	"time"
)

const (
	HeartbeatInterval    = 5 * time.Second
	FailureThreshold     = 3 // Miss 3 heartbeats = failure
	HeartbeatTimeout     = 2 * time.Second
)

// NodeHealth tracks the health status of a node
type NodeHealth struct {
	NodeID       string
	LastSeen     time.Time
	FailureCount int
	IsAlive      bool
}

// HeartbeatMonitor manages health checks for all cluster nodes
type HeartbeatMonitor struct {
	mu          sync.RWMutex
	ring        *HashRing
	clientPool  *ClientPool
	localNodeID string
	health      map[string]*NodeHealth
	stopCh      chan struct{}
	running     bool
	onFailure   func(nodeID string)
}

func NewHeartbeatMonitor(ring *HashRing, clientPool *ClientPool, localNodeID string) *HeartbeatMonitor {
	hm := &HeartbeatMonitor{
		ring:        ring,
		clientPool:  clientPool,
		localNodeID: localNodeID,
		health:      make(map[string]*NodeHealth),
		stopCh:      make(chan struct{}),
	}

	// Initialize health for all nodes
	for _, node := range ring.GetNodes() {
		if node.ID != localNodeID {
			hm.health[node.ID] = &NodeHealth{
				NodeID:   node.ID,
				LastSeen: time.Now(),
				IsAlive:  true,
			}
		}
	}

	return hm
}

// Start begins the heartbeat monitoring loop
func (hm *HeartbeatMonitor) Start() {
	hm.mu.Lock()
	if hm.running {
		hm.mu.Unlock()
		return
	}
	hm.running = true
	hm.mu.Unlock()

	go hm.monitorLoop()
}

// Stop stops the heartbeat monitoring
func (hm *HeartbeatMonitor) Stop() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if !hm.running {
		return
	}

	hm.running = false
	close(hm.stopCh)
}

// OnFailure sets a callback for when a node is detected as failed
func (hm *HeartbeatMonitor) OnFailure(callback func(nodeID string)) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.onFailure = callback
}

// GetNodeHealth returns the health status of a specific node
func (hm *HeartbeatMonitor) GetNodeHealth(nodeID string) *NodeHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if health, exists := hm.health[nodeID]; exists {
		return &NodeHealth{
			NodeID:       health.NodeID,
			LastSeen:     health.LastSeen,
			FailureCount: health.FailureCount,
			IsAlive:      health.IsAlive,
		}
	}
	return nil
}

// GetAllHealth returns health status for all nodes
func (hm *HeartbeatMonitor) GetAllHealth() map[string]*NodeHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make(map[string]*NodeHealth)
	for id, health := range hm.health {
		result[id] = &NodeHealth{
			NodeID:       health.NodeID,
			LastSeen:     health.LastSeen,
			FailureCount: health.FailureCount,
			IsAlive:      health.IsAlive,
		}
	}
	return result
}

// monitorLoop continuously checks node health
func (hm *HeartbeatMonitor) monitorLoop() {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hm.checkAllNodes()
		case <-hm.stopCh:
			return
		}
	}
}

// checkAllNodes sends heartbeats to all nodes and updates health
func (hm *HeartbeatMonitor) checkAllNodes() {
	nodes := hm.ring.GetNodes()

	for _, node := range nodes {
		if node.ID == hm.localNodeID {
			continue // Don't check self
		}

		go hm.checkNode(node)
	}
}

// checkNode sends a PING to a node and updates its health
func (hm *HeartbeatMonitor) checkNode(node Node) {
	client, err := hm.clientPool.GetClient(node.Address)
	if err != nil {
		hm.recordFailure(node.ID)
		return
	}

	// Send PING command with timeout
	done := make(chan bool, 1)
	go func() {
		_, err := client.Forward("PING", nil)
		done <- (err == nil)
	}()

	select {
	case success := <-done:
		if success {
			hm.recordSuccess(node.ID)
		} else {
			hm.recordFailure(node.ID)
		}
	case <-time.After(HeartbeatTimeout):
		hm.recordFailure(node.ID)
	}
}

// recordSuccess marks a successful heartbeat
func (hm *HeartbeatMonitor) recordSuccess(nodeID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	health, exists := hm.health[nodeID]
	if !exists {
		health = &NodeHealth{NodeID: nodeID}
		hm.health[nodeID] = health
	}

	health.LastSeen = time.Now()
	health.FailureCount = 0

	if !health.IsAlive {
		log.Printf("Node %s recovered", nodeID)
		health.IsAlive = true
	}
}

// recordFailure marks a failed heartbeat and triggers failure detection
func (hm *HeartbeatMonitor) recordFailure(nodeID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	health, exists := hm.health[nodeID]
	if !exists {
		health = &NodeHealth{NodeID: nodeID, IsAlive: true}
		hm.health[nodeID] = health
	}

	health.FailureCount++

	// Trigger failure if threshold exceeded
	if health.IsAlive && health.FailureCount >= FailureThreshold {
		log.Printf("Node %s declared failed after %d missed heartbeats", nodeID, health.FailureCount)
		health.IsAlive = false

		// Trigger failure callback if set
		if hm.onFailure != nil {
			go hm.onFailure(nodeID)
		}
	}
}

// IsNodeAlive returns true if the node is considered alive
func (hm *HeartbeatMonitor) IsNodeAlive(nodeID string) bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if health, exists := hm.health[nodeID]; exists {
		return health.IsAlive
	}
	return true // Unknown nodes assumed alive
}
