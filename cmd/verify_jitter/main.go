// verify_jitter measures inter-arrival times of bytes piped through stdin.
// Usage: ./ttylag --serial 100 -- head -c 50 /dev/zero | go run cmd/verify_jitter/main.go
package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	buf := make([]byte, 1)
	prev := time.Now()

	// Read first byte to prime the clock (ignore startup latency)
	_, err := os.Stdin.Read(buf)
	if err != nil {
		return
	}
	prev = time.Now()

	count := 0
	var sum time.Duration
	var min, max time.Duration
	min = 100 * time.Second // Start high

	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		now := time.Now()
		delta := now.Sub(prev)
		prev = now

		sum += delta
		if delta < min {
			min = delta
		}
		if delta > max {
			max = delta
		}
		count++
	}

	if count > 0 {
		avg := sum / time.Duration(count)
		fmt.Printf("Count: %d | Min: %v | Max: %v | Avg: %v\n", count, min, max, avg)
	}
}
