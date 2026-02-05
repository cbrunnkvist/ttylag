# AGENTS.md - ttylag

> Userspace PTY wrapper that simulates laggy/slow network connections for terminal applications.

## Quick Reference

```bash
# Build
go build -o ttylag .

# Run all tests
go test -v ./...

# Run a single test
go test -v -run TestShaperDelay ./...

# Run tests matching pattern
go test -v -run "TestShaper.*" ./...

# Run with race detector
go test -race ./...

# Smoke test (includes CLI verification)
./smoke_test.sh

# Lint (if available)
golangci-lint run
```

## Project Structure

```
ttylag/
├── main.go              # CLI parsing, PTY management, signal handling
├── shaper.go            # Core Shaper type: delay/jitter/rate/chunk/frame
├── shaper_test.go       # Unit tests for Shaper
├── ttylag.1             # Generated man page
├── cmd/
│   ├── genman/          # Man page generator
│   └── timing_analysis/ # Tool to analyze script recordings for timing verification
├── smoke_test.sh        # Integration smoke tests
├── DESIGN.md            # Detailed design document
└── README.md            # User documentation
```

## Code Style Guidelines

### Imports

Group imports in this order, separated by blank lines:
1. Standard library
2. External dependencies
3. Internal packages (none currently)

```go
import (
    "context"
    "fmt"
    "time"

    "github.com/creack/pty"
    "golang.org/x/term"
)
```

### Formatting

- Use `gofmt` (standard Go formatting)
- Tabs for indentation
- No line length limit, but prefer readability

### Types and Naming

- **Exported types**: PascalCase (`ShaperConfig`, `Shaper`)
- **Unexported types**: camelCase (`delayedChunk`)
- **Config structs**: Group related fields with comments
- **Constructors**: `NewXxx(cfg XxxConfig) *Xxx`

```go
// ShaperConfig holds configuration for one direction of traffic shaping.
type ShaperConfig struct {
    Delay     time.Duration // Base delay applied to all data
    Jitter    time.Duration // Jitter range: uniform distribution [-jitter, +jitter]
    Rate      int64         // Bytes per second (0 = unlimited)
}
```

### Error Handling

- Return errors, don't panic (except truly unrecoverable situations)
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Check errors immediately after the call that produces them
- For CLI errors, print to stderr and exit with non-zero code

```go
if err != nil {
    return nil, fmt.Errorf("invalid --rtt: %w", err)
}
```

### Context Usage

- Pass `context.Context` as first parameter to functions that may block
- Use context for cancellation, not for passing values
- Always respect context cancellation in loops

```go
func (s *Shaper) Run(ctx context.Context, src io.Reader, dst io.Writer) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        // ...
        }
    }
}
```

### Concurrency

- Use `sync.WaitGroup` for goroutine coordination
- Use channels for communication between goroutines
- Use `sync.Mutex` for protecting shared state
- Always close channels from the sender side

### Comments

- Package-level doc comments on exported types and functions
- Inline comments for non-obvious logic
- Use `// TODO:` for planned improvements

## Testing Conventions

### Test Naming

```go
func TestShaperDelay(t *testing.T)      // Test specific feature
func TestShaperNoConfig(t *testing.T)   // Test edge case
func TestShaperContextCancel(t *testing.T)  // Test cancellation
```

### Test Structure

- Use table-driven tests when testing multiple inputs
- Use `t.Fatalf` for setup failures, `t.Errorf` for test assertions
- Include timing tolerances for time-sensitive tests (±20ms typical)

```go
// Allow ±20ms tolerance for timing
expectedMin := 80 * time.Millisecond
expectedMax := 150 * time.Millisecond
if elapsed < expectedMin || elapsed > expectedMax {
    t.Errorf("delay out of range: got %v, expected %v to %v", elapsed, expectedMin, expectedMax)
}
```

### Helper Types for Testing

```go
// writeTracker records each write for inspection
type writeTracker struct {
    writes [][]byte
    total  bytes.Buffer
}
```

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/creack/pty` | PTY creation and window size management |
| `github.com/spf13/pflag` | GNU-style flag parsing |
| `golang.org/x/term` | Terminal raw mode |
| `golang.org/x/time/rate` | Token bucket rate limiting |

## Platform Notes

- **Build constraint**: `//go:build !windows` - Windows not supported
- **Signals**: Uses `syscall.SIGWINCH`, `SIGINT`, `SIGTERM` (Unix-only)
- **PTY**: Requires Unix PTY support (Linux, macOS)

## Common Patterns

### CLI Flag Parsing

Uses `spf13/pflag` for GNU-style flag parsing (single dash for short flags, double dash for long):

```go
fs := flag.NewFlagSet("ttylag", flag.ContinueOnError)
rtt := fs.String("rtt", "", "Round-trip time")
jitter := fs.StringP("jitter", "j", "", "Jitter") // -j, --jitter
fs.BoolVarP(&cfg.Help, "help", "h", false, "Show help")
```

pflag handles `--` separator automatically.

### Bandwidth Parsing

Accepts SI units (k=1000, not 1024): `56kbit`, `1mbit`, `100KB`

### Graceful Shutdown

1. Wait for child process to exit
2. Cancel context to signal goroutines
3. Close PTY master
4. Wait for goroutines with timeout
5. Restore terminal state

## Debugging Tips

- Use `--seed` flag for deterministic jitter in testing
- Record sessions with `script -r` for timing analysis
- Check terminal state: `stty -a` before/after running
- Reset terminal if corrupted: `reset` or `stty sane`
