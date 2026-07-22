package cluster

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	nodeConnectionTimeout = 5 * time.Second
	nodeReadTimeout       = 10 * time.Second
	nodeWriteTimeout      = 5 * time.Second
	maxForwardRetries     = 2
)

// NodeClient manages TCP connections to other cluster nodes
type NodeClient struct {
	address string
	mu      sync.Mutex
	conn    net.Conn
}

// NewNodeClient creates a client for communicating with a remote node
func NewNodeClient(address string) *NodeClient {
	return &NodeClient{
		address: address,
	}
}

// Forward sends a command to the remote node and returns the response
func (nc *NodeClient) Forward(cmdLine string, valueData []byte) (string, error) {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	// Establish connection if needed
	if nc.conn == nil {
		conn, err := net.DialTimeout("tcp", nc.address, nodeConnectionTimeout)
		if err != nil {
			return "", fmt.Errorf("failed to connect to node %s: %w", nc.address, err)
		}
		nc.conn = conn
	}

	// Set write deadline
	if err := nc.conn.SetWriteDeadline(time.Now().Add(nodeWriteTimeout)); err != nil {
		nc.closeConnection()
		return "", fmt.Errorf("failed to set write deadline: %w", err)
	}

	// Send command header
	if _, err := fmt.Fprintln(nc.conn, cmdLine); err != nil {
		nc.closeConnection()
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	// Send value data if present (for SET commands)
	if len(valueData) > 0 {
		if _, err := nc.conn.Write(valueData); err != nil {
			nc.closeConnection()
			return "", fmt.Errorf("failed to send value: %w", err)
		}
		// Send newline after value
		if _, err := nc.conn.Write([]byte("\n")); err != nil {
			nc.closeConnection()
			return "", fmt.Errorf("failed to send newline: %w", err)
		}
	}

	// Set read deadline
	if err := nc.conn.SetReadDeadline(time.Now().Add(nodeReadTimeout)); err != nil {
		nc.closeConnection()
		return "", fmt.Errorf("failed to set read deadline: %w", err)
	}

	// Read response
	reader := bufio.NewReader(nc.conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		nc.closeConnection()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("connection closed by remote node")
		}
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimRight(response, "\r\n")
	return response, nil
}

// Close closes the connection to the remote node
func (nc *NodeClient) Close() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.closeConnection()
}

func (nc *NodeClient) closeConnection() {
	if nc.conn != nil {
		_ = nc.conn.Close()
		nc.conn = nil
	}
}
