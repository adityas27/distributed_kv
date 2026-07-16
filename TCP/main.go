package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"tcp_test/cluster"
	"tcp_test/parser"
	"tcp_test/persistence"
	"tcp_test/storage"
)

type Server struct {
	cache          *storage.Cache
	manager        *persistence.SnapshotManager
	clusterRing    *cluster.HashRing
	localNodeID    string
	routingEnabled bool
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
	}, nil
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

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Println("Client connected:", conn.RemoteAddr())

	scanner := bufio.NewScanner(conn)
	if scanner.Err() != nil {
		fmt.Println(scanner.Err().Error())
	}
	for scanner.Scan() {
		line := scanner.Text()
		cmd, err := parser.Parse(line)
		if err != nil {
			fmt.Fprintln(conn, "ERROR", err.Error())
			continue
		}

		response := s.execute(cmd)

		_, err = fmt.Fprintln(conn, response)
		if err != nil {
			return
		}
	}
}

func (s *Server) execute(cmd *parser.Command) string {
	switch cmd.Name {

	case "PING":
		return "PONG"

	case "SET":
		if redirect := s.clusterRedirect(cmd.Key); redirect != "" {
			return redirect
		}
		if err := s.cache.Set(cmd.Key, cmd.Value, cmd.TTL); err != nil {
			return "ERR " + err.Error()
		}
		return "OK"

	case "GET":
		if redirect := s.clusterRedirect(cmd.Key); redirect != "" {
			return redirect
		}
		value, ok := s.cache.Get(cmd.Key)
		if !ok {
			return "NULL"
		}
		return value

	case "DELETE":
		if redirect := s.clusterRedirect(cmd.Key); redirect != "" {
			return redirect
		}
		if err := s.cache.Delete(cmd.Key); err != nil {
			return "ERR " + err.Error()
		}
		return "OK"

	default:
		return "ERROR unknown command"
	}
}

func (s *Server) clusterRedirect(key string) string {
	// Return a redirect only when cluster routing is enabled and this node does not own the key.
	// In the default single-node path the function stays inert and the cache behaves normally.
	if s.clusterRing == nil || !s.routingEnabled || key == "" {
		return ""
	}

	owner := s.clusterRing.GetNode(key)
	if owner.ID == "" || owner.ID == s.localNodeID {
		return ""
	}

	if owner.Address == "" {
		return fmt.Sprintf("ERR key is owned by cluster node %s", owner.ID)
	}

	return fmt.Sprintf("REDIRECT %s %s", owner.ID, owner.Address)
}

func (s *Server) Shutdown() {
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

	// Start the TCP server
	log.Println("Starting cache server on :9000")
	if err := server.Start(":9000"); err != nil {
		log.Fatalf("server error: %v", err)
	}
	defer server.Shutdown()
}
