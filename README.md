# ttylag

A userspace PTY wrapper that simulates laggy/slow network connections for local terminal applications.

Make any local terminal app feel like it's running over SSH on a bad connection, a 9600 baud serial line, or a flaky mobile network.

**Author:** Conny Brunnkvist <cbrunnkvist@gmail.com>

## Installation

```bash
go install github.com/user/ttylag@latest
```

Or build from source:

```bash
git clone https://github.com/user/ttylag
cd ttylag
go build -o ttylag .
```

### Man Page

Generate and install the man page:

```bash
# Generate man page
go run cmd/genman/main.go > ttylag.1

# Install (Linux/macOS)
sudo install -m 644 ttylag.1 /usr/local/share/man/man1/

# View
man ttylag
```

## Quick Start

```bash
# Simulate a 9600 baud serial connection
ttylag --serial 9600 -- bash

# Simulate a laggy SSH connection
ttylag --rtt 400ms --jitter 80ms --down 80kbit --up 20kbit -- htop

# Use a preset profile
ttylag --profile 3g -- vim
```

## Usage

```
ttylag [flags] -- <command> [args...]
```

The `--` separator is required before the command.

### Flags

| Flag | Short | Description | Example |
|------|-------|-------------|---------|
| `--rtt` | | Round-trip time (split evenly between up/down) | `--rtt 200ms` |
| `--up-delay` | | Fixed upstream delay (user→child) | `--up-delay 100ms` |
| `--down-delay` | | Fixed downstream delay (child→user) | `--down-delay 100ms` |
| `--jitter` | `-j` | Jitter for both directions | `-j 50ms` |
| `--up-jitter` | | Upstream jitter only | `--up-jitter 30ms` |
| `--down-jitter` | | Downstream jitter only | `--down-jitter 30ms` |
| `--up` | `-u` | Upstream bandwidth limit | `-u 56kbit` |
| `--down` | `-d` | Downstream bandwidth limit | `-d 1mbit` |
| `--chunk` | `-c` | Max bytes per write | `-c 64` |
| `--frame` | | Coalesce output interval | `--frame 40ms` |
| `--serial` | `-s` | Serial port speed (convenience) | `-s 9600` |
| `--bits-per-byte` | | Bits per byte for serial (default 10) | `--bits-per-byte 10` |
| `--seed` | | Random seed for jitter | `--seed 42` |
| `--profile` | `-p` | Use preset profile | `-p dialup` |
| `--help` | `-h` | Show help | |
| `--version` | `-v` | Show version | |

### Bandwidth Formats

Bandwidth values use SI units (k=1000, not 1024):

- `100` or `100bps` - 100 bits/second
- `56kbit` or `56k` - 56,000 bits/second
- `1mbit` or `1m` - 1,000,000 bits/second
- `100KB` - 100,000 bytes/second

### Preset Profiles

#### Serial
| Profile | Down | Up | Description |
|---------|------|-----|-------------|
| `9600` | 9600bps | 9600bps | 9600 baud serial |
| `2400` | 2400bps | 2400bps | 2400 baud serial |

#### Dial-up
| Profile | RTT | Jitter | Down | Up | Description |
|---------|-----|--------|------|-----|-------------|
| `dialup` | 150ms | 30ms | 56kbit | 33.6kbit | 56k modem |

#### Mobile
| Profile | RTT | Jitter | Down | Up | Description |
|---------|-----|--------|------|-----|-------------|
| `edge` | 500ms | 100ms | 200kbit | 100kbit | 2G/EDGE |
| `3g` | 200ms | 50ms | 1mbit | 384kbit | 3G |
| `lte` | 50ms | 15ms | 20mbit | 5mbit | Good LTE |
| `lte-poor` | 150ms | 50ms | 2mbit | 500kbit | Poor LTE signal |

#### Wired
| Profile | RTT | Jitter | Down | Up | Description |
|---------|-----|--------|------|-----|-------------|
| `dsl` | 50ms | 10ms | 8mbit | 1mbit | Basic DSL |
| `cable` | 30ms | 5ms | 50mbit | 5mbit | Cable modem |

#### Satellite
| Profile | RTT | Jitter | Down | Up | Description |
|---------|-----|--------|------|-----|-------------|
| `satellite` | 600ms | 50ms | 25mbit | 5mbit | Modern (Starlink-ish) |
| `satellite-geo` | 700ms | 100ms | 10mbit | 2mbit | Traditional VSAT |

#### WiFi
| Profile | RTT | Jitter | Down | Up | Description |
|---------|-----|--------|------|-----|-------------|
| `wifi-poor` | 80ms | 40ms | 2mbit | 1mbit | Poor WiFi |
| `wifi-bad` | 200ms | 100ms | 500kbit | 250kbit | Very bad WiFi |

#### Other
| Profile | RTT | Jitter | Down | Up | Description |
|---------|-----|--------|------|-----|-------------|
| `intercontinental` | 250ms | 30ms | 10mbit | 5mbit | Long-distance (US↔Asia) |

## Examples

### Test how your TUI app behaves on slow connections

```bash
# See how htop renders over a slow link
ttylag --rtt 600ms --jitter 100ms --down 50kbit --up 10kbit -- htop

# Test vim over simulated 3G
ttylag --profile 3g -- vim myfile.txt
```

### Simulate serial terminal

```bash
# Classic 9600 baud
ttylag --serial 9600 -- bash

# Even slower - 2400 baud
ttylag --serial 2400 -- bash
```

### Bursty output with framing

```bash
# Output arrives in 40ms bursts
ttylag --rtt 100ms --frame 40ms --chunk 32 -- bash
```

### Testing with deterministic jitter

```bash
# Use a fixed seed for reproducible behavior
ttylag --rtt 200ms --jitter 50ms --seed 12345 -- bash
```

## How It Works

```
┌─────────────────────────────────────────────────────────┐
│                   User's Terminal                        │
│               (stdin/stdout in raw mode)                 │
└─────────────────────────────────────────────────────────┘
                │                          ▲
                │ keystrokes               │ display
                ▼                          │
        ┌──────────────┐           ┌──────────────┐
        │  UP Shaper   │           │ DOWN Shaper  │
        │              │           │              │
        │ • delay      │           │ • delay      │
        │ • jitter     │           │ • jitter     │
        │ • rate limit │           │ • rate limit │
        │ • chunking   │           │ • chunking   │
        └──────────────┘           └──────────────┘
                │                          ▲
                ▼                          │
┌─────────────────────────────────────────────────────────┐
│                    PTY Master (ttylag)                   │
└─────────────────────────────────────────────────────────┘
                │                          ▲
                ▼                          │
┌─────────────────────────────────────────────────────────┐
│              PTY Slave (child process)                   │
│              e.g., bash, htop, vim                       │
└─────────────────────────────────────────────────────────┘
```

ttylag creates a pseudo-terminal (PTY) and runs your command attached to it. All input from your keyboard goes through an "upstream shaper" before reaching the child process. All output from the child goes through a "downstream shaper" before reaching your screen.

Each shaper applies:
1. **Delay** - Fixed base delay
2. **Jitter** - Random variation (uniform distribution)
3. **Rate limiting** - Token bucket bandwidth control
4. **Chunking** - Split data into small pieces
5. **Framing** - Coalesce output into periodic bursts

## Testing

### Run Tests

```bash
# Unit tests
go test -v ./...

# Smoke test
./smoke_test.sh
```

### Verify Timing with Script Recording

You can use `script` to record sessions with timing data for verification:

```bash
# Record a session (macOS)
script -r test.rec timeout 5 ./ttylag --rtt 200ms -- sh -c 'echo START; sleep 1; echo END'

# Play back to verify timing
script -p test.rec

# Analyze timing data
go run ./cmd/timing_analysis/main.go test.rec
```

## Troubleshooting

### Terminal is messed up after exit

ttylag restores terminal state on exit, but if it crashes or is killed with SIGKILL, you may need to reset your terminal:

```bash
reset
# or
stty sane
```

### Child process doesn't receive Ctrl-C

This should work normally - Ctrl-C is passed through the PTY as bytes. If the child isn't responding, the shaped delay may make it seem slow.

### TUI app doesn't resize properly

ttylag handles SIGWINCH and propagates terminal size changes to the PTY. If resizing seems delayed, that's expected - the resize happens immediately, but the app's redraw output goes through the shaper.

### Window resize causes redraw glitches

With high latency and low bandwidth, the terminal may display partial redraws. This is intentional - it simulates what actually happens over slow links.

## Limitations

- **stdout/stderr merged**: PTY combines both streams; they cannot be separated
- **No Windows support**: PTY concepts don't map to Windows console
- **UTF-8 chunking**: Chunks may split multi-byte characters (terminals handle this gracefully)
- **No packet loss**: Only delay/bandwidth simulation, not drops

## Platform Support

- **Linux**: Full support
- **macOS**: Full support
- **Windows**: Not supported (no PTY)

## Dependencies

- [github.com/creack/pty](https://github.com/creack/pty) - PTY creation
- [github.com/spf13/pflag](https://github.com/spf13/pflag) - GNU-style flag parsing
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) - Terminal raw mode
- [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate) - Token bucket rate limiting

## FAQ

### Why is it called `ttylag` when it admits to being a "PTY lag simulator"?

Because you can't remember "ptylag", and if you do, pronouncing that causes you to spit on the screen.

N.b. the marketing team also considered calling it "lagshim" or "stutty" (ha-ha) but, come on, this is not a Ruby project ;-)

## License

MIT License - see [LICENSE](LICENSE) for details.
