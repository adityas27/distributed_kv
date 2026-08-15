package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type BenchmarkConfig struct {
	Address        string
	Clients        int
	Duration       int
	KeySize        int
	ValueSize      int
	ReadPercent    int
	SetPercent     int
	DeletePercent  int
	KeySpace       int
}

type BenchmarkResult struct {
	TotalOps       int64
	SuccessOps     int64
	FailedOps      int64
	GetOps         int64
	SetOps         int64
	DeleteOps      int64
	TotalLatency   int64
	MinLatency     int64
	MaxLatency     int64
	Duration       time.Duration
}

func main() {
	config := BenchmarkConfig{}

	flag.StringVar(&config.Address, "addr", "localhost:5000", "Server address")
	flag.IntVar(&config.Clients, "clients", 10, "Number of concurrent clients")
	flag.IntVar(&config.Duration, "duration", 30, "Benchmark duration in seconds")
	flag.IntVar(&config.KeySize, "keysize", 10, "Key size in bytes")
	flag.IntVar(&config.ValueSize, "valuesize", 100, "Value size in bytes")
	flag.IntVar(&config.ReadPercent, "read", 80, "Percentage of GET operations")
	flag.IntVar(&config.SetPercent, "write", 15, "Percentage of SET operations")
	flag.IntVar(&config.DeletePercent, "delete", 5, "Percentage of DELETE operations")
	flag.IntVar(&config.KeySpace, "keyspace", 10000, "Total number of unique keys")
	flag.Parse()

	// Validate percentages
	total := config.ReadPercent + config.SetPercent + config.DeletePercent
	if total != 100 {
		fmt.Printf("Error: percentages must sum to 100 (got %d)\n", total)
		os.Exit(1)
	}

	fmt.Println("=== Distributed KV Cache Benchmark ===")
	fmt.Printf("Server:      %s\n", config.Address)
	fmt.Printf("Clients:     %d\n", config.Clients)
	fmt.Printf("Duration:    %ds\n", config.Duration)
	fmt.Printf("Key Size:    %d bytes\n", config.KeySize)
	fmt.Printf("Value Size:  %d bytes\n", config.ValueSize)
	fmt.Printf("Operations:  GET=%d%% SET=%d%% DELETE=%d%%\n", 
		config.ReadPercent, config.SetPercent, config.DeletePercent)
	fmt.Printf("Key Space:   %d keys\n", config.KeySpace)
	fmt.Println()

	// Pre-populate cache with some data
	fmt.Print("Pre-populating cache... ")
	prepopulate(config)
	fmt.Println("done")
	fmt.Println()

	// Run benchmark
	result := runBenchmark(config)

	// Print results
	printResults(result)
}

func prepopulate(config BenchmarkConfig) {
	conn, err := net.Dial("tcp", config.Address)
	if err != nil {
		fmt.Printf("Error connecting: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Insert 50% of keyspace
	numKeys := config.KeySpace / 2
	for i := 0; i < numKeys; i++ {
		key := generateKey(i, config.KeySize)
		value := generateValue(config.ValueSize)
		
		cmd := fmt.Sprintf("SET %s %d\n", key, len(value))
		conn.Write([]byte(cmd))
		conn.Write([]byte(value))
		conn.Write([]byte("\n"))
		
		// Read response
		reader := bufio.NewReader(conn)
		reader.ReadString('\n')
	}
}

func runBenchmark(config BenchmarkConfig) BenchmarkResult {
	var result BenchmarkResult
	result.MinLatency = int64(^uint64(0) >> 1) // Max int64

	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	startTime := time.Now()

	// Start worker goroutines
	for i := 0; i < config.Clients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			runClient(clientID, config, &result, stopChan)
		}(i)
	}

	// Run for specified duration
	time.Sleep(time.Duration(config.Duration) * time.Second)
	close(stopChan)

	wg.Wait()

	result.Duration = time.Since(startTime)
	return result
}

func runClient(clientID int, config BenchmarkConfig, result *BenchmarkResult, stopChan chan struct{}) {
	conn, err := net.Dial("tcp", config.Address)
	if err != nil {
		fmt.Printf("Client %d: connection failed: %v\n", clientID, err)
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(clientID)))

	for {
		select {
		case <-stopChan:
			return
		default:
			// Decide operation type
			opType := rng.Intn(100)
			
			var cmd string
			var latency time.Duration
			var success bool

			start := time.Now()

			if opType < config.ReadPercent {
				// GET operation
				keyID := rng.Intn(config.KeySpace)
				key := generateKey(keyID, config.KeySize)
				cmd = fmt.Sprintf("GET %s\n", key)
				
				conn.Write([]byte(cmd))
				response, err := reader.ReadString('\n')
				success = err == nil && response != "NULL\n"
				
				atomic.AddInt64(&result.GetOps, 1)

			} else if opType < config.ReadPercent+config.SetPercent {
				// SET operation
				keyID := rng.Intn(config.KeySpace)
				key := generateKey(keyID, config.KeySize)
				value := generateValue(config.ValueSize)
				
				cmd = fmt.Sprintf("SET %s %d\n", key, len(value))
				conn.Write([]byte(cmd))
				conn.Write([]byte(value))
				conn.Write([]byte("\n"))
				
				response, err := reader.ReadString('\n')
				success = err == nil && response == "OK\n"
				
				atomic.AddInt64(&result.SetOps, 1)

			} else {
				// DELETE operation
				keyID := rng.Intn(config.KeySpace)
				key := generateKey(keyID, config.KeySize)
				cmd = fmt.Sprintf("DELETE %s\n", key)
				
				conn.Write([]byte(cmd))
				response, err := reader.ReadString('\n')
				success = err == nil && response == "OK\n"
				
				atomic.AddInt64(&result.DeleteOps, 1)
			}

			latency = time.Since(start)
			latencyMicros := latency.Microseconds()

			atomic.AddInt64(&result.TotalOps, 1)
			atomic.AddInt64(&result.TotalLatency, latencyMicros)

			if success {
				atomic.AddInt64(&result.SuccessOps, 1)
			} else {
				atomic.AddInt64(&result.FailedOps, 1)
			}

			// Update min/max latency
			for {
				currentMin := atomic.LoadInt64(&result.MinLatency)
				if latencyMicros >= currentMin {
					break
				}
				if atomic.CompareAndSwapInt64(&result.MinLatency, currentMin, latencyMicros) {
					break
				}
			}

			for {
				currentMax := atomic.LoadInt64(&result.MaxLatency)
				if latencyMicros <= currentMax {
					break
				}
				if atomic.CompareAndSwapInt64(&result.MaxLatency, currentMax, latencyMicros) {
					break
				}
			}
		}
	}
}

func generateKey(id int, size int) string {
	key := fmt.Sprintf("key_%d", id)
	if len(key) < size {
		padding := make([]byte, size-len(key))
		for i := range padding {
			padding[i] = 'x'
		}
		key = key + string(padding)
	}
	return key[:size]
}

func generateValue(size int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, size)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

func printResults(result BenchmarkResult) {
	fmt.Println("=== Benchmark Results ===")
	fmt.Println()
	
	fmt.Printf("Duration:        %.2fs\n", result.Duration.Seconds())
	fmt.Printf("Total Ops:       %d\n", result.TotalOps)
	fmt.Printf("  Success:       %d (%.1f%%)\n", 
		result.SuccessOps, float64(result.SuccessOps)/float64(result.TotalOps)*100)
	fmt.Printf("  Failed:        %d (%.1f%%)\n", 
		result.FailedOps, float64(result.FailedOps)/float64(result.TotalOps)*100)
	fmt.Println()
	
	fmt.Printf("Operations Breakdown:\n")
	fmt.Printf("  GET:           %d (%.1f%%)\n", 
		result.GetOps, float64(result.GetOps)/float64(result.TotalOps)*100)
	fmt.Printf("  SET:           %d (%.1f%%)\n", 
		result.SetOps, float64(result.SetOps)/float64(result.TotalOps)*100)
	fmt.Printf("  DELETE:        %d (%.1f%%)\n", 
		result.DeleteOps, float64(result.DeleteOps)/float64(result.TotalOps)*100)
	fmt.Println()
	
	throughput := float64(result.TotalOps) / result.Duration.Seconds()
	fmt.Printf("Throughput:      %.2f ops/sec\n", throughput)
	fmt.Println()
	
	avgLatency := float64(result.TotalLatency) / float64(result.TotalOps)
	fmt.Printf("Latency:\n")
	fmt.Printf("  Average:       %.2f µs (%.3f ms)\n", avgLatency, avgLatency/1000)
	fmt.Printf("  Min:           %d µs (%.3f ms)\n", 
		result.MinLatency, float64(result.MinLatency)/1000)
	fmt.Printf("  Max:           %d µs (%.3f ms)\n", 
		result.MaxLatency, float64(result.MaxLatency)/1000)
	fmt.Println()
}
