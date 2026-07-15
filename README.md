# Distributed KV Cache

A Go-based key-value cache with a TCP interface, TTL expiration, LRU eviction, WAL-backed persistence, snapshot recovery, and an initial consistent-hash cluster layer.

## Current Status

The project is in a solid single-node stage with persistence and distributed foundations in place.

| Phase | Status | Notes |
| --- | --- | --- |
| Phase 1 - Core KV Store | Done | In-memory `SET`, `GET`, `DELETE`, thread-safe access, and TCP command handling are implemented. |
| Phase 2 - Cache Features | In progress | TTL and LRU eviction are implemented; stats are basic and do not yet report hits, misses, or evictions. |
| Phase 3 - Persistence | Done | WAL logging, recovery on startup, and snapshot creation are implemented. |
| Phase 4 - Distributed Foundations | In progress | Consistent hashing, virtual nodes, and cluster config loading exist, but they are not wired into request routing yet. |
| Phase 5 - Distributed Operations | Not started | Request routing and node-to-node communication are still planned. |
| Phase 6 - Reliability | Not started | Replication, heartbeats, failure detection, and rebalancing are not implemented yet. |
| Phase 7 - Observability | Not started | Metrics, structured logging, and benchmarking are still on the roadmap. |

## What Works Today

- In-memory cache with thread-safe access using `RWMutex`.
- `SET`, `GET`, `DELETE`, and `PING` commands over TCP.
- TTL support with background cleanup.
- LRU eviction when key count or memory limits are exceeded.
- WAL persistence for writes and deletes.
- Snapshot creation and startup recovery from snapshot plus WAL.
- Consistent-hash ring primitives and cluster configuration loading.

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

### Start the server

```bash
go run ./tcp
```

The server listens on `:9000` by default.

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
- The distributed packages are present as infrastructure, but the server still behaves as a single node.

