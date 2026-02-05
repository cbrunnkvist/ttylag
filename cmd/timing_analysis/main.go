package main

// timing_test.go - Analyze script recordings to verify ttylag timing
//
// Usage:
//   1. Record a session:
//      script -r recording.txt ./ttylag --rtt 200ms -- sh -c 'for i in 1 2 3; do echo $i; sleep 0.1; done'
//
//   2. Analyze (on macOS the timing is embedded in the recording):
//      go run timing_test.go recording.txt
//
// This is a manual verification tool, not an automated test.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"
)

// scriptHeader is the header format for macOS script -r recordings
// The format is proprietary but we can detect timing gaps in output
type timingEntry struct {
	timestamp time.Duration
	data      []byte
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run timing_analysis.go <recording-file>")
		fmt.Println("")
		fmt.Println("Record a session first:")
		fmt.Println("  script -r test.rec timeout 5 ./ttylag --rtt 200ms -- bash")
		fmt.Println("")
		fmt.Println("Then analyze:")
		fmt.Println("  go run timing_analysis.go test.rec")
		os.Exit(1)
	}

	filename := os.Args[1]
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Analyzing: %s (%d bytes)\n\n", filename, len(data))

	// Try to detect format
	if bytes.HasPrefix(data, []byte("TTYREC")) || detectTTYRec(data) {
		analyzeTTYRec(data)
	} else if detectMacOSScript(data) {
		analyzeMacOSScript(data)
	} else {
		// Fall back to simple analysis
		analyzeSimple(data)
	}
}

func detectTTYRec(data []byte) bool {
	// ttyrec format: 12-byte header (sec, usec, len) followed by data
	if len(data) < 12 {
		return false
	}
	// Check if first record looks valid
	sec := binary.LittleEndian.Uint32(data[0:4])
	usec := binary.LittleEndian.Uint32(data[4:8])
	length := binary.LittleEndian.Uint32(data[8:12])

	// Sanity checks
	return sec < 2000000000 && usec < 1000000 && length < 1000000 && int(length)+12 <= len(data)
}

func detectMacOSScript(data []byte) bool {
	// macOS script -r format starts with "Script started on"
	return bytes.Contains(data[:min(200, len(data))], []byte("Script started"))
}

func analyzeTTYRec(data []byte) {
	fmt.Println("Detected: ttyrec format")
	fmt.Println("")

	var entries []timingEntry
	var firstTime time.Duration
	offset := 0

	for offset+12 <= len(data) {
		sec := binary.LittleEndian.Uint32(data[offset : offset+4])
		usec := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		length := binary.LittleEndian.Uint32(data[offset+8 : offset+12])

		if offset+12+int(length) > len(data) {
			break
		}

		ts := time.Duration(sec)*time.Second + time.Duration(usec)*time.Microsecond
		if len(entries) == 0 {
			firstTime = ts
		}

		entries = append(entries, timingEntry{
			timestamp: ts - firstTime,
			data:      data[offset+12 : offset+12+int(length)],
		})

		offset += 12 + int(length)
	}

	printTimingAnalysis(entries)
}

func analyzeMacOSScript(data []byte) {
	fmt.Println("Detected: macOS script format")
	fmt.Println("")
	fmt.Println("Note: macOS script -r embeds timing in a binary format.")
	fmt.Println("For detailed timing analysis, use 'script -p <file>' to replay.")
	fmt.Println("")

	// Show content preview
	fmt.Println("Content preview:")
	fmt.Println("================")

	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineCount := 0
	for scanner.Scan() && lineCount < 20 {
		line := scanner.Text()
		// Filter out binary data
		if isPrintable(line) {
			fmt.Println(line)
			lineCount++
		}
	}

	if lineCount == 0 {
		fmt.Println("(binary data - use 'script -p' to replay)")
	}
}

func analyzeSimple(data []byte) {
	fmt.Println("Detected: Plain text recording")
	fmt.Println("")

	// Just show the content
	reader := bufio.NewReader(bytes.NewReader(data))
	lineCount := 0
	for lineCount < 50 {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if isPrintable(line) {
			fmt.Print(line)
			lineCount++
		}
	}
}

func printTimingAnalysis(entries []timingEntry) {
	if len(entries) == 0 {
		fmt.Println("No timing entries found")
		return
	}

	fmt.Printf("Found %d timing entries\n\n", len(entries))

	// Calculate gaps between entries
	var gaps []time.Duration
	for i := 1; i < len(entries); i++ {
		gap := entries[i].timestamp - entries[i-1].timestamp
		gaps = append(gaps, gap)
	}

	// Show significant gaps (> 10ms)
	fmt.Println("Significant timing gaps (>10ms):")
	fmt.Println("================================")

	significantCount := 0
	for i, gap := range gaps {
		if gap > 10*time.Millisecond {
			preview := entries[i+1].data
			if len(preview) > 40 {
				preview = preview[:40]
			}
			fmt.Printf("  %s: +%v -> %q\n", entries[i].timestamp, gap, sanitize(preview))
			significantCount++
		}
	}

	if significantCount == 0 {
		fmt.Println("  (none found)")
	}

	// Statistics
	fmt.Println("")
	fmt.Println("Timing Statistics:")
	fmt.Println("==================")
	fmt.Printf("  Total duration: %v\n", entries[len(entries)-1].timestamp)
	fmt.Printf("  Total entries: %d\n", len(entries))

	if len(gaps) > 0 {
		var totalGap time.Duration
		var maxGap time.Duration
		for _, g := range gaps {
			totalGap += g
			if g > maxGap {
				maxGap = g
			}
		}
		fmt.Printf("  Average gap: %v\n", totalGap/time.Duration(len(gaps)))
		fmt.Printf("  Max gap: %v\n", maxGap)
	}
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

func sanitize(data []byte) string {
	var result []byte
	for _, b := range data {
		if b >= 32 && b < 127 {
			result = append(result, b)
		} else if b == '\n' {
			result = append(result, '\\', 'n')
		} else if b == '\r' {
			result = append(result, '\\', 'r')
		} else {
			result = append(result, '.')
		}
	}
	return string(result)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
