//go:build ignore

// genman generates the ttylag man page.
// Usage: go run cmd/genman/main.go > ttylag.1
package main

import (
	"fmt"
	"os"
)

func main() {
	// Use a fixed date for reproducible builds/CI
	date := "February 2026"

	manpage := fmt.Sprintf(`.TH TTYLAG 1 "%s" "ttylag 0.1.0" "User Commands"
.SH NAME
ttylag \- simulate laggy terminal connections
.SH SYNOPSIS
.B ttylag
[\fIflags\fR] \-\- \fIcommand\fR [\fIargs...\fR]
.SH DESCRIPTION
.B ttylag
wraps a command in a pseudo-terminal (PTY) and applies configurable delay,
jitter, and bandwidth limits to simulate slow network connections.
.PP
All input from your keyboard goes through an "upstream shaper" before reaching
the child process. All output from the child goes through a "downstream shaper"
before reaching your screen.
.SH OPTIONS
.TP
.B \-\-rtt \fIduration\fR
Round-trip time, split evenly between upstream and downstream delays.
Example: \fB\-\-rtt 200ms\fR
.TP
.B \-\-up\-delay \fIduration\fR
Fixed upstream delay (user → child).
.TP
.B \-\-down\-delay \fIduration\fR
Fixed downstream delay (child → user).
.TP
.BR \-j ", " \-\-jitter " \fIduration\fR"
Jitter applied to both directions. Adds uniform random variation in the range
[\-jitter, +jitter] to each data transfer.
.TP
.B \-\-up\-jitter \fIduration\fR
Upstream jitter only.
.TP
.B \-\-down\-jitter \fIduration\fR
Downstream jitter only.
.TP
.BR \-u ", " \-\-up " \fIbandwidth\fR"
Upstream bandwidth limit. Example: \fB\-\-up 56kbit\fR
.TP
.BR \-d ", " \-\-down " \fIbandwidth\fR"
Downstream bandwidth limit. Example: \fB\-\-down 1mbit\fR
.TP
.BR \-c ", " \-\-chunk " \fIbytes\fR"
Maximum bytes per write (0 = unlimited). Splits data into smaller pieces.
.TP
.B \-\-frame \fIduration\fR
Coalesce output into periodic bursts. Example: \fB\-\-frame 40ms\fR
.TP
.BR \-s ", " \-\-serial " \fIbps\fR"
Serial port speed in bits per second. Convenience flag that sets bandwidth
limits based on baud rate. Example: \fB\-\-serial 9600\fR
.TP
.B \-\-bits\-per\-byte \fIn\fR
Bits per byte for serial calculation (default: 10 for 8N1).
.TP
.B \-\-seed \fIint\fR
Random seed for jitter (0 = random). Useful for reproducible testing.
.TP
.BR \-p ", " \-\-profile " \fIname\fR"
Use a preset connection profile. See \fBPROFILES\fR section.
.TP
.BR \-h ", " \-\-help
Show help message.
.TP
.BR \-v ", " \-\-version
Show version information.
.SH BANDWIDTH FORMATS
Bandwidth values use SI units (k=1000, not 1024):
.TP
.B 100\fR or \fB100bps
100 bits per second
.TP
.B 56kbit\fR or \fB56k
56,000 bits per second
.TP
.B 1mbit\fR or \fB1m
1,000,000 bits per second
.TP
.B 100KB
100,000 bytes per second
.SH PROFILES
Preset connection profiles are available:
.SS Serial
.TP
.B 9600
9600 baud serial connection
.TP
.B 2400
2400 baud serial connection
.SS Dial-up
.TP
.B dialup
56k modem (150ms RTT, 30ms jitter, 56kbit down, 33.6kbit up)
.SS Mobile
.TP
.B edge
2G/EDGE mobile (500ms RTT, 100ms jitter, 200kbit down, 100kbit up)
.TP
.B 3g
3G mobile (200ms RTT, 50ms jitter, 1mbit down, 384kbit up)
.TP
.B lte
Good LTE (50ms RTT, 15ms jitter, 20mbit down, 5mbit up)
.TP
.B lte\-poor
Poor LTE signal (150ms RTT, 50ms jitter, 2mbit down, 500kbit up)
.SS Wired
.TP
.B dsl
Basic DSL (50ms RTT, 10ms jitter, 8mbit down, 1mbit up)
.TP
.B cable
Cable modem (30ms RTT, 5ms jitter, 50mbit down, 5mbit up)
.SS Satellite
.TP
.B satellite
Modern satellite like Starlink (600ms RTT, 50ms jitter, 25mbit down, 5mbit up)
.TP
.B satellite\-geo
Traditional geostationary VSAT (700ms RTT, 100ms jitter, 10mbit down, 2mbit up)
.SS WiFi
.TP
.B wifi\-poor
Poor WiFi (80ms RTT, 40ms jitter, 2mbit down, 1mbit up)
.TP
.B wifi\-bad
Very bad WiFi (200ms RTT, 100ms jitter, 500kbit down, 250kbit up)
.SS Other
.TP
.B intercontinental
Long-distance connection, e.g., US to Asia (250ms RTT, 30ms jitter, 10mbit down, 5mbit up)
.SH EXAMPLES
Simulate a 9600 baud serial connection:
.PP
.RS
.nf
ttylag \-\-serial 9600 \-\- bash
.fi
.RE
.PP
Simulate a laggy SSH connection:
.PP
.RS
.nf
ttylag \-\-rtt 400ms \-\-jitter 80ms \-\-down 80kbit \-\- htop
.fi
.RE
.PP
Use a preset profile:
.PP
.RS
.nf
ttylag \-\-profile 3g \-\- vim myfile.txt
.fi
.RE
.PP
Bursty output with framing:
.PP
.RS
.nf
ttylag \-\-rtt 100ms \-\-frame 40ms \-\-chunk 32 \-\- bash
.fi
.RE
.PP
Deterministic jitter for testing:
.PP
.RS
.nf
ttylag \-\-rtt 200ms \-\-jitter 50ms \-\-seed 12345 \-\- bash
.fi
.RE
.SH EXIT STATUS
.B ttylag
exits with the exit status of the wrapped command, or 1 if an error occurs
before or during execution.
.SH ENVIRONMENT
.B ttylag
does not use any environment variables. It passes the current environment
to the child process unchanged.
.SH NOTES
.IP \(bu 2
Requires a real terminal (TTY) to run. Will error if stdin is not a terminal.
.IP \(bu 2
stdout and stderr from the child process are merged (PTY limitation).
.IP \(bu 2
Terminal state is restored on exit. If the terminal becomes corrupted
(e.g., after SIGKILL), run \fBreset\fR or \fBstty sane\fR to fix it.
.IP \(bu 2
SIGWINCH is handled to propagate terminal size changes to the child process.
.IP \(bu 2
SIGINT and SIGTERM are forwarded to the child process.
.SH BUGS
.IP \(bu 2
Chunking may split multi-byte UTF-8 characters (terminals handle this gracefully).
.IP \(bu 2
No packet loss simulation (only delay/bandwidth).
.SH AUTHOR
Written for testing terminal applications under adverse network conditions.
.SH SEE ALSO
.BR ssh (1),
.BR script (1),
.BR tc (8)
`, date)

	fmt.Fprint(os.Stdout, manpage)
}
