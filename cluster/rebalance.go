package cluster

import (
	"log"
	"sync"
	"time"
)

// RebalanceManager handles key redistribution after topology changes
type RebalanceManager struct {
	mu                sync.RWMutex
	ring              *HashRing
	localNodeID       string
	replicaManager    *ReplicaManager
	onKeyMigrate      func(key string, targetNode Node) error
	enabled           bool
	pendingMigrations map[string]Node
}

func NewRebalanceManager(ring *HashRing, localNodeID string, replicaManager *ReplicaManager) *RebalanceManager {
	return &RebalanceManager{
		ring:              ring,
		localNodeID:       localNodeID,
		replicaManager:    replicaManager,
		enabled:           true,
		pendingMigrations: make(map[string]Node),
	}
}

// OnKeyMigrate sets a callback for migrating keys
func (rb *RebalanceManager) OnKeyMigrate(callback func(key string, targetNode Node) error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.onKeyMigrate = callback
}

// TriggerRebalance checks which keys need to be migrated after ring changes
func (rb *RebalanceManager) TriggerRebalance(keys []string) {
	if !rb.enabled {
		return
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	log.Printf("Starting rebalance for %d keys", len(keys))
	
	migrateCount := 0
	for _, key := range keys {
		// Check if we still own this key
		if rb.replicaManager != nil && rb.replicaManager.ShouldHandle(key) {
			continue
		}

		// Find new owner
		newOwner := rb.ring.GetNode(key)
		if newOwner.ID == "" || newOwner.ID == rb.localNodeID {
			continue
		}

		// Schedule migration
		rb.pendingMigrations[key] = newOwner
		migrateCount++
	}

	log.Printf("Scheduled %d keys for migration", migrateCount)

	// Execute migrations asynchronously
	if migrateCount > 0 && rb.onKeyMigrate != nil {
		go rb.executeMigrations()
	}
}

// executeMigrations processes pending key migrations
func (rb *RebalanceManager) executeMigrations() {
	rb.mu.Lock()
	migrations := make(map[string]Node)
	for k, v := range rb.pendingMigrations {
		migrations[k] = v
	}
	rb.pendingMigrations = make(map[string]Node)
	rb.mu.Unlock()

	successCount := 0
	failCount := 0

	for key, targetNode := range migrations {
		if err := rb.onKeyMigrate(key, targetNode); err != nil {
			log.Printf("Failed to migrate key %s to %s: %v", key, targetNode.ID, err)
			failCount++
		} else {
			successCount++
		}
		
		// Rate limit migrations
		time.Sleep(10 * time.Millisecond)
	}

	log.Printf("Migration complete: %d succeeded, %d failed", successCount, failCount)
}

// SetEnabled enables or disables rebalancing
func (rb *RebalanceManager) SetEnabled(enabled bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.enabled = enabled
}

// GetPendingCount returns number of pending migrations
func (rb *RebalanceManager) GetPendingCount() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return len(rb.pendingMigrations)
}
