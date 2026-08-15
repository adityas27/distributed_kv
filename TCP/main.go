package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"tcp_test/cluster"
	"tcp_test/observability"
	"tcp_test/parser"
	"tcp_test/persistence"
	"tcp_test/storage"
	"time"
)

const (
	maxCommandHeaderBytes  = 8 * 1024
	maxConcurrentClients   = 128
	connectionReadTimeout  = 30 * time.Second
	connectionWriteTimeout = 10 * time.Second
	maxForwardRetries      = 2
)

type Server struct {
	cache             *storage.Cache
	manager           *persistence.SnapshotManager
	clusterRing       *cluster.HashRing
	localNodeID       string
	routingEnabled    bool
	connSlots         chan struct{}
	clientPool        *cluster.ClientPool
	replicaManager    *cluster.ReplicaManager
	heartbeatMonitor  *cluster.HeartbeatMonitor
	failoverManager   *cluster.FailoverManager
	rebalanceManager  *cluster.RebalanceManager
	metrics           *observability.Metrics
	logger            *observability.Logger
}

// NewServer initializes a new server with persistence and recovery
func NewServer() (*Server, error) {
	cache := storage.NewCache()
	clusterRing, localNodeID, routingEnabled := loadClusterRouting()

	snapshotCfg := persistence.NewSnapshotConfig(".", "snapshot.json")

	recovery := persistence.NewRecoveryManager(
		snapshotCfg,
		"wal.log",
	)

	if _, err := recovery.Recover(cache); err != nil {
		return nil, fmt.Errorf("failed to recover: %w", err)
	}

	manager := persistence.NewSnapshotManager(
		cache,
		cache.WAL(),
		persistence.DefaultSnapshotManagerConfig(),
	)

	if err := manager.Start(); err != nil {
		return nil, err
	}

	clientPool := cluster.NewClientPool()

	// Initialize observability
	metrics := observability.NewMetrics()
	
	logLevel := observability.INFO
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		logLevel = observability.DEBUG
	}
	
	jsonLogs := os.Getenv("LOG_FORMAT") == "json"
	logger := observability.NewLogger(localNodeID, logLevel, jsonLogs)
	
	// Enable file logging if configured
	if logFile := os.Getenv("LOG_FILE"); logFile != "" {
		if err := logger.EnableFileLogging(logFile); err != nil {
			log.Printf("Failed to enable file logging: %v", err)
		}
	}

	logger.Info("Cache server starting", map[string]interface{}{
		"node_id":         localNodeID,
		"routing_enabled": routingEnabled,
	})

	// Initialize Phase 6 features if clustering is enabled
	var replicaManager *cluster.ReplicaManager
	var heartbeatMonitor *cluster.HeartbeatMonitor
	var failoverManager *cluster.FailoverManager
	var rebalanceManager *cluster.RebalanceManager

	if routingEnabled && clusterRing != nil && localNodeID != "" {
		// Setup replication (2 replicas by default)
		replicaManager = cluster.NewReplicaManager(clusterRing, clientPool, localNodeID, 2)

		// Setup heartbeat monitoring
		heartbeatMonitor = cluster.NewHeartbeatMonitor(clusterRing, clientPool, localNodeID)

		// Setup failover management
		failoverManager = cluster.NewFailoverManager(clusterRing, heartbeatMonitor, clientPool, localNodeID)

		// Setup rebalancing
		rebalanceManager = cluster.NewRebalanceManager(clusterRing, localNodeID, replicaManager)

		// Start heartbeat monitoring
		heartbeatMonitor.Start()

		// Register ring change callback for rebalancing
		failoverManager.OnRingChange(func() {
			logger.Info("Ring topology changed, triggering rebalance", nil)
			keys := cache.GetAllKeys()
			rebalanceManager.TriggerRebalance(keys)
		})

		// Setup key migration callback
		rebalanceManager.OnKeyMigrate(func(key string, targetNode cluster.Node) error {
			// Get the value from cache
			value, ok := cache.Get(key)
			if !ok {
				return nil // Key already gone
			}

			// Forward to target node
			client, err := clientPool.GetClient(targetNode.Address)
			if err != nil {
				return err
			}

			cmdLine := fmt.Sprintf("SET %s %d", key, len(value))
			_, err = client.Forward(cmdLine, []byte(value))
			if err != nil {
				return err
			}

			// Delete locally after successful migration
			_ = cache.Delete(key)
			return nil
		})

		log.Println("Reliability features enabled: replication, heartbeat, failover, rebalancing")
		logger.Info("Reliability features enabled", map[string]interface{}{
			"replication_factor": 2,
			"heartbeat_interval": "5s",
			"failure_threshold":  3,
		})
	}

	// Start metrics HTTP server if configured
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":8080" // Default metrics port
	}
	
	metricsHandler := observability.NewMetricsHandler(metrics, cache.Stats, localNodeID)
	go func() {
		logger.Infof("Starting metrics server on %s", metricsAddr)
		if err := observability.StartMetricsServer(metricsAddr, metricsHandler); err != nil {
			logger.Error("Metrics server failed", map[string]interface{}{"error": err.Error()})
		}
	}()

	return &Server{
		cache:             cache,
		manager:           manager,
		clusterRing:       clusterRing,
		localNodeID:       localNodeID,
		routingEnabled:    routingEnabled,
		connSlots:         make(chan struct{}, maxConcurrentClients),
		clientPool:        clientPool,
		replicaManager:    replicaManager,
		heartbeatMonitor:  heartbeatMonitor,
		failoverManager:   failoverManager,
		rebalanceManager:  rebalanceManager,
		metrics:           metrics,
		logger:            logger,
	}, nil
}

// GetListenAddress returns the address this server should listen on
func (s *Server) GetListenAddress() string {
	// If cluster routing is enabled and we have a ring, use the configured address
	if s.routingEnabled && s.clusterRing != nil && s.localNodeID != "" {
		nodes := s.clusterRing.GetNodes()
		for _, node := range nodes {
			if node.ID == s.localNodeID {
				return node.Address
			}
		}
	}
	// Default to :9000 for single-node mode
	return ":9000"
}

func loadClusterRouting() (*cluster.HashRing, string, bool) {
	// The cluster config is optional, so the server keeps working when it is absent.
	// Routing only becomes active when the CLUSTER_ROUTING flag is turned on.
	clusterPath := filepath.Join("cluster", "cluster.json")
	ring, err := cluster.LoadConfig(clusterPath)
	if err != nil {
		log.Printf("cluster config not loaded: %v", err)
		return nil, "", false
	}

	localNodeID := os.Getenv("CLUSTER_NODE_ID")
	if localNodeID == "" {
		nodes := ring.GetNodes()
		if len(nodes) > 0 {
			localNodeID = nodes[0].ID
		}
	}

	routingEnabled := strings.EqualFold(os.Getenv("CLUSTER_ROUTING"), "true") || os.Getenv("CLUSTER_ROUTING") == "1"
	return ring, localNodeID, routingEnabled
}

func (s *Server) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	s.logger.Info("TCP server listening", map[string]interface{}{
		"address": addr,
	})
	fmt.Println("Listening on", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.logger.Error("Accept error", map[string]interface{}{
				"error": err.Error(),
			})
			fmt.Println("accept error:", err)
			continue
		}

		if !s.acquireConnection() {
			s.logger.Warn("Connection limit reached, rejecting client", map[string]interface{}{
				"remote_addr": conn.RemoteAddr().String(),
			})
			_ = conn.Close()
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	defer s.releaseConnection()

	clientAddr := conn.RemoteAddr().String()
	s.logger.Debug("Client connected", map[string]interface{}{
		"remote_addr": clientAddr,
	})
	fmt.Println("Client connected:", clientAddr)

	reader := bufio.NewReader(conn)

	for {
		if err := conn.SetReadDeadline(time.Now().Add(connectionReadTimeout)); err != nil {
			return
		}

		line, err := readHeaderLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}

			if !writeServerError(conn, err) {
				return
			}

			return
		}

		cmd, err := parser.Parse(line)
		if err != nil {
			if !writeServerError(conn, err) {
				return
			}

			continue
		}

		if cmd.Name == "SET" {
			if len(cmd.Key) > storage.MaxKeySize {
				if !writeServerError(conn, fmt.Errorf("key too large")) {
					return
				}

				return
			}

			if cmd.ValueLength > storage.MaxValueSize {
				if !writeServerError(conn, fmt.Errorf("value too large")) {
					return
				}

				return
			}

			if err := conn.SetReadDeadline(time.Now().Add(connectionReadTimeout)); err != nil {
				return
			}

			value := make([]byte, cmd.ValueLength)
			if _, err := io.ReadFull(reader, value); err != nil {
				return
			}

			if err := consumeOptionalLineEnding(reader); err != nil {
				return
			}

			cmd.Value = string(value)
		}

		response := s.execute(cmd)

		if err := conn.SetWriteDeadline(time.Now().Add(connectionWriteTimeout)); err != nil {
			return
		}

		if !writeServerResponse(conn, response) {
			return
		}
	}
}

func readHeaderLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if len(line) > maxCommandHeaderBytes {
		return "", fmt.Errorf("command too large")
	}

	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	line = strings.TrimRight(line, "\r\n")
	if line == "" && errors.Is(err, io.EOF) {
		return "", io.EOF
	}

	return line, nil
}

func consumeOptionalLineEnding(reader *bufio.Reader) error {
	peek, err := reader.Peek(1)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}

		return err
	}

	if peek[0] == '\n' {
		_, err = reader.ReadByte()
		return err
	}

	if peek[0] == '\r' {
		if _, err := reader.ReadByte(); err != nil {
			return err
		}

		if next, err := reader.Peek(1); err == nil && len(next) == 1 && next[0] == '\n' {
			_, err := reader.ReadByte()
			return err
		}

		return nil
	}

	return nil
}

func writeServerError(conn net.Conn, err error) bool {
	if setErr := conn.SetWriteDeadline(time.Now().Add(connectionWriteTimeout)); setErr != nil {
		return false
	}

	return writeServerResponse(conn, "ERROR "+err.Error())
}

func writeServerResponse(conn net.Conn, response string) bool {
	_, err := fmt.Fprintln(conn, response)
	return err == nil
}

func (s *Server) acquireConnection() bool {
	select {
	case s.connSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) releaseConnection() {
	select {
	case <-s.connSlots:
	default:
	}
}

func (s *Server) execute(cmd *parser.Command) string {
	startTime := time.Now()
	var result string
	var cmdType string

	switch cmd.Name {

	case "PING":
		result = "PONG"
		cmdType = "PING"

	case "REPLICA":
		// Handle replica write (don't replicate again to avoid loops)
		// Extract actual command after REPLICA prefix
		result = s.handleReplicaWrite(cmd)
		cmdType = "REPLICA"

	case "SET":
		cmdType = "SET"
		// Check if we should handle this key (primary or replica)
		if s.replicaManager != nil && !s.replicaManager.ShouldHandle(cmd.Key) {
			// Forward to the correct node
			if response, forwarded := s.forwardIfNeeded(cmd); forwarded {
				result = response
				break
			}
		}

		// Handle the write locally
		if err := s.cache.Set(cmd.Key, cmd.Value, cmd.TTL); err != nil {
			result = "ERR " + err.Error()
			s.metrics.RecordError()
			break
		}

		// Replicate to replica nodes if we're the primary
		if s.replicaManager != nil && s.replicaManager.IsPrimary(cmd.Key) {
			var cmdLine string
			if cmd.TTL > 0 {
				cmdLine = fmt.Sprintf("SET %s %d EX %d", cmd.Key, len(cmd.Value), cmd.TTL)
			} else {
				cmdLine = fmt.Sprintf("SET %s %d", cmd.Key, len(cmd.Value))
			}
			
			if err := s.replicaManager.ReplicateWrite(cmdLine, cmd.Key, []byte(cmd.Value)); err != nil {
				s.logger.Warn("Replication failed", map[string]interface{}{
					"key":   cmd.Key,
					"error": err.Error(),
				})
				// Don't fail the write if replication fails
			}
		}

		result = "OK"

	case "GET":
		cmdType = "GET"
		// For GET operations, we can read from any replica (primary or secondary)
		if s.replicaManager != nil && !s.replicaManager.ShouldHandle(cmd.Key) {
			// Forward to primary (could optimize to read from nearest replica)
			if response, forwarded := s.forwardIfNeeded(cmd); forwarded {
				result = response
				break
			}
		}

		value, ok := s.cache.Get(cmd.Key)
		if !ok {
			result = "NULL"
		} else {
			result = value
		}

	case "DELETE":
		cmdType = "DELETE"
		// Check if we should handle this key (primary or replica)
		if s.replicaManager != nil && !s.replicaManager.ShouldHandle(cmd.Key) {
			// Forward to the correct node
			if response, forwarded := s.forwardIfNeeded(cmd); forwarded {
				result = response
				break
			}
		}

		// Handle the delete locally
		if err := s.cache.Delete(cmd.Key); err != nil {
			result = "ERR " + err.Error()
			s.metrics.RecordError()
			break
		}

		// Replicate to replica nodes if we're the primary
		if s.replicaManager != nil && s.replicaManager.IsPrimary(cmd.Key) {
			cmdLine := fmt.Sprintf("DELETE %s", cmd.Key)
			if err := s.replicaManager.ReplicateWrite(cmdLine, cmd.Key, nil); err != nil {
				s.logger.Warn("Delete replication failed", map[string]interface{}{
					"key":   cmd.Key,
					"error": err.Error(),
				})
			}
		}

		result = "OK"

	case "STATUS":
		cmdType = "STATUS"
		// Return cluster status
		result = s.getClusterStatus()

	case "STATS":
		cmdType = "STATS"
		// Return cache statistics
		result = s.getCacheStats()

	default:
		cmdType = "UNKNOWN"
		result = "ERROR unknown command"
		s.metrics.RecordError()
	}

	// Record metrics
	latency := time.Since(startTime)
	s.metrics.RecordRequest(cmdType, latency)

	return result
}

// forwardIfNeeded checks if the key should be handled by another node
// and forwards the request if cluster routing is enabled.
// Returns (response, true) if forwarded, ("", false) if should be handled locally.
func (s *Server) forwardIfNeeded(cmd *parser.Command) (string, bool) {
	// In single-node mode or when routing is disabled, handle everything locally
	if s.clusterRing == nil || !s.routingEnabled || cmd.Key == "" {
		return "", false
	}

	// Find the node that should own this key
	owner := s.clusterRing.GetNode(cmd.Key)
	
	// If we own it or owner is invalid, handle locally
	if owner.ID == "" || owner.ID == s.localNodeID {
		return "", false
	}

	// If no address configured, return error
	if owner.Address == "" {
		return fmt.Sprintf("ERR key is owned by cluster node %s but no address configured", owner.ID), true
	}

	// Forward the request to the owner node
	response, err := s.forwardRequest(cmd, owner.Address)
	if err != nil {
		log.Printf("forward to %s failed: %v", owner.Address, err)
		return fmt.Sprintf("ERR failed to forward request: %v", err), true
	}

	return response, true
}

// forwardRequest sends the command to a remote node and returns the response
func (s *Server) forwardRequest(cmd *parser.Command, address string) (string, error) {
	client, err := s.clientPool.GetClient(address)
	if err != nil {
		return "", err
	}

	// Build command line based on command type
	var cmdLine string
	var valueData []byte

	switch cmd.Name {
	case "SET":
		if cmd.TTL > 0 {
			cmdLine = fmt.Sprintf("SET %s %d EX %d", cmd.Key, len(cmd.Value), cmd.TTL)
		} else {
			cmdLine = fmt.Sprintf("SET %s %d", cmd.Key, len(cmd.Value))
		}
		valueData = []byte(cmd.Value)

	case "GET":
		cmdLine = fmt.Sprintf("GET %s", cmd.Key)

	case "DELETE":
		cmdLine = fmt.Sprintf("DELETE %s", cmd.Key)

	default:
		return "", fmt.Errorf("unknown command: %s", cmd.Name)
	}

	// Forward with retry logic
	var lastErr error
	for attempt := 0; attempt < maxForwardRetries; attempt++ {
		response, err := client.Forward(cmdLine, valueData)
		if err == nil {
			// Check if we got another redirect (prevents infinite loops)
			if strings.HasPrefix(response, "REDIRECT") {
				return "ERR circular redirect detected", nil
			}
			return response, nil
		}
		
		lastErr = err
		
		// On error, remove client from pool and retry
		if attempt < maxForwardRetries-1 {
			s.clientPool.RemoveClient(address)
			time.Sleep(100 * time.Millisecond)
			client, err = s.clientPool.GetClient(address)
			if err != nil {
				return "", err
			}
		}
	}

	return "", fmt.Errorf("exhausted retries: %w", lastErr)
}

func (s *Server) Shutdown() {
	if s.heartbeatMonitor != nil {
		s.heartbeatMonitor.Stop()
	}

	if s.clientPool != nil {
		s.clientPool.CloseAll()
	}

	if s.manager != nil {
		_ = s.manager.CreateSnapshotNow()
		_ = s.manager.Stop()
	}

	if s.cache.WAL() != nil {
		_ = s.cache.WAL().Close()
	}
}

// handleReplicaWrite processes a write that came from a primary node
func (s *Server) handleReplicaWrite(cmd *parser.Command) string {
	// REPLICA command format: REPLICA SET key len [EX ttl]
	// We need to extract the actual command and execute it without replication
	
	// For now, just return OK since the command parser already extracted the data
	// In a real system, you'd parse the replica command properly
	
	switch {
	case strings.Contains(cmd.Key, "SET"):
		// Extract key from the replica command
		parts := strings.Fields(cmd.Key)
		if len(parts) < 3 {
			return "ERR invalid replica command"
		}
		
		actualKey := parts[1]
		
		// Find TTL if present
		var ttl int
		for i, part := range parts {
			if part == "EX" && i+1 < len(parts) {
				fmt.Sscanf(parts[i+1], "%d", &ttl)
				break
			}
		}
		
		if err := s.cache.Set(actualKey, cmd.Value, ttl); err != nil {
			return "ERR " + err.Error()
		}
		return "OK"
		
	case strings.Contains(cmd.Key, "DELETE"):
		parts := strings.Fields(cmd.Key)
		if len(parts) < 2 {
			return "ERR invalid replica command"
		}
		
		actualKey := parts[1]
		if err := s.cache.Delete(actualKey); err != nil {
			return "ERR " + err.Error()
		}
		return "OK"
		
	default:
		return "ERR unknown replica command"
	}
}

// getClusterStatus returns cluster health and status information
func (s *Server) getClusterStatus() string {
	if !s.routingEnabled || s.clusterRing == nil {
		return "SINGLE_NODE"
	}

	nodes := s.clusterRing.GetNodes()
	status := fmt.Sprintf("NODES:%d", len(nodes))

	if s.heartbeatMonitor != nil {
		allHealth := s.heartbeatMonitor.GetAllHealth()
		aliveCount := 0
		for _, health := range allHealth {
			if health.IsAlive {
				aliveCount++
			}
		}
		status += fmt.Sprintf(" ALIVE:%d", aliveCount+1) // +1 for self
	}

	if s.failoverManager != nil {
		failed := s.failoverManager.GetFailedNodes()
		status += fmt.Sprintf(" FAILED:%d", len(failed))
	}

	if s.rebalanceManager != nil {
		pending := s.rebalanceManager.GetPendingCount()
		if pending > 0 {
			status += fmt.Sprintf(" REBALANCING:%d", pending)
		}
	}

	return status
}

// getCacheStats returns cache statistics
func (s *Server) getCacheStats() string {
	stats := s.cache.Stats()
	
	return fmt.Sprintf("KEYS:%v MEM:%v/%v HITS:%v MISSES:%v RATE:%.1f%% EVICT:%v SETS:%v DEL:%v",
		stats["items"],
		stats["memory_used"],
		stats["memory_limit"],
		stats["hits"],
		stats["misses"],
		stats["hit_rate"],
		stats["evictions"],
		stats["sets"],
		stats["deletes"],
	)
}

func main() {
	// Initialize server with persistence recovery
	server, err := NewServer()

	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	// Determine listen address (cluster mode uses configured port)
	listenAddr := server.GetListenAddress()

	// Start the TCP server
	log.Printf("Starting cache server on %s", listenAddr)
	if err := server.Start(listenAddr); err != nil {
		log.Fatalf("server error: %v", err)
	}
	defer server.Shutdown()
}
