package cluster

import (
	"fmt"
	"sync"
)

// ReplicationFactor defines how many replicas each key should have
const DefaultReplicationFactor = 2

// ReplicaManager handles replication of writes across nodes
type ReplicaManager struct {
	ring              *HashRing
	replicationFactor int
	clientPool        *ClientPool
	localNodeID       string
	enabled           bool
}

func NewReplicaManager(ring *HashRing, clientPool *ClientPool, localNodeID string, factor int) *ReplicaManager {
	if factor < 1 {
		factor = DefaultReplicationFactor
	}

	return &ReplicaManager{
		ring:              ring,
		replicationFactor: factor,
		clientPool:        clientPool,
		localNodeID:       localNodeID,
		enabled:           true,
	}
}

// GetReplicas returns the primary and replica nodes for a key
// First node is primary, rest are replicas
func (rm *ReplicaManager) GetReplicas(key string) []Node {
	if rm.ring == nil || !rm.enabled {
		return nil
	}

	nodes := rm.ring.GetNodes()
	if len(nodes) == 0 {
		return nil
	}
	
	if len(nodes) == 1 {
		return nodes
	}

	// Find primary node
	primary := rm.ring.GetNode(key)
	if primary.ID == "" {
		return nil
	}

	// Build replica list starting with primary
	replicas := []Node{primary}
	added := make(map[string]bool)
	added[primary.ID] = true

	// Get sortedHashes to find next nodes in ring order
	sortedHashes := rm.ring.GetSortedHashes()
	if len(sortedHashes) == 0 {
		return replicas
	}

	// Find primary position in ring
	keyHash := Hash(key)
	primaryIdx := 0
	for i, h := range sortedHashes {
		if rm.ring.GetNodeAtHash(h).ID == primary.ID {
			primaryIdx = i
			break
		}
	}

	// Walk the ring forward to get next N-1 unique nodes
	count := len(sortedHashes)
	for i := 1; i < count && len(replicas) < rm.replicationFactor; i++ {
		idx := (primaryIdx + i) % count
		node := rm.ring.GetNodeAtHash(sortedHashes[idx])
		if !added[node.ID] {
			replicas = append(replicas, node)
			added[node.ID] = true
		}
	}

	return replicas
}

// ShouldHandle returns true if this node should handle the key (primary or replica)
func (rm *ReplicaManager) ShouldHandle(key string) bool {
	replicas := rm.GetReplicas(key)
	for _, node := range replicas {
		if node.ID == rm.localNodeID {
			return true
		}
	}
	return false
}

// IsPrimary returns true if this node is the primary for the key
func (rm *ReplicaManager) IsPrimary(key string) bool {
	replicas := rm.GetReplicas(key)
	if len(replicas) == 0 {
		return false
	}
	return replicas[0].ID == rm.localNodeID
}

// ReplicateWrite sends a write operation to all replica nodes
func (rm *ReplicaManager) ReplicateWrite(cmd string, key string, valueData []byte) error {
	if !rm.enabled {
		return nil
	}

	replicas := rm.GetReplicas(key)
	if len(replicas) <= 1 {
		return nil // No replicas
	}

	// Skip primary (self) and replicate to others
	var wg sync.WaitGroup
	errors := make(chan error, len(replicas)-1)

	for i := 1; i < len(replicas); i++ {
		replica := replicas[i]
		if replica.ID == rm.localNodeID {
			continue
		}

		wg.Add(1)
		go func(node Node) {
			defer wg.Done()
			if err := rm.sendToReplica(node.Address, cmd, valueData); err != nil {
				errors <- fmt.Errorf("replica %s: %w", node.ID, err)
			}
		}(replica)
	}

	wg.Wait()
	close(errors)

	// Collect errors but don't fail the write
	var lastErr error
	for err := range errors {
		lastErr = err
	}

	return lastErr // Return last error if any (logged but not critical)
}

func (rm *ReplicaManager) sendToReplica(address string, cmd string, valueData []byte) error {
	client, err := rm.clientPool.GetClient(address)
	if err != nil {
		return err
	}

	// Add REPLICA prefix to avoid infinite replication loops
	replicaCmd := "REPLICA " + cmd

	_, err = client.Forward(replicaCmd, valueData)
	return err
}

// SetEnabled enables or disables replication
func (rm *ReplicaManager) SetEnabled(enabled bool) {
	rm.enabled = enabled
}
