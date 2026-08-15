# Distributed KV Cache - Usage Guide

Complete guide for running and using the distributed cache system.

---

## Quick Start (Single Node)

### 1. Build
```bash
go build -o main.exe ./tcp/main.go
```

### 2. Run
```bash
main.exe
```

### 3. Connect
```bash
nc localhost 9000
```

### 4. Test Commands
```
PING
SET mykey 5
hello
GET mykey
DELETE mykey
STATS
```

---

## Cluster Mode (3 Nodes)

### Option 1: Using Start Script

```bash
cd test
start_cluster.bat
```

This starts 3 nodes on ports 5000, 5001, 5002.

### Option 2: Manual Start

**Terminal 1 - Node 1:**
```bash
set CLUSTER_NODE_ID=node1
set CLUSTER_ROUTING=true
main.exe
```

**Terminal 2 - Node 2:**
```bash
set CLUSTER_NODE_ID=node2
set CLUSTER_ROUTING=true
main.exe
```

**Terminal 3 - Node 3:**
```bash
set CLUSTER_NODE_ID=node3
set CLUSTER_ROUTING=true
main.exe
```

---

## Environment Variables

### Cluster Configuration
```bash
# Node identifier
set CLUSTER_NODE_ID=node1

# Enable cluster routing
set CLUSTER_ROUTING=true
```

### Observability
```bash
# Metrics HTTP port
set METRICS_ADDR=:8080

# Log level (DEBUG, INFO, WARN, ERROR)
set LOG_LEVEL=INFO

# Log format (json or text)
set LOG_FORMAT=json

# Optional log file
set LOG_FILE=server.log
```

### Complete Example
```bash
set CLUSTER_NODE_ID=node1
set CLUSTER_ROUTING=true
set LOG_LEVEL=INFO
set LOG_FORMAT=json
set METRICS_ADDR=:8080
main.exe
```

---

## Command Reference

### SET Command
Store a key-value pair.

**Syntax:**
```
SET <key> <value_length> [EX <ttl_seconds>]
<value>
```

**Examples:**
```
SET mykey 11
hello world

SET tempkey 4 EX 60
data
```

**Response:**
- `OK` - Success
- `ERR <message>` - Error

### GET Command
Retrieve a value by key.

**Syntax:**
```
GET <key>
```

**Example:**
```
GET mykey
```

**Response:**
- `<value>` - The stored value
- `NULL` - Key not found or expired

### DELETE Command
Remove a key.

**Syntax:**
```
DELETE <key>
```

**Example:**
```
DELETE mykey
```

**Response:**
- `OK` - Success
- `ERR <message>` - Error

### PING Command
Health check.

**Syntax:**
```
PING
```

**Response:**
```
PONG
```

### STATUS Command
Cluster status information.

**Syntax:**
```
STATUS
```

**Example Response:**
```
NODES:3 ALIVE:3 FAILED:0
```

In single-node mode:
```
SINGLE_NODE
```

### STATS Command
Cache statistics.

**Syntax:**
```
STATS
```

**Example Response:**
```
KEYS:42 MEM:1048576/536870912 HITS:1250 MISSES:50 RATE:96.2% EVICT:5 SETS:500 DEL:25
```

---

## Monitoring & Metrics

### HTTP Metrics Endpoint

Access metrics via HTTP:

```bash
curl http://localhost:8080/metrics
```

**Example Response:**
```json
{
  "node_id": "node1",
  "timestamp": 1692115200,
  "uptime_seconds": 3600.5,
  "requests": {
    "total": 15000,
    "get": 12000,
    "set": 2500,
    "delete": 500,
    "errors": 10,
    "rate_per_sec": "4.17",
    "recent_per_sec": "5.20"
  },
  "latency_micros": {
    "avg_all": "150.25",
    "avg_get": "75.50",
    "avg_set": "250.30",
    "avg_delete": "100.15"
  },
  "cache": {
    "items": 42,
    "memory_used": 1048576,
    "memory_limit": 536870912,
    "hits": 1250,
    "misses": 50,
    "hit_rate": 96.2,
    "evictions": 5,
    "sets": 500,
    "deletes": 25
  },
  "memory": {
    "alloc_bytes": 5242880,
    "total_alloc_bytes": 104857600,
    "sys_bytes": 10485760,
    "num_gc": 42,
    "goroutines": 25
  }
}
```

### Health Check

```bash
curl http://localhost:8080/health
```

**Response:**
```
OK
```

---

## Benchmarking

### Build Benchmark Tool

```bash
cd benchmark
go build -o benchmark.exe benchmark.go
```

### Run Benchmark

**Basic:**
```bash
benchmark.exe -addr localhost:5000
```

**Custom Configuration:**
```bash
benchmark.exe -addr localhost:5000 ^
              -clients 20 ^
              -duration 60 ^
              -read 80 ^
              -write 15 ^
              -delete 5 ^
              -keyspace 10000 ^
              -keysize 16 ^
              -valuesize 256
```

### Benchmark Options

| Option | Default | Description |
|--------|---------|-------------|
| `-addr` | localhost:5000 | Server address |
| `-clients` | 10 | Concurrent clients |
| `-duration` | 30 | Test duration (seconds) |
| `-read` | 80 | GET percentage |
| `-write` | 15 | SET percentage |
| `-delete` | 5 | DELETE percentage |
| `-keyspace` | 10000 | Unique keys |
| `-keysize` | 10 | Key size (bytes) |
| `-valuesize` | 100 | Value size (bytes) |

**Note:** Read + Write + Delete must equal 100%

### Example Output

```
=== Distributed KV Cache Benchmark ===
Server:      localhost:5000
Clients:     10
Duration:    30s
Key Size:    10 bytes
Value Size:  100 bytes
Operations:  GET=80% SET=15% DELETE=5%
Key Space:   10000 keys

Pre-populating cache... done

=== Benchmark Results ===

Duration:        30.05s
Total Ops:       125000
  Success:       124850 (99.9%)
  Failed:        150 (0.1%)

Operations Breakdown:
  GET:           100000 (80.0%)
  SET:           18750 (15.0%)
  DELETE:        6250 (5.0%)

Throughput:      4159.73 ops/sec

Latency:
  Average:       120.45 µs (0.120 ms)
  Min:           45 µs (0.045 ms)
  Max:           5420 µs (5.420 ms)
```

---

## Testing Failover

### 1. Start Cluster
```bash
cd test
start_cluster.bat
```

### 2. Connect and Insert Data
```bash
nc localhost 5000
SET key1 6
value1
SET key2 6
value2
```

### 3. Check Status
```
STATUS
```
Output: `NODES:3 ALIVE:3 FAILED:0`

### 4. Kill a Node
Close one of the node terminals (e.g., Node 2)

### 5. Wait for Failure Detection
Wait 15-20 seconds (3 heartbeats × 5s interval)

### 6. Check Status Again
```
STATUS
```
Output: `NODES:3 ALIVE:2 FAILED:1`

### 7. Verify Data Still Accessible
```
GET key1
GET key2
```

Data should still be accessible from remaining nodes due to replication!

---

## Testing Replication

### 1. Connect to Node 1
```bash
nc localhost 5000
```

### 2. Insert Data
```
SET replicated_key 14
replicated_val
```

### 3. Connect to Node 2
```bash
nc localhost 5001
```

### 4. Read Same Key
```
GET replicated_key
```

You should receive: `replicated_val`

This demonstrates the key is replicated across nodes!

---

## Viewing Logs

### Plain Text Logs (Default)
```
[2026-08-15T10:30:45Z] INFO [node1] Cache server starting {"node_id":"node1","routing_enabled":true}
[2026-08-15T10:30:45Z] INFO [node1] Reliability features enabled {"replication_factor":2,"heartbeat_interval":"5s"}
[2026-08-15T10:30:45Z] INFO [node1] Starting metrics server on :8080
[2026-08-15T10:30:45Z] INFO [node1] TCP server listening {"address":"localhost:5000"}
```

### JSON Logs
Set `LOG_FORMAT=json`:

```json
{"timestamp":"2026-08-15T10:30:45Z","level":"INFO","node_id":"node1","message":"Cache server starting","fields":{"node_id":"node1","routing_enabled":true}}
{"timestamp":"2026-08-15T10:30:45Z","level":"INFO","node_id":"node1","message":"Reliability features enabled","fields":{"replication_factor":2,"heartbeat_interval":"5s"}}
```

### Log to File
```bash
set LOG_FILE=server.log
main.exe
```

Logs will be written to `server.log` in addition to stdout.

---

## Client Integration Examples

### Go Client
```go
package main

import (
    "bufio"
    "fmt"
    "net"
)

func main() {
    conn, _ := net.Dial("tcp", "localhost:5000")
    defer conn.Close()

    // SET
    fmt.Fprintln(conn, "SET mykey 5")
    fmt.Fprintln(conn, "hello")
    reader := bufio.NewReader(conn)
    response, _ := reader.ReadString('\n')
    fmt.Println(response) // OK

    // GET
    fmt.Fprintln(conn, "GET mykey")
    value, _ := reader.ReadString('\n')
    fmt.Println(value) // hello
}
```

### Python Client
```python
import socket

conn = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
conn.connect(('localhost', 5000))

# SET
conn.sendall(b"SET mykey 5\nhello\n")
response = conn.recv(1024)
print(response.decode())  # OK

# GET
conn.sendall(b"GET mykey\n")
value = conn.recv(1024)
print(value.decode())  # hello

conn.close()
```

### Bash/Netcat
```bash
# SET
(echo "SET mykey 5"; echo "hello") | nc localhost 5000

# GET
echo "GET mykey" | nc localhost 5000

# DELETE
echo "DELETE mykey" | nc localhost 5000
```

---

## Troubleshooting

### Issue: Cannot connect to server
**Check:**
- Is server running? Look for "Listening on" message
- Correct port? Default is 9000 (single) or 5000-5002 (cluster)
- Firewall blocking connections?

### Issue: Keys not found after failover
**Check:**
- Is replication enabled? (Cluster mode only)
- Did you wait for replication to complete?
- Check STATUS command output

### Issue: High latency
**Check:**
- Network latency between nodes
- Is cache full and evicting? (Check STATS)
- High concurrent load? (Monitor metrics)

### Issue: Metrics endpoint not accessible
**Check:**
- Is METRICS_ADDR set correctly?
- Default port is :8080
- Try: `curl http://localhost:8080/metrics`

### Issue: Logs not appearing
**Check:**
- LOG_LEVEL setting (default is INFO)
- Set LOG_LEVEL=DEBUG for verbose output
- Check LOG_FILE path if file logging enabled

---

## Configuration Files

### cluster/cluster.json
```json
{
    "virtual_nodes": 100,
    "nodes": [
        {
            "id": "node1",
            "address": "localhost:5000"
        },
        {
            "id": "node2",
            "address": "localhost:5001"
        },
        {
            "id": "node3",
            "address": "localhost:5002"
        }
    ]
}
```

To add more nodes:
1. Add entry to nodes array
2. Restart all nodes
3. Keys will rebalance automatically

---

## Best Practices

### Data Safety
- Always run cluster mode with replication in production
- Use TTL for temporary data
- Monitor cache hit rate via STATS

### Performance
- Keep key/value sizes reasonable (<1MB)
- Use connection pooling in clients
- Monitor memory usage and evictions

### Monitoring
- Set up metrics scraping from /metrics endpoint
- Enable JSON logging for log aggregation
- Monitor STATUS for cluster health

### Capacity Planning
- Memory limit: 512MB per node (configurable)
- Max keys: 100 per node (configurable)
- Plan for 2x replication overhead

---

## Summary

This distributed KV cache provides:
- ✅ High performance in-memory storage
- ✅ Automatic data distribution across nodes
- ✅ Multi-replica fault tolerance
- ✅ Self-healing via failover and rebalancing
- ✅ Production-grade observability
- ✅ Simple TCP protocol

Perfect for caching, session storage, and distributed state management!
