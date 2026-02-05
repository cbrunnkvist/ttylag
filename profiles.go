//go:build !windows
// +build !windows

package main

import "time"

// profiles defines preset configurations for common connection types.
var profiles = map[string]Config{
	// Serial connections
	"9600": {
		UpRate:   960, // 9600 baud / 10 bits per byte
		DownRate: 960,
	},
	"2400": {
		UpRate:   240, // 2400 baud / 10 bits per byte
		DownRate: 240,
	},

	// Dial-up modems
	"dialup": {
		RTT:      150 * time.Millisecond,
		Jitter:   30 * time.Millisecond,
		DownRate: 56000 / 8, // 56kbit -> bytes
		UpRate:   33600 / 8, // 33.6kbit -> bytes
	},

	// Mobile networks
	"edge": {
		RTT:      500 * time.Millisecond,
		Jitter:   100 * time.Millisecond,
		DownRate: 200000 / 8, // 200kbit
		UpRate:   100000 / 8, // 100kbit
	},
	"3g": {
		RTT:      200 * time.Millisecond,
		Jitter:   50 * time.Millisecond,
		DownRate: 1000000 / 8, // 1mbit
		UpRate:   384000 / 8,  // 384kbit
	},
	"lte": {
		RTT:      50 * time.Millisecond,
		Jitter:   15 * time.Millisecond,
		DownRate: 20000000 / 8, // 20mbit
		UpRate:   5000000 / 8,  // 5mbit
	},
	"lte-poor": {
		RTT:      150 * time.Millisecond,
		Jitter:   50 * time.Millisecond,
		DownRate: 2000000 / 8, // 2mbit
		UpRate:   500000 / 8,  // 500kbit
	},

	// Wired connections
	"dsl": {
		RTT:      50 * time.Millisecond,
		Jitter:   10 * time.Millisecond,
		DownRate: 8000000 / 8, // 8mbit
		UpRate:   1000000 / 8, // 1mbit
	},
	"cable": {
		RTT:      30 * time.Millisecond,
		Jitter:   5 * time.Millisecond,
		DownRate: 50000000 / 8, // 50mbit
		UpRate:   5000000 / 8,  // 5mbit
	},

	// Satellite
	"satellite": {
		RTT:      600 * time.Millisecond, // Geostationary orbit
		Jitter:   50 * time.Millisecond,
		DownRate: 25000000 / 8, // 25mbit (Starlink-ish)
		UpRate:   5000000 / 8,  // 5mbit
	},
	"satellite-geo": {
		RTT:      700 * time.Millisecond, // High geostationary latency
		Jitter:   100 * time.Millisecond,
		DownRate: 10000000 / 8, // 10mbit (traditional VSAT)
		UpRate:   2000000 / 8,  // 2mbit
	},

	// WiFi scenarios
	"wifi-poor": {
		RTT:      80 * time.Millisecond,
		Jitter:   40 * time.Millisecond,
		DownRate: 2000000 / 8, // 2mbit
		UpRate:   1000000 / 8, // 1mbit
	},
	"wifi-bad": {
		RTT:      200 * time.Millisecond,
		Jitter:   100 * time.Millisecond,
		DownRate: 500000 / 8, // 500kbit
		UpRate:   250000 / 8, // 250kbit
	},

	// International/long-distance
	"intercontinental": {
		RTT:      250 * time.Millisecond, // e.g., US to Asia
		Jitter:   30 * time.Millisecond,
		DownRate: 10000000 / 8, // 10mbit (typical VPN)
		UpRate:   5000000 / 8,  // 5mbit
	},
}
