package observability

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"
)

type MetricsHandler struct {
	metrics    *Metrics
	cacheStats func() map[string]interface{}
	nodeID     string
}

func NewMetricsHandler(metrics *Metrics, cacheStats func() map[string]interface{}, nodeID string) *MetricsHandler {
	return &MetricsHandler{
		metrics:    metrics,
		cacheStats: cacheStats,
		nodeID:     nodeID,
	}
}

// ServeHTTP handles HTTP requests for metrics
func (mh *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshot := mh.metrics.GetSnapshot()
	cacheStats := mh.cacheStats()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	response := map[string]interface{}{
		"node_id":   mh.nodeID,
		"timestamp": time.Now().Unix(),
		"uptime_seconds": snapshot.UptimeSeconds,
		
		"requests": map[string]interface{}{
			"total":           snapshot.TotalRequests,
			"get":             snapshot.GetRequests,
			"set":             snapshot.SetRequests,
			"delete":          snapshot.DeleteRequests,
			"errors":          snapshot.ErrorCount,
			"rate_per_sec":    fmt.Sprintf("%.2f", snapshot.RequestsPerSec),
			"recent_per_sec":  fmt.Sprintf("%.2f", snapshot.RecentThroughput),
		},
		
		"latency_micros": map[string]interface{}{
			"avg_all":    fmt.Sprintf("%.2f", snapshot.AvgLatencyMicros),
			"avg_get":    fmt.Sprintf("%.2f", snapshot.AvgGetLatencyMicros),
			"avg_set":    fmt.Sprintf("%.2f", snapshot.AvgSetLatencyMicros),
			"avg_delete": fmt.Sprintf("%.2f", snapshot.AvgDelLatencyMicros),
		},
		
		"cache": cacheStats,
		
		"memory": map[string]interface{}{
			"alloc_bytes":       memStats.Alloc,
			"total_alloc_bytes": memStats.TotalAlloc,
			"sys_bytes":         memStats.Sys,
			"num_gc":            memStats.NumGC,
			"goroutines":        runtime.NumGoroutine(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// StartMetricsServer starts HTTP server for metrics endpoint
func StartMetricsServer(addr string, handler *MetricsHandler) error {
	http.Handle("/metrics", handler)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	log.Printf("Metrics endpoint available at http://%s/metrics", addr)
	log.Printf("Health check available at http://%s/health", addr)

	return http.ListenAndServe(addr, nil)
}
