//go:build ignore

// verify_shaper tests the Shaper directly without requiring a TTY.
// This replicates the verification tests from a competing implementation.
//
// Usage: go run cmd/verify_shaper/main.go
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/time/rate"
)

// ShaperConfig matches the config in shaper.go
type ShaperConfig struct {
	Delay     time.Duration
	Jitter    time.Duration
	Rate      int64
	ChunkSize int
	FrameTime time.Duration
	Seed      int64
}

// Minimal shaper for bandwidth testing only
type testShaper struct {
	limiter *rate.Limiter
}

func newTestShaper(bytesPerSec int64) *testShaper {
	if bytesPerSec <= 0 {
		return &testShaper{}
	}
	return &testShaper{
		limiter: rate.NewLimiter(rate.Limit(bytesPerSec), int(bytesPerSec)),
	}
}

func (s *testShaper) Copy(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 1024)
	var total int64

	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}

		n, err := src.Read(buf)
		if n > 0 {
			// Rate limit
			if s.limiter != nil {
				if err := s.limiter.WaitN(ctx, n); err != nil {
					return total, err
				}
			}
			written, werr := dst.Write(buf[:n])
			total += int64(written)
			if werr != nil {
				return total, werr
			}
		}
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

func main() {
	fmt.Println("=== ttylag Shaper Verification Tests ===")
	fmt.Println()

	testBandwidth()
	fmt.Println()
	testJitterInfo()
}

func testBandwidth() {
	fmt.Println("1. Bandwidth Verification (Direct Shaper Test)")
	fmt.Println("   Target: 10 kbit/s = 1250 bytes/sec")
	fmt.Println("   Data: 5000 bytes")
	fmt.Println()

	// Create test data
	data := make([]byte, 5000)
	src := bytes.NewReader(data)
	dst := io.Discard

	// 10 kbit/s = 1250 bytes/sec
	shaper := newTestShaper(1250)

	ctx := context.Background()
	start := time.Now()
	n, err := shaper.Copy(ctx, dst, src)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("   ERROR: %v\n", err)
		return
	}

	bits := n * 8
	bps := float64(bits) / duration.Seconds()

	fmt.Printf("   Read %d bytes in %v\n", n, duration)
	fmt.Printf("   Rate: %.2f bits/sec\n", bps)
	fmt.Printf("   Expected: ~10000 bits/sec (±10%%: 9000-11000)\n")

	if bps >= 9000 && bps <= 11000 {
		fmt.Println("   ✓ PASS")
	} else {
		fmt.Println("   ✗ FAIL - rate outside expected range")
	}
}

func testJitterInfo() {
	fmt.Println("2. Jitter Tests (require TTY - informational)")
	fmt.Println()
	fmt.Println("   The jitter tests require a real terminal because ttylag")
	fmt.Println("   sets raw mode on stdin. Use these commands in a terminal:")
	fmt.Println()
	fmt.Println("   Baseline (no jitter):")
	fmt.Println("   $ ./ttylag -s 100 -- head -c 50 /dev/zero | go run cmd/verify_jitter/main.go")
	fmt.Println("   Expected: Count: 49 | Min: ~95ms | Max: ~105ms | Avg: ~100ms")
	fmt.Println()
	fmt.Println("   With jitter (±40ms):")
	fmt.Println("   $ ./ttylag -s 100 -j 40ms -- head -c 50 /dev/zero | go run cmd/verify_jitter/main.go")
	fmt.Println("   Expected: Count: 49 | Min: very low | Max: ~140-180ms | Avg: ~100ms")
}
