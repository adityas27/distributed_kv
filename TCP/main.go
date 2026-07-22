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
	cache          *storage.Cache
	manager        *persistence.SnapshotManager
	clusterRing    *cluster.HashRing
	localNodeID    string
	routingEnabled bool
	connSlots      chan struct{}
	clientPool     *cluster.ClientPool
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

	return &Server{
		cache:          cache,
		manager:        manager,
		clusterRing:    clusterRing,
		localNodeID:    localNodeID,
		routingEnabled: routingEnabled,
		connSlots:      make(chan struct{}, maxConcurrentClients),
		clientPool:     cluster.NewClientPool(),
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

	fmt.Println("Listening on", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}

		if !s.acquireConnection() {
			_ = conn.Close()
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	defer s.releaseConnection()

	fmt.Println("Client connected:", conn.RemoteAddr())

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
	switch cmd.Name {

	case "PING":
		return "PONG"

	case "SET":
		// Check if we need to forward to another node
		if response, forwarded := s.forwardIfNeeded(cmd); forwarded {
			return response
		}
		if err := s.cache.Set(cmd.Key, cmd.Value, cmd.TTL); err != nil {
			return "ERR " + err.Error()
		}
		return "OK"

	case "GET":
		// Check if we need to forward to another node
		if response, forwarded := s.forwardIfNeeded(cmd); forwarded {
			return response
		}
		value, ok := s.cache.Get(cmd.Key)
		if !ok {
			return "NULL"
		}
		return value

	case "DELETE":
		// Check if we need to forward to another node
		if response, forwarded := s.forwardIfNeeded(cmd); forwarded {
			return response
		}
		if err := s.cache.Delete(cmd.Key); err != nil {
			return "ERR " + err.Error()
		}
		return "OK"

	default:
		return "ERROR unknown command"
	}
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
