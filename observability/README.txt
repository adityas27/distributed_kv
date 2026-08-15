OBSERVABILITY PACKAGE
=====================

This package provides metrics collection, structured logging, and HTTP endpoints
for monitoring and debugging the distributed KV cache.

METRICS
-------
Tracks performance and operational metrics:
- Request counts (total, GET, SET, DELETE)
- Error rates
- Latency (average, min, max in microseconds)
- Throughput (requests/sec, recent throughput)
- Uptime

Usage:
  metrics := observability.NewMetrics()
  metrics.RecordRequest("GET", latency)
  snapshot := metrics.GetSnapshot()

HTTP ENDPOINT
-------------
Exposes metrics via HTTP:
- GET /metrics - JSON metrics including cache stats and memory
- GET /health - Simple health check

Start server:
  handler := observability.NewMetricsHandler(metrics, cacheStatsFunc, "node1")
  observability.StartMetricsServer(":8080", handler)

STRUCTURED LOGGING
------------------
Provides leveled, structured logging with JSON or text output:

Levels: DEBUG, INFO, WARN, ERROR

Usage:
  logger := observability.NewLogger("node1", observability.INFO, false)
  logger.Info("Server starting", map[string]interface{}{
    "port": 5000,
    "mode": "cluster",
  })

File logging:
  logger.EnableFileLogging("server.log")

ENVIRONMENT VARIABLES
---------------------
- METRICS_ADDR: Metrics HTTP server address (default ":8080")
- LOG_LEVEL: Logging level (DEBUG/INFO/WARN/ERROR, default INFO)
- LOG_FORMAT: Output format ("json" or "text", default "text")
- LOG_FILE: Optional file path for log output

BENCHMARK TOOL
--------------
Located in ../benchmark/benchmark.go

Test cluster performance with configurable workload:

  benchmark.exe -addr localhost:5000 \
                -clients 10 \
                -duration 30 \
                -read 70 \
                -write 25 \
                -delete 5 \
                -keyspace 10000

Options:
  -addr: Server address
  -clients: Number of concurrent clients
  -duration: Test duration in seconds
  -keysize: Key size in bytes
  -valuesize: Value size in bytes
  -read: Percentage of GET operations
  -write: Percentage of SET operations
  -delete: Percentage of DELETE operations
  -keyspace: Total unique keys to use

Output:
  - Total operations and success rate
  - Throughput (ops/sec)
  - Latency statistics (min/avg/max)
  - Operation breakdown by type
