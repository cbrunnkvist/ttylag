# ttylag Design Document

A userspace PTY wrapper that simulates laggy/slow network connections for local terminal applications.

## Overview

`ttylag` interposes between the user's terminal and a child process running on a pseudo-terminal (PTY), applying configurable delays, jitter, and bandwidth limits to simulate conditions like "SSH over a bad link" or "9600 baud serial connection".

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           User's Terminal                                │
│                     (stdin/stdout in raw mode)                          │
└─────────────────────────────────────────────────────────────────────────┘
                    │                              ▲
                    │ keystrokes                   │ display output
                    ▼                              │
           ┌────────────────┐              ┌────────────────┐
           │  UP Shaper     │              │  DOWN Shaper   │
           │  (user→child)  │              │  (child→user)  │
           │                │              │                │
           │ • delay+jitter │              │ • delay+jitter │
           │ • rate limit   │              │ • rate limit   │
           │ • chunking     │              │ • chunking     │
           │ • framing      │              │ • framing      │
           └────────────────┘              └────────────────┘
                    │                              ▲
                    │                              │
                    ▼                              │
┌─────────────────────────────────────────────────────────────────────────┐
│                         PTY Master (ttylag)                             │
└─────────────────────────────────────────────────────────────────────────┘
                    │                              ▲
                    │ write to slave               │ read from slave
                    ▼                              │
┌─────────────────────────────────────────────────────────────────────────┐
│                    PTY Slave (child process)                            │
│                    e.g., bash, htop, vim                                │
└─────────────────────────────────────────────────────────────────────────┘
```

## CLI Specification

### Usage

```
ttylag [flags] -- <command> [args...]
```

The `--` separator is required before the command to avoid flag parsing ambiguity.

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--rtt` | duration | 0 | Round-trip time (split evenly between up/down delays) |
| `--up-delay` | duration | 0 | Fixed delay for user→child direction (overrides RTT/2) |
| `--down-delay` | duration | 0 | Fixed delay for child→user direction (overrides RTT/2) |
| `--jitter` | duration | 0 | Jitter applied to both directions (uniform distribution) |
| `--up-jitter` | duration | 0 | Jitter for user→child only (overrides --jitter) |
| `--down-jitter` | duration | 0 | Jitter for child→user only (overrides --jitter) |
| `--up` | bandwidth | 0 | Bandwidth limit user→child (0 = unlimited) |
| `--down` | bandwidth | 0 | Bandwidth limit child→user (0 = unlimited) |
| `--chunk` | int | 0 | Max bytes per write (0 = no chunking) |
| `--frame` | duration | 0 | Coalesce output into bursts every N ms (0 = no framing) |
| `--serial` | int | 0 | Serial port speed in bps (convenience preset) |
| `--bits-per-byte` | int | 10 | Bits per byte for serial calculation (8N1 = 10) |
| `--seed` | int64 | (random) | Random seed for deterministic jitter |
| `--help` | bool | false | Show help message |

### Bandwidth Format

Bandwidth values accept SI suffixes (k=1000, not 1024):

- `100` or `100bps` - 100 bits/second
- `9600bps` - 9600 bits/second  
- `56kbit` or `56k` - 56,000 bits/second
- `1mbit` or `1m` - 1,000,000 bits/second
- `100KB` or `100KBps` - 100,000 bytes/second (×8 for bits)

### Duration Format

Standard Go duration format: `100ms`, `1s`, `1.5s`, `500us`

### Convenience Presets

#### Serial Mode (`--serial <bps>`)

Simulates serial port timing. For 8N1 encoding: 1 start bit + 8 data bits + 1 stop bit = 10 bits per byte.

```
--serial 9600
```

Equivalent to:
```
--up 9600bps --down 9600bps
```

Bytes/second = bps / bits-per-byte = 9600 / 10 = 960 bytes/sec

#### Optional Named Profiles

| Profile | RTT | Jitter | Down | Up | Description |
|---------|-----|--------|------|-----|-------------|
| `dialup` | 150ms | 30ms | 56kbit | 33.6kbit | 56k modem |
| `edge` | 500ms | 100ms | 200kbit | 100kbit | 2G/EDGE mobile |
| `3g` | 200ms | 50ms | 1mbit | 384kbit | 3G mobile |
| `dsl` | 50ms | 10ms | 8mbit | 1mbit | Basic DSL |

## Key Design Decisions

### 1. Jitter Distribution

**Choice**: Uniform distribution in range `[-jitter, +jitter]`

**Rationale**: Simple, predictable, easy to reason about. The total delay is:
```
actual_delay = base_delay + uniform(-jitter, +jitter)
```

Clamped to minimum of 0 (negative delays are impossible).

**Trade-off**: When `jitter > base_delay`, the distribution becomes truncated/biased toward positive values. This is acceptable and documented.

### 2. Shaping Pipeline Order

For each direction, data flows through the shaper in this order:

```
Input → Delay Queue → Chunk Splitter → Rate Limiter → Frame Coalescer → Output
```

1. **Delay Queue**: Holds bytes until `arrival_time + delay + jitter` has passed
2. **Chunk Splitter**: Breaks data into pieces of at most `--chunk` bytes
3. **Rate Limiter**: Token bucket controls throughput (bytes/second)
4. **Frame Coalescer**: If `--frame > 0`, batches output to emit every N ms

### 3. Token Bucket Configuration

- **Rate**: Configured bandwidth in bytes/second
- **Burst**: `max(chunk_size, rate * 0.1)` — allows 100ms worth of burst, or one chunk minimum
- **Implementation**: Use `golang.org/x/time/rate.Limiter` for correctness

### 4. Buffer Management

**Bounded buffers** to prevent unbounded memory growth:

- **Per-direction queue**: 64KB max (typical PTY buffer size)
- **Backpressure**: When queue is full, block reads from source
- **Behavior**: If child spews faster than shaped rate allows, reads block

### 5. Framing Behavior

When `--frame` is set:

- Output is held in a buffer
- Every `frame` duration, the entire buffer is written as one burst
- Chunks are still respected within the burst
- Rate limiting still applies to the burst

### 6. Signal Handling

| Signal | Behavior |
|--------|----------|
| SIGWINCH | Propagate terminal size to PTY slave |
| SIGINT/SIGTERM | Forward to child process group, cleanup, exit |
| SIGQUIT | Same as SIGINT (allows Ctrl-\ to reach child via PTY) |

User-typed Ctrl-C/Ctrl-\ flow through the PTY as bytes (raw mode), reaching the child naturally.

### 7. Terminal Restoration

Terminal state MUST be restored even on:
- Normal exit
- Child crash
- User kills ttylag
- Panic

Implementation: 
1. `defer` for normal paths
2. Signal handler for SIGINT/SIGTERM
3. Panic recovery wrapper

### 8. Exit Code Propagation

`ttylag` exits with the child process's exit code when possible:
- Child exits normally: use child's exit code
- Child killed by signal: exit with 128 + signal number (Unix convention)
- ttylag error (e.g., couldn't spawn PTY): exit 1

## Go Implementation Plan

### Package Structure

Single `main` package with logical separation:

```
ttylag/
├── main.go           # Entry point, CLI parsing, orchestration
├── shaper.go         # Shaper type implementing delay/rate/chunk/frame
├── shaper_test.go    # Unit tests for Shaper
├── pty.go            # PTY spawning and management
├── terminal.go       # Raw mode, restoration, SIGWINCH
├── bandwidth.go      # Bandwidth string parsing
├── go.mod
├── go.sum
├── README.md
└── DESIGN.md
```

### Core Types

```go
// ShaperConfig holds configuration for one direction
type ShaperConfig struct {
    Delay      time.Duration  // Base delay
    Jitter     time.Duration  // Jitter range (uniform -j to +j)
    Rate       int64          // Bytes per second (0 = unlimited)
    Burst      int            // Token bucket burst size
    ChunkSize  int            // Max bytes per write (0 = unlimited)
    FrameTime  time.Duration  // Coalesce interval (0 = disabled)
    Seed       int64          // Random seed for jitter
}

// Shaper applies delay/jitter/rate/chunk/frame to a byte stream
type Shaper struct {
    config  ShaperConfig
    input   chan []byte       // Incoming data
    output  chan []byte       // Shaped data ready to write
    done    chan struct{}     // Shutdown signal
    rng     *rand.Rand        // Jitter RNG
    limiter *rate.Limiter     // Token bucket
}

// delayedChunk represents bytes waiting to be released
type delayedChunk struct {
    data    []byte
    dueTime time.Time
}
```

### Goroutine Architecture

```
Main Goroutine
    │
    ├── spawns PTY with child process
    ├── sets up signal handlers
    ├── sets terminal to raw mode
    │
    ├── goroutine: stdin → upShaper.input
    │       (reads from os.Stdin, sends to shaper)
    │
    ├── goroutine: upShaper.output → ptyMaster
    │       (writes shaped data to PTY)
    │
    ├── goroutine: ptyMaster → downShaper.input
    │       (reads from PTY, sends to shaper)
    │
    ├── goroutine: downShaper.output → stdout
    │       (writes shaped data to os.Stdout)
    │
    ├── goroutine: upShaper.run()
    │       (delay queue + rate limiting + chunking + framing)
    │
    ├── goroutine: downShaper.run()
    │       (delay queue + rate limiting + chunking + framing)
    │
    └── waits for child exit, triggers cleanup
```

### Shaper Pseudocode

```go
func (s *Shaper) run(ctx context.Context) {
    var delayQueue []delayedChunk
    var frameBuffer []byte
    frameTicker := time.NewTicker(s.config.FrameTime) // if framing enabled
    
    for {
        select {
        case <-ctx.Done():
            return
            
        case data := <-s.input:
            // Calculate due time with jitter
            jitter := s.randomJitter()
            dueTime := time.Now().Add(s.config.Delay + jitter)
            delayQueue = append(delayQueue, delayedChunk{data, dueTime})
            
        case <-frameTicker.C:
            // Flush frame buffer if framing enabled
            if len(frameBuffer) > 0 {
                s.output <- frameBuffer
                frameBuffer = nil
            }
            
        default:
            // Process delay queue
            now := time.Now()
            for len(delayQueue) > 0 && delayQueue[0].dueTime.Before(now) {
                chunk := delayQueue[0]
                delayQueue = delayQueue[1:]
                
                // Apply chunking
                for _, piece := range s.splitChunks(chunk.data) {
                    // Apply rate limiting
                    s.limiter.WaitN(ctx, len(piece))
                    
                    if s.config.FrameTime > 0 {
                        // Buffer for framing
                        frameBuffer = append(frameBuffer, piece...)
                    } else {
                        // Emit immediately
                        s.output <- piece
                    }
                }
            }
            
            // Sleep until next event
            s.sleepUntilNextEvent(delayQueue, frameTicker)
        }
    }
}
```

### Shutdown Sequence

1. Child process exits (detected via `cmd.Wait()`)
2. Close `upShaper.input` to signal upstream done
3. Drain `downShaper.output` for reasonable time (100ms)
4. Cancel context to stop all goroutines
5. Close PTY master
6. Restore terminal state
7. Exit with child's exit code

## Testing Plan

### Unit Tests (shaper_test.go)

1. **Delay Test**: Verify bytes are held for correct duration (±10ms tolerance)
2. **Jitter Test**: Verify jitter distribution is within expected range
3. **Rate Limit Test**: Verify throughput matches configured rate (±10%)
4. **Chunk Test**: Verify output writes are at most chunk size
5. **Frame Test**: Verify output bursts at frame intervals

### Integration Tests (Manual)

```bash
# Test 1: Serial mode feels slow
ttylag --serial 9600 -- bash
# Type commands, verify visible character-by-character output

# Test 2: Laggy htop
ttylag --rtt 600ms --jitter 100ms --down 50kbit --up 10kbit --chunk 64 -- htop
# Verify laggy, bursty updates; q to quit still works

# Test 3: Window resize
ttylag --rtt 200ms -- vim
# Resize terminal window, verify vim redraws correctly

# Test 4: Clean exit
ttylag --serial 9600 -- bash -c "exit 42"
echo $?  # Should be 42

# Test 5: Ctrl-C propagation
ttylag --rtt 100ms -- bash
# Run `sleep 100`, press Ctrl-C, verify it interrupts
```

### Automated Timing Tests

```bash
# RTT verification (should take ~100ms)
time echo "test" | ttylag --rtt 100ms -- cat

# Bandwidth verification
dd if=/dev/zero bs=1000 count=10 2>/dev/null | \
  timeout 15s ttylag --up 1000bps -- cat | wc -c
# Should output 10000 bytes in ~10 seconds (1000 bytes/sec = 800 bits/sec effective)
```

### Script-Based Timing Verification

Use macOS/Linux `script` command to record sessions with timing data:

```bash
# Record a session with timing (macOS)
script -r test.rec timeout 5 ./ttylag --rtt 200ms -- sh -c 'echo START; sleep 1; echo END'

# Play back the recording in real-time to verify lag
script -p test.rec

# For more detailed analysis, use the timing_analysis tool:
go run ./cmd/timing_analysis/main.go test.rec
```

The recording captures output timing, allowing verification that:
- Delay matches configured RTT (within tolerance)
- Bandwidth limiting creates expected gaps between outputs
- Jitter is within configured range

## Limitations

1. **stdout/stderr merged**: PTY combines both streams; cannot be separated
2. **No Windows support**: PTY concepts don't map to Windows console
3. **UTF-8 chunking**: Chunks may split multi-byte characters (terminal handles gracefully)
4. **Jitter truncation**: When jitter > delay, distribution is biased positive
5. **No packet loss simulation**: Only delay/bandwidth, not drops (could be added)

## Dependencies

- `github.com/creack/pty` - PTY creation and window size management
- `golang.org/x/term` - Terminal raw mode
- `golang.org/x/time/rate` - Token bucket rate limiting

All are well-maintained, minimal, and appropriate for the task.
