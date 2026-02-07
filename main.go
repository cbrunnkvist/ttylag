//go:build !windows
// +build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	flag "github.com/spf13/pflag"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

var version = "0.1.3"

// Default terminal dimensions when stdin is not a TTY
const (
	defaultTermCols = 80
	defaultTermRows = 24
)

// Shutdown timing
const (
	drainTimeout       = 30 * time.Second       // Max time to wait for downstream to drain
	goroutineExitWait  = 500 * time.Millisecond // Max time to wait for goroutines to exit
	defaultBitsPerByte = 10                     // 8N1 serial: 1 start + 8 data + 1 stop
)

// Config holds all command-line configuration
type Config struct {
	// Delays
	RTT       time.Duration
	UpDelay   time.Duration
	DownDelay time.Duration

	// Jitter
	Jitter     time.Duration
	UpJitter   time.Duration
	DownJitter time.Duration

	// Bandwidth (bytes per second)
	UpRate   int64
	DownRate int64

	// Chunking and framing
	ChunkSize int
	FrameTime time.Duration

	// Serial mode
	Serial      int
	BitsPerByte int
	SerialMode  bool // Use wire serialization model (smooth byte-by-byte) vs token bucket (bursty)

	// Misc
	Seed         int64
	Profile      string
	Help         bool
	Version      bool
	ListProfiles bool

	// Command to run
	Command []string
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if cfg.Version {
		fmt.Printf("ttylag %s\n", version)
		os.Exit(0)
	}

	if cfg.ListProfiles {
		printProfiles()
		os.Exit(0)
	}

	if len(cfg.Command) == 0 {
		fmt.Fprintln(os.Stderr, "ttylag: simulate laggy terminal connections")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "error: no command specified")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: ttylag [flags] -- <command> [args...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Quick examples:")
		fmt.Fprintln(os.Stderr, "  ttylag --serial 9600 -- bash      # 9600 baud serial")
		fmt.Fprintln(os.Stderr, "  ttylag --profile 3g -- htop       # 3G mobile network")
		fmt.Fprintln(os.Stderr, "  ttylag --rtt 200ms -- vim         # 200ms round-trip latency")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Run 'ttylag --help' for full options.")
		os.Exit(1)
	}

	exitCode := run(cfg)
	os.Exit(exitCode)
}

func parseFlags() (*Config, error) {
	cfg := &Config{
		BitsPerByte: defaultBitsPerByte,
	}

	// Custom flag set to handle -- separator (pflag handles this automatically)
	fs := flag.NewFlagSet("ttylag", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.SortFlags = false // Preserve definition order in help

	// Define flags with GNU-style short/long options
	// Long-only flags (no short equivalent)
	rtt := fs.String("rtt", "", "Round-trip time (split evenly up/down)")
	upDelay := fs.String("up-delay", "", "Upstream delay (user→child)")
	downDelay := fs.String("down-delay", "", "Downstream delay (child→user)")
	jitter := fs.StringP("jitter", "j", "", "Jitter for both directions")
	upJitter := fs.String("up-jitter", "", "Upstream jitter")
	downJitter := fs.String("down-jitter", "", "Downstream jitter")
	upRate := fs.StringP("up", "u", "", "Upstream bandwidth limit (e.g., 56kbit)")
	downRate := fs.StringP("down", "d", "", "Downstream bandwidth limit")
	chunkSize := fs.IntP("chunk", "c", 0, "Max bytes per write (0=unlimited)")
	frameTime := fs.String("frame", "", "Coalesce output interval (e.g., 40ms)")
	serial := fs.IntP("serial", "s", 0, "Serial port speed in bps (e.g., 9600)")
	bitsPerByte := fs.Int("bits-per-byte", defaultBitsPerByte, "Bits per byte for serial (default 10 for 8N1)")
	seed := fs.Int64("seed", 0, "Random seed for jitter (0=random)")
	profile := fs.StringP("profile", "p", "", "Connection profile (see below)")
	fs.BoolVarP(&cfg.Help, "help", "h", false, "Show help")
	fs.BoolVarP(&cfg.Version, "version", "v", false, "Show version")
	fs.BoolVarP(&cfg.ListProfiles, "list-profiles", "L", false, "List available profiles")

	// Custom usage
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "ttylag - simulate laggy terminal connections")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Wraps a command in a PTY and applies configurable delay, jitter,")
		fmt.Fprintln(os.Stderr, "and bandwidth limits to simulate slow network connections.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: ttylag [flags] -- <command> [args...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  ttylag --serial 9600 -- bash")
		fmt.Fprintln(os.Stderr, "  ttylag --rtt 400ms --jitter 80ms --down 80kbit -- htop")
		fmt.Fprintln(os.Stderr, "  ttylag --profile 3g -- ssh user@host")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Bandwidth formats: 100, 100bps, 56kbit, 56k, 1mbit, 100KB")
		fmt.Fprintln(os.Stderr, "  k=1000 (SI units), not 1024")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Profiles:")
		fmt.Fprintln(os.Stderr, "  Serial:    9600, 2400")
		fmt.Fprintln(os.Stderr, "  Dial-up:   dialup")
		fmt.Fprintln(os.Stderr, "  Mobile:    edge, 3g, lte, lte-poor")
		fmt.Fprintln(os.Stderr, "  Wired:     dsl, cable")
		fmt.Fprintln(os.Stderr, "  Satellite: satellite, satellite-geo")
		fmt.Fprintln(os.Stderr, "  WiFi:      wifi-poor, wifi-bad")
		fmt.Fprintln(os.Stderr, "  Other:     intercontinental")
	}

	// Parse flags (pflag handles -- separator automatically)
	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	// Get command args (everything after --)
	cfg.Command = fs.Args()

	if cfg.Help {
		fs.Usage()
		return cfg, flag.ErrHelp
	}

	// Apply profile first (can be overridden by explicit flags)
	if *profile != "" {
		p, ok := profiles[*profile]
		if !ok {
			return nil, fmt.Errorf("unknown profile: %s", *profile)
		}
		cfg.RTT = p.RTT
		cfg.Jitter = p.Jitter
		cfg.UpRate = p.UpRate
		cfg.DownRate = p.DownRate
		cfg.SerialMode = p.SerialMode
	}

	// Parse duration flags
	for _, d := range []struct {
		value    string
		flagName string
		dst      *time.Duration
	}{
		{*rtt, "rtt", &cfg.RTT},
		{*upDelay, "up-delay", &cfg.UpDelay},
		{*downDelay, "down-delay", &cfg.DownDelay},
		{*jitter, "jitter", &cfg.Jitter},
		{*upJitter, "up-jitter", &cfg.UpJitter},
		{*downJitter, "down-jitter", &cfg.DownJitter},
		{*frameTime, "frame", &cfg.FrameTime},
	} {
		if err := parseDuration(d.value, d.flagName, d.dst); err != nil {
			return nil, err
		}
	}

	// Parse bandwidth flags
	if *upRate != "" {
		rate, err := parseBandwidth(*upRate)
		if err != nil {
			return nil, fmt.Errorf("invalid --up: %w", err)
		}
		cfg.UpRate = rate
	}
	if *downRate != "" {
		rate, err := parseBandwidth(*downRate)
		if err != nil {
			return nil, fmt.Errorf("invalid --down: %w", err)
		}
		cfg.DownRate = rate
	}

	// Handle serial mode
	cfg.Serial = *serial
	cfg.BitsPerByte = *bitsPerByte
	if cfg.Serial > 0 {
		bytesPerSec := int64(cfg.Serial / cfg.BitsPerByte)
		// Serial sets both directions if not explicitly set
		if cfg.UpRate == 0 {
			cfg.UpRate = bytesPerSec
		}
		if cfg.DownRate == 0 {
			cfg.DownRate = bytesPerSec
		}
		// Serial mode uses wire serialization model for authentic feel
		cfg.SerialMode = true
	}

	// Other values
	cfg.ChunkSize = *chunkSize
	cfg.Seed = *seed

	// Apply RTT to delays if explicit delays not set
	if cfg.RTT > 0 {
		halfRTT := cfg.RTT / 2
		if cfg.UpDelay == 0 {
			cfg.UpDelay = halfRTT
		}
		if cfg.DownDelay == 0 {
			cfg.DownDelay = halfRTT
		}
	}

	// Apply global jitter if per-direction jitter not set
	if cfg.Jitter > 0 {
		if cfg.UpJitter == 0 {
			cfg.UpJitter = cfg.Jitter
		}
		if cfg.DownJitter == 0 {
			cfg.DownJitter = cfg.Jitter
		}
	}

	return cfg, nil
}

// parseDuration parses a duration flag value into dst if non-empty.
// Returns an error with the flag name if parsing fails.
func parseDuration(s string, flagName string, dst *time.Duration) error {
	if s == "" {
		return nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid --%s: %w", flagName, err)
	}
	*dst = d
	return nil
}

// parseBandwidth parses bandwidth strings like "56kbit", "1mbit", "100KB"
// Returns bytes per second. Uses SI units (k=1000).
func parseBandwidth(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, nil
	}

	// Regex to parse: number + optional unit
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([a-z/]*)$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid bandwidth format: %s", s)
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}

	unit := matches[2]
	var multiplier float64 = 1
	isBytes := false

	switch unit {
	case "", "bps", "bit", "bits":
		multiplier = 1
	case "k", "kbit", "kbps":
		multiplier = 1000
	case "m", "mbit", "mbps":
		multiplier = 1000000
	case "g", "gbit", "gbps":
		multiplier = 1000000000
	case "b", "bps/s", "byte", "bytes":
		multiplier = 1
		isBytes = true
	case "kb", "kb/s", "kbps/s":
		multiplier = 1000
		isBytes = true
	case "mb", "mb/s", "mbps/s":
		multiplier = 1000000
		isBytes = true
	default:
		return 0, fmt.Errorf("unknown bandwidth unit: %s", unit)
	}

	bits := value * multiplier
	if isBytes {
		return int64(bits), nil // Already in bytes
	}
	return int64(bits / 8), nil // Convert bits to bytes
}

// getTerminalSize returns the terminal dimensions, or defaults if unavailable.
func getTerminalSize() (width, height int) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		w, h, err := term.GetSize(int(os.Stdin.Fd()))
		if err == nil {
			return w, h
		}
		fmt.Fprintf(os.Stderr, "warning: could not get terminal size: %v (using %dx%d)\n", err, defaultTermCols, defaultTermRows)
	}
	return defaultTermCols, defaultTermRows
}

// makeShaperConfigs creates upstream and downstream shaper configurations from CLI config.
func makeShaperConfigs(cfg *Config) (up, down ShaperConfig) {
	up = ShaperConfig{
		Delay:      cfg.UpDelay,
		Jitter:     cfg.UpJitter,
		Rate:       cfg.UpRate,
		ChunkSize:  cfg.ChunkSize,
		FrameTime:  cfg.FrameTime,
		Seed:       cfg.Seed,
		SerialMode: cfg.SerialMode,
	}
	down = ShaperConfig{
		Delay:      cfg.DownDelay,
		Jitter:     cfg.DownJitter,
		Rate:       cfg.DownRate,
		ChunkSize:  cfg.ChunkSize,
		FrameTime:  cfg.FrameTime,
		Seed:       cfg.Seed + 1, // Different seed for each direction
		SerialMode: cfg.SerialMode,
	}
	return up, down
}

// waitWithTimeout waits for a WaitGroup with a timeout.
// Returns true if all goroutines finished, false if timeout.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func run(cfg *Config) int {
	// Get terminal info
	stdinIsTerminal := term.IsTerminal(int(os.Stdin.Fd()))
	width, height := getTerminalSize()

	// Create the command
	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)

	// Start with PTY
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error starting pty: %v\n", err)
		return 1
	}

	// When stdout is not a terminal (piping/redirecting), disable ONLCR on the
	// PTY to prevent CR+LF conversion. Without this, piped output contains
	// CR+LF (0d 0a) instead of just LF (0a), causing display artifacts.
	stdoutIsTerminal := term.IsTerminal(int(os.Stdout.Fd()))
	if !stdoutIsTerminal {
		if termios, err := unix.IoctlGetTermios(int(ptmx.Fd()), ioctlGetTermios); err == nil {
			termios.Oflag &^= unix.ONLCR
			unix.IoctlSetTermios(int(ptmx.Fd()), ioctlSetTermios, termios)
		}
	}

	// Set terminal to raw mode (only if BOTH stdin and stdout are terminals)
	var oldState *term.State
	if stdinIsTerminal && stdoutIsTerminal {
		var err error
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			ptmx.Close()
			fmt.Fprintf(os.Stderr, "error setting raw mode: %v\n", err)
			return 1
		}
	}

	// Ensure terminal restoration on exit (only if we changed it)
	restoreTerminal := func() {
		if oldState != nil {
			term.Restore(int(os.Stdin.Fd()), oldState)
		}
	}
	defer restoreTerminal()

	// Separate contexts for upstream and downstream
	// Upstream can be cancelled immediately when child exits
	// Downstream needs to drain its buffer before stopping
	upCtx, upCancel := context.WithCancel(context.Background())
	downCtx, downCancel := context.WithCancel(context.Background())
	defer upCancel()
	defer downCancel()

	// Signal handling (uses upstream context for signals)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)

	// WaitGroup for goroutines
	var wg sync.WaitGroup

	// Shaper configs
	upConfig, downConfig := makeShaperConfigs(cfg)

	// Upstream: stdin -> shaper -> PTY
	wg.Add(1)
	go func() {
		defer wg.Done()
		upShaper := NewShaper(upConfig)
		upShaper.Run(upCtx, os.Stdin, ptmx)
	}()

	// Downstream: PTY -> shaper -> stdout
	downDone := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(downDone)
		downShaper := NewShaper(downConfig)
		downShaper.Run(downCtx, ptmx, os.Stdout)
	}()

	// Signal handler goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-upCtx.Done():
				return
			case sig := <-sigCh:
				switch sig {
				case syscall.SIGWINCH:
					// Propagate terminal size change to PTY
					w, h, err := term.GetSize(int(os.Stdin.Fd()))
					if err == nil {
						pty.Setsize(ptmx, &pty.Winsize{
							Rows: uint16(h),
							Cols: uint16(w),
						})
					}
				case syscall.SIGINT, syscall.SIGTERM:
					// Forward signal to child process group
					if cmd.Process != nil {
						syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
					}
				}
			}
		}
	}()

	// Wait for child process
	waitErr := cmd.Wait()

	// Cancel upstream context to stop upstream shaper (which may be blocked on stdin)
	upCancel()

	// Wait for downstream shaper to finish naturally (it will get EOF from PTY)
	// with a generous timeout for rate-limited connections
	select {
	case <-downDone:
		// Downstream finished draining
	case <-time.After(drainTimeout):
		// Timeout - cancel downstream and proceed
		downCancel()
	}

	// Wait for all goroutines with a short timeout
	waitWithTimeout(&wg, goroutineExitWait)

	// Now close PTY (for cleanup)
	ptmx.Close()

	// Restore terminal before exiting
	restoreTerminal()

	// Determine exit code
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}
