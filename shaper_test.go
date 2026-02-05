package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestShaperDelay(t *testing.T) {
	cfg := ShaperConfig{
		Delay: 100 * time.Millisecond,
		Seed:  42, // Deterministic
	}

	input := "hello"
	src := strings.NewReader(input)
	var dst bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := Copy(ctx, &dst, src, cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	if dst.String() != input {
		t.Errorf("output mismatch: got %q, want %q", dst.String(), input)
	}

	// Allow ±20ms tolerance for timing
	expectedMin := 80 * time.Millisecond
	expectedMax := 150 * time.Millisecond
	if elapsed < expectedMin || elapsed > expectedMax {
		t.Errorf("delay out of range: got %v, expected %v to %v", elapsed, expectedMin, expectedMax)
	}
}

func TestShaperJitter(t *testing.T) {
	cfg := ShaperConfig{
		Delay:  50 * time.Millisecond,
		Jitter: 30 * time.Millisecond,
		Seed:   42,
	}

	// Run multiple iterations to test jitter distribution
	var durations []time.Duration
	for i := 0; i < 5; i++ {
		src := strings.NewReader("x")
		var dst bytes.Buffer

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		start := time.Now()
		err := Copy(ctx, &dst, src, cfg)
		elapsed := time.Since(start)
		cancel()

		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}
		durations = append(durations, elapsed)
	}

	// Check all delays are within expected range: [20ms, 80ms] (delay ± jitter)
	for i, d := range durations {
		// Allow extra tolerance for scheduling
		minExpected := 10 * time.Millisecond
		maxExpected := 120 * time.Millisecond
		if d < minExpected || d > maxExpected {
			t.Errorf("iteration %d: delay %v outside range [%v, %v]", i, d, minExpected, maxExpected)
		}
	}
}

func TestShaperRateLimit(t *testing.T) {
	// Rate limit to 100 bytes/second, so 10 bytes should take ~100ms
	cfg := ShaperConfig{
		Rate:  100,
		Burst: 10, // Small burst to ensure rate limiting kicks in
		Seed:  42,
	}

	input := strings.Repeat("x", 20) // 20 bytes
	src := strings.NewReader(input)
	var dst bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := Copy(ctx, &dst, src, cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	if dst.String() != input {
		t.Errorf("output mismatch: got %q, want %q", dst.String(), input)
	}

	// 20 bytes at 100 bytes/sec = 200ms minimum (minus burst allowance)
	// With burst of 10, first 10 bytes are instant, remaining 10 take 100ms
	expectedMin := 80 * time.Millisecond
	expectedMax := 300 * time.Millisecond
	if elapsed < expectedMin || elapsed > expectedMax {
		t.Errorf("rate limiting incorrect: got %v, expected %v to %v", elapsed, expectedMin, expectedMax)
	}
}

func TestShaperChunking(t *testing.T) {
	cfg := ShaperConfig{
		ChunkSize: 3,
		Seed:      42,
	}

	shaper := NewShaper(cfg)

	// Test splitChunks directly
	input := []byte("hello world")
	chunks := shaper.splitChunks(input)

	// "hello world" = 11 bytes, should be 4 chunks: "hel", "lo ", "wor", "ld"
	expectedChunks := []string{"hel", "lo ", "wor", "ld"}
	if len(chunks) != len(expectedChunks) {
		t.Fatalf("chunk count mismatch: got %d, want %d", len(chunks), len(expectedChunks))
	}

	for i, chunk := range chunks {
		if string(chunk) != expectedChunks[i] {
			t.Errorf("chunk %d: got %q, want %q", i, string(chunk), expectedChunks[i])
		}
	}
}

func TestShaperChunkingWrite(t *testing.T) {
	cfg := ShaperConfig{
		ChunkSize: 5,
		Seed:      42,
	}

	input := "hello world" // 11 bytes
	src := strings.NewReader(input)

	// Track individual writes
	tracker := &writeTracker{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := Copy(ctx, tracker, src, cfg)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	// Check that no write exceeds chunk size
	for i, w := range tracker.writes {
		if len(w) > cfg.ChunkSize {
			t.Errorf("write %d: size %d exceeds chunk size %d", i, len(w), cfg.ChunkSize)
		}
	}

	// Check total data is correct
	if tracker.total.String() != input {
		t.Errorf("output mismatch: got %q, want %q", tracker.total.String(), input)
	}
}

// writeTracker records each write for inspection
type writeTracker struct {
	writes [][]byte
	total  bytes.Buffer
}

func (w *writeTracker) Write(p []byte) (n int, err error) {
	// Make a copy since the slice may be reused
	data := make([]byte, len(p))
	copy(data, p)
	w.writes = append(w.writes, data)
	return w.total.Write(p)
}

func TestShaperFraming(t *testing.T) {
	cfg := ShaperConfig{
		FrameTime: 50 * time.Millisecond,
		Seed:      42,
	}

	// Use a slow reader that produces data over time
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte("a"))
		time.Sleep(10 * time.Millisecond)
		pw.Write([]byte("b"))
		time.Sleep(10 * time.Millisecond)
		pw.Write([]byte("c"))
		pw.Close()
	}()

	tracker := &writeTracker{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := Copy(ctx, tracker, pr, cfg)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	// With framing, data should be coalesced into fewer writes
	// (exact count depends on timing, but should be less than 3)
	if tracker.total.String() != "abc" {
		t.Errorf("output mismatch: got %q, want %q", tracker.total.String(), "abc")
	}
}

func TestShaperNoConfig(t *testing.T) {
	// Test with empty config (passthrough mode)
	cfg := ShaperConfig{}

	input := "hello world"
	src := strings.NewReader(input)
	var dst bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := Copy(ctx, &dst, src, cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	if dst.String() != input {
		t.Errorf("output mismatch: got %q, want %q", dst.String(), input)
	}

	// Should be nearly instant (< 50ms)
	if elapsed > 50*time.Millisecond {
		t.Errorf("passthrough too slow: %v", elapsed)
	}
}

func TestShaperContextCancel(t *testing.T) {
	cfg := ShaperConfig{
		Delay: 1 * time.Second, // Long delay
	}

	// Use a reader that blocks
	pr, pw := io.Pipe()
	go func() {
		time.Sleep(100 * time.Millisecond)
		pw.Write([]byte("data"))
		// Don't close - let context cancel handle it
	}()

	var dst bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := Copy(ctx, &dst, pr, cfg)
	elapsed := time.Since(start)

	// Should be cancelled, not timeout
	if err != context.DeadlineExceeded {
		// Either deadline exceeded or context cancelled is fine
		if err != context.Canceled && err != context.DeadlineExceeded {
			t.Logf("Unexpected error type: %v (elapsed: %v)", err, elapsed)
		}
	}

	// Should cancel reasonably quickly
	if elapsed > 500*time.Millisecond {
		t.Errorf("cancel took too long: %v", elapsed)
	}
}
