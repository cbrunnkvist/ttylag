// verify_rate measures throughput of data piped through stdin.
// Usage: ./ttylag --down 10kbit -- head -c 5000 /dev/zero | go run cmd/verify_rate/main.go
package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	start := time.Now()
	// Read everything from Stdin until EOF
	n, err := io.Copy(io.Discard, os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	duration := time.Since(start)

	bits := int64(n) * 8
	bps := float64(bits) / duration.Seconds()

	fmt.Printf("Read %d bytes in %v\n", n, duration)
	fmt.Printf("Rate: %.2f bits/sec\n", bps)
}
