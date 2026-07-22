package cluster

import (
	"fmt"
	"sync"
)

// ClientPool manages connections to all nodes in the cluster
type ClientPool struct {
	mu      sync.RWMutex
	clients map[string]*NodeClient
}

// NewClientPool creates a new client pool
func NewClientPool() *ClientPool {
	return &ClientPool{
		clients: make(map[string]*NodeClient),
	}
}

// GetClient returns a client for the given node address, creating one if needed
func (cp *ClientPool) GetClient(address string) (*NodeClient, error) {
	if address == "" {
		return nil, fmt.Errorf("empty address")
	}

	// Try read lock first for fast path
	cp.mu.RLock()
	client, exists := cp.clients[address]
	cp.mu.RUnlock()

	if exists {
		return client, nil
	}

	// Need to create new client
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Double-check in case another goroutine created it
	client, exists = cp.clients[address]
	if exists {
		return client, nil
	}

	// Create new client
	client = NewNodeClient(address)
	cp.clients[address] = client
	return client, nil
}

// RemoveClient removes a client from the pool (used when node fails)
func (cp *ClientPool) RemoveClient(address string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if client, exists := cp.clients[address]; exists {
		client.Close()
		delete(cp.clients, address)
	}
}

// CloseAll closes all client connections
func (cp *ClientPool) CloseAll() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for _, client := range cp.clients {
		client.Close()
	}
	cp.clients = make(map[string]*NodeClient)
}
