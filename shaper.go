package main

import (
	"context"
	"io"
	"math/rand"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Buffer and limit sizes
const (
	readBufferSize  = 4096      // Size of read buffer for source data
	readChanBuffer  = 16        // Channel buffer for read data
	maxBurstSize    = 65536     // Cap burst at 64KB to prevent huge initial bursts
	initialWakeTime = time.Hour // Initial wake timer (will be reset immediately)
)

// ShaperConfig holds configuration for one direction of traffic shaping.
type ShaperConfig struct {
	Delay      time.Duration // Base delay applied to all data
	Jitter     time.Duration // Jitter range: uniform distribution [-jitter, +jitter]
	Rate       int64         // Bytes per second (0 = unlimited)
	Burst      int           // Token bucket burst size (0 = auto-calculate)
	ChunkSize  int           // Max bytes per write (0 = unlimited)
	FrameTime  time.Duration // Coalesce output interval (0 = disabled)
	Seed       int64         // Random seed for jitter (0 = use current time)
	SerialMode bool          // Use wire serialization model (smooth) vs token bucket (bursty)
}

// delayedChunk represents data waiting to be released after its due time.
type delayedChunk struct {
	data    []byte
	dueTime time.Time
}

// Shaper applies delay, jitter, rate limiting, chunking, and framing to a byte stream.
// It reads from an input channel and writes shaped data to an output writer.
//
// Two rate limiting modes are supported:
//   - Token bucket (default): Bursty output, feels like packet networks
//   - Wire serialization (SerialMode): Smooth byte-by-byte output, feels like serial links
type Shaper struct {
	config     ShaperConfig
	rng        *rand.Rand
	limiter    *rate.Limiter // Used in token bucket mode
	wireFreeAt time.Time     // Used in serial mode: when the wire becomes free
	mu         sync.Mutex
}

// NewShaper creates a new Shaper with the given configuration.
func NewShaper(cfg ShaperConfig) *Shaper {
	// Initialize random source
	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	// Initialize rate limiter if rate is set and NOT in serial mode
	// Serial mode uses wire serialization instead of token bucket
	var limiter *rate.Limiter
	if cfg.Rate > 0 && !cfg.SerialMode {
		// Calculate burst size: at least one chunk, or 100ms of data
		burst := cfg.Burst
		if burst == 0 {
			burst = int(cfg.Rate / 10) // 100ms of data
			if cfg.ChunkSize > 0 && cfg.ChunkSize > burst {
				burst = cfg.ChunkSize
			}
			if burst < 1 {
				burst = 1
			}
			// Cap burst at 64KB to prevent huge initial bursts
			if burst > maxBurstSize {
				burst = maxBurstSize
			}
		}
		limiter = rate.NewLimiter(rate.Limit(cfg.Rate), burst)
	}

	return &Shaper{
		config:     cfg,
		rng:        rng,
		limiter:    limiter,
		wireFreeAt: time.Now(),
	}
}

// randomJitter returns a random duration in [-jitter, +jitter].
func (s *Shaper) randomJitter() time.Duration {
	if s.config.Jitter == 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Generate random value in range [0, 2*jitter], then subtract jitter
	jitterRange := int64(s.config.Jitter) * 2
	jitter := time.Duration(s.rng.Int63n(jitterRange)) - s.config.Jitter
	return jitter
}

// splitChunks splits data into chunks of at most ChunkSize bytes.
// If ChunkSize is 0, returns the data as a single chunk.
func (s *Shaper) splitChunks(data []byte) [][]byte {
	if s.config.ChunkSize <= 0 || len(data) <= s.config.ChunkSize {
		return [][]byte{data}
	}

	var chunks [][]byte
	for len(data) > 0 {
		end := s.config.ChunkSize
		if end > len(data) {
			end = len(data)
		}
		// Make a copy to avoid issues with buffer reuse
		chunk := make([]byte, end)
		copy(chunk, data[:end])
		chunks = append(chunks, chunk)
		data = data[end:]
	}
	return chunks
}

// Run processes data from src and writes shaped output to dst.
// It blocks until ctx is cancelled or src returns an error (including io.EOF).
// The function handles delay, jitter, rate limiting, chunking, and framing.
func (s *Shaper) Run(ctx context.Context, src io.Reader, dst io.Writer) error {
	// Channel for data read from source
	readCh := make(chan []byte, readChanBuffer)
	readErr := make(chan error, 1)

	// Start reader goroutine
	go func() {
		defer close(readCh)
		buf := make([]byte, readBufferSize)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				// Make a copy to avoid buffer reuse issues
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case readCh <- data:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					readErr <- err
				}
				return
			}
		}
	}()

	// Delay queue for pending data
	var delayQueue []delayedChunk

	// Frame buffer for coalescing
	var frameBuffer []byte
	var frameTicker *time.Ticker
	var frameTickCh <-chan time.Time

	if s.config.FrameTime > 0 {
		frameTicker = time.NewTicker(s.config.FrameTime)
		frameTickCh = frameTicker.C
		defer frameTicker.Stop()
	}

	// Timer for waking up when next chunk is due
	wakeTimer := time.NewTimer(initialWakeTime)
	defer wakeTimer.Stop()

	for {
		// Calculate next wake time based on delay queue
		var nextWake time.Time
		if len(delayQueue) > 0 {
			nextWake = delayQueue[0].dueTime
			if !wakeTimer.Stop() {
				select {
				case <-wakeTimer.C:
				default:
				}
			}
			wakeTimer.Reset(time.Until(nextWake))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-readErr:
			return err

		case data, ok := <-readCh:
			if !ok {
				// Source closed, drain remaining data
				return s.drainQueue(ctx, dst, delayQueue, frameBuffer)
			}
			// Calculate due time with jitter
			jitter := s.randomJitter()
			totalDelay := s.config.Delay + jitter
			if totalDelay < 0 {
				totalDelay = 0
			}
			dueTime := time.Now().Add(totalDelay)
			delayQueue = append(delayQueue, delayedChunk{data: data, dueTime: dueTime})

		case <-wakeTimer.C:
			// Process ready chunks
			frameBuffer = s.processReadyChunks(ctx, dst, &delayQueue, frameBuffer)

		case <-frameTickCh:
			// Emit frame buffer
			if len(frameBuffer) > 0 {
				if err := s.writeWithRateLimit(ctx, dst, frameBuffer); err != nil {
					return err
				}
				frameBuffer = nil
			}
		}
	}
}

// processReadyChunks writes chunks whose due time has passed.
// Returns updated frame buffer.
func (s *Shaper) processReadyChunks(ctx context.Context, dst io.Writer, queue *[]delayedChunk, frameBuffer []byte) []byte {
	now := time.Now()
	for len(*queue) > 0 && (*queue)[0].dueTime.Before(now) {
		chunk := (*queue)[0]
		*queue = (*queue)[1:]

		// Apply chunking
		pieces := s.splitChunks(chunk.data)
		for _, piece := range pieces {
			if s.config.FrameTime > 0 {
				// Buffer for framing
				frameBuffer = append(frameBuffer, piece...)
			} else {
				// Write immediately with rate limiting
				if err := s.writeWithRateLimit(ctx, dst, piece); err != nil {
					// Log error but continue
					return frameBuffer
				}
			}
		}
	}
	return frameBuffer
}

// writeWithRateLimit writes data respecting the configured rate limiting mode.
// In serial mode, it uses wire serialization (smooth byte-by-byte timing).
// In default mode, it uses token bucket (bursty output).
func (s *Shaper) writeWithRateLimit(ctx context.Context, dst io.Writer, data []byte) error {
	if s.config.Rate == 0 {
		// No rate limiting
		_, err := dst.Write(data)
		return err
	}

	if s.config.SerialMode {
		return s.writeWithWireSerialization(ctx, dst, data)
	}
	return s.writeWithTokenBucket(ctx, dst, data)
}

// writeWithWireSerialization writes data using wire serialization timing.
// This simulates a serial link where each byte takes a fixed time to transmit,
// producing smooth, character-by-character output.
func (s *Shaper) writeWithWireSerialization(ctx context.Context, dst io.Writer, data []byte) error {
	for _, b := range data {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Calculate when this byte can be transmitted
		// Time per byte = 1 / Rate (in seconds)
		byteTime := time.Duration(float64(time.Second) / float64(s.config.Rate))

		s.mu.Lock()
		now := time.Now()
		if now.After(s.wireFreeAt) {
			s.wireFreeAt = now
		}
		s.wireFreeAt = s.wireFreeAt.Add(byteTime)
		transmitAt := s.wireFreeAt
		s.mu.Unlock()

		// Wait until it's time to transmit
		waitTime := time.Until(transmitAt)
		if waitTime > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
			}
		}

		// Write the single byte
		if _, err := dst.Write([]byte{b}); err != nil {
			return err
		}
	}
	return nil
}

// writeWithTokenBucket writes data using token bucket rate limiting.
// This produces bursty output typical of packet networks.
func (s *Shaper) writeWithTokenBucket(ctx context.Context, dst io.Writer, data []byte) error {
	if s.limiter == nil {
		_, err := dst.Write(data)
		return err
	}

	// Write in pieces no larger than burst size
	burst := s.limiter.Burst()
	for len(data) > 0 {
		toWrite := len(data)
		if toWrite > burst {
			toWrite = burst
		}

		// Wait for tokens
		if err := s.limiter.WaitN(ctx, toWrite); err != nil {
			return err
		}

		_, err := dst.Write(data[:toWrite])
		if err != nil {
			return err
		}
		data = data[toWrite:]
	}
	return nil
}

// drainQueue writes any remaining data in the delay queue and frame buffer.
// This function ignores context cancellation to ensure all buffered data is written.
func (s *Shaper) drainQueue(ctx context.Context, dst io.Writer, queue []delayedChunk, frameBuffer []byte) error {
	// Use a background context for draining - we want to finish writing
	// even if the main context is cancelled
	drainCtx := context.Background()

	// Wait for all delayed chunks to become ready and write them
	for len(queue) > 0 {
		chunk := queue[0]
		queue = queue[1:]

		// Wait until due time
		waitTime := time.Until(chunk.dueTime)
		if waitTime > 0 {
			time.Sleep(waitTime)
		}

		// Apply chunking and write
		pieces := s.splitChunks(chunk.data)
		for _, piece := range pieces {
			if s.config.FrameTime > 0 {
				frameBuffer = append(frameBuffer, piece...)
			} else {
				if err := s.writeWithRateLimit(drainCtx, dst, piece); err != nil {
					return err
				}
			}
		}
	}

	// Write any remaining frame buffer
	if len(frameBuffer) > 0 {
		if err := s.writeWithRateLimit(drainCtx, dst, frameBuffer); err != nil {
			return err
		}
	}

	return nil
}

// Copy is a convenience function that creates a Shaper and runs it.
// It shapes data from src to dst according to cfg.
func Copy(ctx context.Context, dst io.Writer, src io.Reader, cfg ShaperConfig) error {
	shaper := NewShaper(cfg)
	return shaper.Run(ctx, src, dst)
}
