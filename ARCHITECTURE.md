# ttylag Architecture

High-level architectural overview for new developers.

## Overview

ttylag is a PTY (pseudo-terminal) wrapper that simulates slow/laggy network connections or the archetypal serial console experience. It sits between your terminal and a child process, applying configurable delays and bandwidth limits.

## 1. Data Flow Diagram

```mermaid
flowchart TB
    subgraph UserTerminal["User's Terminal"]
        stdin["stdin (raw mode)"]
        stdout["stdout"]
    end

    subgraph ttylag["ttylag Process"]
        direction TB
        upShaper["UP Shaper<br/>delay + jitter + rate limit"]
        downShaper["DOWN Shaper<br/>delay + jitter + rate limit"]
        ptmx["PTY Master<br/>(file descriptor)"]
    end

    subgraph Child["Child Process (e.g. bash, vim)"]
        pts["PTY Slave<br/>(stdin/stdout/stderr)"]
    end

    stdin -->|"keystrokes"| upShaper
    upShaper -->|"shaped"| ptmx
    ptmx <-->|"kernel PTY"| pts
    pts -->|"output"| ptmx
    ptmx -->|"reads"| downShaper
    downShaper -->|"shaped"| stdout

    style upShaper fill:#f9f,stroke:#333
    style downShaper fill:#9ff,stroke:#333
```

**Key insight**: The PTY master and slave are connected by the kernel. ttylag controls the master side; the child process sees the slave as its terminal.

## 2. Process & Goroutine Structure

```mermaid
flowchart LR
    subgraph Main["main goroutine"]
        init["Parse flags<br/>Create PTY<br/>Set raw mode"]
        wait["Wait for child<br/>Cleanup"]
    end

    subgraph Goroutines["Spawned Goroutines"]
        g1["stdin reader<br/>→ upShaper"]
        g2["upShaper output<br/>→ PTY master"]
        g3["PTY master reader<br/>→ downShaper"]
        g4["downShaper output<br/>→ stdout"]
        g5["Signal handler<br/>(SIGWINCH, SIGINT)"]
    end

    init --> g1 & g2 & g3 & g4 & g5
    g1 & g2 & g3 & g4 & g5 --> wait
```

## 3. Shaper Pipeline (per direction)

Each shaper processes data through these stages:

```mermaid
flowchart LR
    A["Input<br/>(bytes)"] --> B["Delay Queue<br/>hold until due_time"]
    B --> C["Chunk Splitter<br/>break into pieces"]
    C --> D["Rate Limiter<br/>(token bucket OR<br/>wire serialization)"]
    D --> E["Frame Buffer<br/>(optional coalesce)"]
    E --> F["Output<br/>(writes)"]

    style B fill:#ffa,stroke:#333
    style D fill:#afa,stroke:#333
```

**Two rate limiting modes:**
- **Token Bucket** (default): Bursty output, like packet networks
- **Serial Mode** (`--serial`): Smooth byte-by-byte, like RS-232

## 4. PTY Mechanics (OS Level)

```mermaid
flowchart TB
    subgraph Userspace
        ttylag["ttylag<br/>(owns PTY master)"]
        child["Child Process<br/>(attached to PTY slave)"]
    end

    subgraph Kernel
        ptmx_dev["/dev/ptmx<br/>Master FD"]
        pts_dev["/dev/pts/N<br/>Slave FD"]
        termios["termios<br/>(line discipline)"]
    end

    ttylag -->|"open"| ptmx_dev
    child -->|"stdin/stdout/stderr"| pts_dev
    ptmx_dev <-->|"kernel buffer"| pts_dev
    pts_dev --- termios

    style Kernel fill:#eef,stroke:#339
```

**What the kernel does:**
- Provides bidirectional byte stream between master/slave
- Handles terminal window size (TIOCSWINSZ)
- Line discipline (echo, signals) happens at slave side

**What ttylag does:**
- Opens PTY master, forks child on slave
- Puts user's terminal in raw mode (bypass line discipline)
- Intercepts all I/O through the master fd

## 5. Signal Flow

```mermaid
sequenceDiagram
    participant User as User Terminal
    participant ttylag
    participant Child

    Note over User,Child: Ctrl+C typed
    User->>ttylag: ETX byte (0x03) via stdin
    ttylag->>ttylag: Pass through UP shaper
    ttylag->>Child: Write to PTY master
    Note over Child: Kernel delivers SIGINT<br/>to child's process group

    Note over User,Child: Terminal resized
    User->>ttylag: SIGWINCH
    ttylag->>ttylag: ioctl(TIOCSWINSZ) on PTY master
    Note over Child: Kernel delivers SIGWINCH to child
```

**Key point**: Control characters (Ctrl+C, Ctrl+Z) flow through ttylag as raw bytes because the user's terminal is in raw mode. The PTY slave's line discipline converts them to signals for the child.

## 6. Shutdown Sequence

```mermaid
sequenceDiagram
    participant Child
    participant ttylag
    participant User as User Terminal

    Child->>Child: exit(N)
    Child->>ttylag: PTY master gets EOF/EIO
    ttylag->>ttylag: Cancel upstream context
    ttylag->>ttylag: Wait for downstream to drain<br/>(up to 30s for slow rates)
    ttylag->>User: Restore terminal mode
    ttylag->>ttylag: Exit with child's exit code
```

## Quick Reference

| Component | File | Purpose |
|-----------|------|---------|
| CLI & orchestration | `main.go` | Flag parsing, PTY setup, signal handling |
| Traffic shaping | `shaper.go` | Delay, jitter, rate limiting, chunking |
| Connection profiles | `profiles.go` | Preset configurations (3g, dialup, etc.) |

## Further Reading

- `DESIGN.md` - Detailed design decisions and rationale
- `AGENTS.md` - Development conventions and testing
