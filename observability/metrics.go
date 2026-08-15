package observability

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects performance and operational metrics
type Metrics struct {
	mu sync.RWMutex

	// Request counters
	totalRequests  int64
	getRequests    int64
	setRequests    int64
	deleteRequests int64
	errorCount     int64

	// Latency tracking (in microseconds)
	totalLatency   int64
	getLatency     int64
	setLatency     int64
	deleteLatency  int64
	latencySamples int64

	// Throughput tracking
	startTime      time.Time
	lastResetTime  time.Time
	requestsWindow []int64
	windowIndex    int
	windowSize     int
}

func NewMetrics() *Metrics {
	windowSize := 60 // Track last 60 seconds
	return &Metrics{
		startTime:      time.Now(),
		lastResetTime:  time.Now(),
		requestsWindow: make([]int64, windowSize),
		windowSize:     windowSize,
	}
}

// RecordRequest records a completed request with latency
func (m *Metrics) RecordRequest(cmdType string, latency time.Duration) {
	atomic.AddInt64(&m.totalRequests, 1)

	latencyMicros := latency.Microseconds()
	atomic.AddInt64(&m.totalLatency, latencyMicros)
	atomic.AddInt64(&m.latencySamples, 1)

	switch cmdType {
	case "GET":
		atomic.AddInt64(&m.getRequests, 1)
		atomic.AddInt64(&m.getLatency, latencyMicros)
	case "SET":
		atomic.AddInt64(&m.setRequests, 1)
		atomic.AddInt64(&m.setLatency, latencyMicros)
	case "DELETE":
		atomic.AddInt64(&m.deleteRequests, 1)
		atomic.AddInt64(&m.deleteLatency, latencyMicros)
	}

	// Update sliding window for throughput
	m.mu.Lock()
	currentSecond := int(time.Since(m.startTime).Seconds()) % m.windowSize
	m.requestsWindow[currentSecond]++
	m.mu.Unlock()
}

// RecordError increments error counter
func (m *Metrics) RecordError() {
	atomic.AddInt64(&m.errorCount, 1)
}

// GetSnapshot returns current metrics snapshot
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := atomic.LoadInt64(&m.totalRequests)
	gets := atomic.LoadInt64(&m.getRequests)
	sets := atomic.LoadInt64(&m.setRequests)
	deletes := atomic.LoadInt64(&m.deleteRequests)
	errors := atomic.LoadInt64(&m.errorCount)

	totalLat := atomic.LoadInt64(&m.totalLatency)
	getLat := atomic.LoadInt64(&m.getLatency)
	setLat := atomic.LoadInt64(&m.setLatency)
	deleteLat := atomic.LoadInt64(&m.deleteLatency)
	samples := atomic.LoadInt64(&m.latencySamples)

	avgLatency := float64(0)
	if samples > 0 {
		avgLatency = float64(totalLat) / float64(samples)
	}

	avgGetLatency := float64(0)
	if gets > 0 {
		avgGetLatency = float64(getLat) / float64(gets)
	}

	avgSetLatency := float64(0)
	if sets > 0 {
		avgSetLatency = float64(setLat) / float64(sets)
	}

	avgDeleteLatency := float64(0)
	if deletes > 0 {
		avgDeleteLatency = float64(deleteLat) / float64(deletes)
	}

	uptime := time.Since(m.startTime)
	requestsPerSec := float64(0)
	if uptime.Seconds() > 0 {
		requestsPerSec = float64(total) / uptime.Seconds()
	}

	// Calculate recent throughput (last 10 seconds)
	recentReqs := int64(0)
	currentSecond := int(time.Since(m.startTime).Seconds()) % m.windowSize
	for i := 0; i < 10 && i < m.windowSize; i++ {
		idx := (currentSecond - i + m.windowSize) % m.windowSize
		recentReqs += m.requestsWindow[idx]
	}
	recentThroughput := float64(recentReqs) / 10.0

	return MetricsSnapshot{
		TotalRequests:       total,
		GetRequests:         gets,
		SetRequests:         sets,
		DeleteRequests:      deletes,
		ErrorCount:          errors,
		AvgLatencyMicros:    avgLatency,
		AvgGetLatencyMicros: avgGetLatency,
		AvgSetLatencyMicros: avgSetLatency,
		AvgDelLatencyMicros: avgDeleteLatency,
		RequestsPerSec:      requestsPerSec,
		RecentThroughput:    recentThroughput,
		UptimeSeconds:       uptime.Seconds(),
	}
}

// Reset resets all metrics
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	atomic.StoreInt64(&m.totalRequests, 0)
	atomic.StoreInt64(&m.getRequests, 0)
	atomic.StoreInt64(&m.setRequests, 0)
	atomic.StoreInt64(&m.deleteRequests, 0)
	atomic.StoreInt64(&m.errorCount, 0)
	atomic.StoreInt64(&m.totalLatency, 0)
	atomic.StoreInt64(&m.getLatency, 0)
	atomic.StoreInt64(&m.setLatency, 0)
	atomic.StoreInt64(&m.deleteLatency, 0)
	atomic.StoreInt64(&m.latencySamples, 0)

	m.requestsWindow = make([]int64, m.windowSize)
	m.lastResetTime = time.Now()
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	TotalRequests       int64
	GetRequests         int64
	SetRequests         int64
	DeleteRequests      int64
	ErrorCount          int64
	AvgLatencyMicros    float64
	AvgGetLatencyMicros float64
	AvgSetLatencyMicros float64
	AvgDelLatencyMicros float64
	RequestsPerSec      float64
	RecentThroughput    float64
	UptimeSeconds       float64
}
