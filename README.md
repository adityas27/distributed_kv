# Distributed KV Cache

A Go-based key-value cache with a TCP interface, TTL expiration, LRU eviction, WAL-backed persistence, snapshot recovery, and an initial consistent-hash cluster layer.

## Current Status

The project is now a **fully functional distributed cache** with automatic request forwarding!

| Phase | Status | Notes |
| --- | --- | --- |
| Phase 1 - Core KV Store | ✅ Done | In-memory `SET`, `GET`, `DELETE`, thread-safe access, and TCP command handling are implemented. |
| Phase 2 - Cache Features | ⚠️ In progress | TTL and LRU eviction are implemented; stats are basic and do not yet report hits, misses, or evictions. |
| Phase 3 - Persistence | ✅ Done | WAL logging, recovery on startup, and snapshot creation are implemented. |
| Phase 4 - Distributed Foundations | ✅ Done | Consistent hashing, virtual nodes, cluster config, AND transparent request forwarding are fully implemented! |
| Phase 5 - Distributed Operations | ✅ Mostly Done | Request routing and node-to-node communication are complete. Advanced features optional. |
| Phase 6 - Reliability | Not started | Replication, heartbeats, failure detection, and rebalancing are not implemented yet. |
| Phase 7 - Observability | Not started | Metrics, structured logging, and benchmarking are still on the roadmap. |

## What Works Today

- In-memory cache with thread-safe access using `RWMutex`.
- `SET`, `GET`, `DELETE`, and `PING` commands over TCP.
- TTL support with background cleanup.
- LRU eviction when key count or memory limits are exceeded.
- WAL persistence for writes and deletes.
- Snapshot creation and startup recovery from snapshot plus WAL.
- **✨ NEW: Full distributed mode with automatic request forwarding!**
- **✨ NEW: Multi-node cluster support (3+ nodes)**
- **✨ NEW: Consistent hashing with transparent routing**
- **✨ NEW: Connection pooling for inter-node communication**

## Command Protocol

The TCP server accepts plain text commands:

```text
SET key value [EX seconds]
GET key
DELETE key
PING
```

Examples:

```text
SET name alice
SET session token EX 60
GET name
DELETE name
PING
```

## Getting Started

### Requirements

- Go 1.26 or newer

### Run the tests

```bash
go test ./...
```

### Start a single node (development)

```bash
go run ./tcp
```

The server listens on `:9000` by default.

### Start a 3-node cluster (distributed mode)

**Windows:**
```batch
cd test
start_cluster.bat
```

**Manual (3 separate terminals):**
```bash
# Terminal 1 - Node 1 on :5000
set CLUSTER_NODE_ID=node1
set CLUSTER_ROUTING=true
go run ./tcp

# Terminal 2 - Node 2 on :5001
set CLUSTER_NODE_ID=node2
set CLUSTER_ROUTING=true
go run ./tcp

# Terminal 3 - Node 3 on :5002
set CLUSTER_NODE_ID=node3
set CLUSTER_ROUTING=true
go run ./tcp
```

### Test the cluster

```bash
# Option 1: Automated test
cd test
go run test_cluster.go

# Option 2: Manual with netcat
nc localhost 5000
SET testkey 5
hello
GET testkey
```

Connect to **any node** - requests are automatically forwarded to the correct node!

## Project Layout

```text
cluster/      Hash ring and cluster configuration
parser/       Text command parser
persistence/  Snapshot manager and recovery logic
storage/      Thread-safe in-memory cache
tcp/          TCP server entry point
wal/          WAL implementation
```

## Notes

- Runtime files such as `wal.log` and `snapshot.json` are created in the project root.
- **The distributed mode is fully operational!** Requests are automatically forwarded to the correct node.
- Each node maintains its own WAL and snapshot for the keys it owns.
- Set `CLUSTER_ROUTING=true` to enable distributed mode.
- See `PHASE4_COMPLETION.md` for detailed distributed deployment guide.

