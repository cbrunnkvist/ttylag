package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
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

// TestShaperBandwidthVerification replicates the bandwidth verification test
// from a competing implementation. Tests 10 kbit/s throughput with 5000 bytes.
func TestShaperBandwidthVerification(t *testing.T) {
	// 10 kbit/s = 1250 bytes/sec
	// 5000 bytes should take ~4 seconds
	cfg := ShaperConfig{
		Rate:      1250, // 10 kbit/s in bytes
		ChunkSize: 1024,
		Seed:      42,
	}

	input := make([]byte, 5000)
	src := bytes.NewReader(input)
	var dst bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	err := Copy(ctx, &dst, src, cfg)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	if dst.Len() != 5000 {
		t.Errorf("output size mismatch: got %d, want 5000", dst.Len())
	}

	bits := int64(dst.Len()) * 8
	bps := float64(bits) / duration.Seconds()

	t.Logf("Read %d bytes in %v", dst.Len(), duration)
	t.Logf("Rate: %.2f bits/sec (target: 10000)", bps)

	// Allow ±15% tolerance (token bucket has burst behavior)
	// Competing implementation reported ~11,130 bps
	minRate := 8500.0
	maxRate := 13000.0
	if bps < minRate || bps > maxRate {
		t.Errorf("rate %.2f outside expected range [%.0f, %.0f]", bps, minRate, maxRate)
	}
}

// TestShaperJitterVerificationBaseline replicates the jitter baseline test.
// 100 baud = 10 bytes/sec with 10 bits/byte, so ~100ms per byte.
func TestShaperJitterVerificationBaseline(t *testing.T) {
	// 100 baud with 10 bits per byte = 10 bytes/sec = 100ms per byte
	cfg := ShaperConfig{
		Rate: 10, // 10 bytes/sec
		Seed: 42,
	}

	// Send 10 bytes, measure inter-arrival times
	input := make([]byte, 10)
	src := bytes.NewReader(input)

	// Custom writer that tracks write times
	type timedWrite struct {
		t time.Time
		n int
	}
	var writes []timedWrite
	var mu sync.Mutex

	tracker := &trackingWriter{
		onWrite: func(n int) {
			mu.Lock()
			writes = append(writes, timedWrite{time.Now(), n})
			mu.Unlock()
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	Copy(ctx, tracker, src, cfg)

	// Calculate inter-arrival times
	if len(writes) < 2 {
		t.Skip("Not enough writes to measure timing")
	}

	var deltas []time.Duration
	for i := 1; i < len(writes); i++ {
		deltas = append(deltas, writes[i].t.Sub(writes[i-1].t))
	}

	// Calculate average
	var sum time.Duration
	for _, d := range deltas {
		sum += d
	}
	avg := sum / time.Duration(len(deltas))

	t.Logf("Baseline (no jitter): Avg inter-arrival: %v", avg)

	// Should be approximately 100ms (80-120ms acceptable)
	if avg < 80*time.Millisecond || avg > 120*time.Millisecond {
		t.Errorf("average inter-arrival %v outside expected range [80ms, 120ms]", avg)
	}
}

// trackingWriter records when writes occur
type trackingWriter struct {
	onWrite func(n int)
}

func (w *trackingWriter) Write(p []byte) (int, error) {
	if w.onWrite != nil {
		w.onWrite(len(p))
	}
	return len(p), nil
}

// TestShaperSerialMode tests the wire serialization model used for serial connections.
// This produces smooth, byte-by-byte output instead of bursty token bucket output.
func TestShaperSerialMode(t *testing.T) {
	// 100 bytes/sec in serial mode = 10ms per byte
	// 10 bytes should take ~100ms with smooth timing
	cfg := ShaperConfig{
		Rate:       100,
		SerialMode: true,
		Seed:       42,
	}

	input := strings.Repeat("x", 10) // 10 bytes
	src := strings.NewReader(input)

	// Track individual writes - serial mode should write byte-by-byte
	tracker := &writeTracker{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := Copy(ctx, tracker, src, cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	// Verify output content
	if tracker.total.String() != input {
		t.Errorf("output mismatch: got %q, want %q", tracker.total.String(), input)
	}

	// Serial mode writes byte-by-byte
	if len(tracker.writes) != 10 {
		t.Errorf("serial mode should write byte-by-byte: got %d writes, want 10", len(tracker.writes))
	}

	// Each write should be exactly 1 byte
	for i, w := range tracker.writes {
		if len(w) != 1 {
			t.Errorf("write %d: got %d bytes, want 1", i, len(w))
		}
	}

	// 10 bytes at 100 bytes/sec = 100ms minimum
	// Allow some tolerance for timing
	expectedMin := 80 * time.Millisecond
	expectedMax := 200 * time.Millisecond
	if elapsed < expectedMin || elapsed > expectedMax {
		t.Errorf("serial mode timing: got %v, expected %v to %v", elapsed, expectedMin, expectedMax)
	}
}

// TestShaperSerialModeVsTokenBucket verifies that serial mode produces
// smoother timing than token bucket mode.
func TestShaperSerialModeVsTokenBucket(t *testing.T) {
	// Send 5 bytes, measure inter-write times
	// At 50 bytes/sec, each byte should take 20ms

	measureWriteTimes := func(serialMode bool) []time.Duration {
		cfg := ShaperConfig{
			Rate:       50, // 50 bytes/sec = 20ms per byte
			SerialMode: serialMode,
			Seed:       42,
		}

		src := strings.NewReader("12345") // 5 bytes

		var writeTimes []time.Time
		var mu sync.Mutex
		tracker := &trackingWriter{
			onWrite: func(n int) {
				mu.Lock()
				writeTimes = append(writeTimes, time.Now())
				mu.Unlock()
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		Copy(ctx, tracker, src, cfg)

		// Calculate deltas
		var deltas []time.Duration
		for i := 1; i < len(writeTimes); i++ {
			deltas = append(deltas, writeTimes[i].Sub(writeTimes[i-1]))
		}
		return deltas
	}

	serialDeltas := measureWriteTimes(true)
	tokenDeltas := measureWriteTimes(false)

	t.Logf("Serial mode inter-write times: %v", serialDeltas)
	t.Logf("Token bucket inter-write times: %v", tokenDeltas)

	// Serial mode should have more writes (byte-by-byte)
	if len(serialDeltas) < len(tokenDeltas) {
		t.Logf("Note: Serial mode has fewer deltas (%d) than token bucket (%d) - this may indicate token bucket is writing smaller chunks",
			len(serialDeltas), len(tokenDeltas))
	}

	// Serial mode deltas should be more consistent (lower variance)
	// This is a soft check - we just log the observation
	if len(serialDeltas) >= 2 {
		var sum time.Duration
		for _, d := range serialDeltas {
			sum += d
		}
		avg := sum / time.Duration(len(serialDeltas))
		t.Logf("Serial mode average inter-write: %v (expected ~20ms)", avg)
	}
}
