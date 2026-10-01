package helper

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// shutdownSwift is the helper source holding its shutdown timings.
var shutdownSwift = filepath.Join("..", "..", "helper", "Sources", "Nu11SignalProtocol", "Shutdown.swift")

// swiftSeconds reads `static let <name>... = <number>` from the helper's
// shutdown source, so the Go side cannot drift from the Swift one.
func swiftSeconds(t *testing.T, source, name string) time.Duration {
	t.Helper()
	re := regexp.MustCompile(`static let ` + name + `(?:: \w+)? = ([0-9.]+)`)
	m := re.FindStringSubmatch(source)
	if m == nil {
		t.Fatalf("%s not found in %s", name, shutdownSwift)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return time.Duration(v * float64(time.Second))
}

// TestCloseTimeoutCoversTheHelpersQuietExit pins the timeout chain
// documented on DefaultCloseTimeout: Close never kills a helper that is
// still waiting for requests, fading out or within its own backstop.
func TestCloseTimeoutCoversTheHelpersQuietExit(t *testing.T) {
	raw, err := os.ReadFile(shutdownSwift)
	if err != nil {
		t.Fatalf("read helper shutdown timings: %v", err)
	}
	src := string(raw)
	grace := swiftSeconds(t, src, "requestGraceSeconds")
	backstop := swiftSeconds(t, src, "backstopSeconds")
	fade := swiftSeconds(t, src, "fadeSeconds") + swiftSeconds(t, src, "fadeMarginSeconds")
	pause := swiftSeconds(t, src, "pauseWaitSeconds")

	if grace != helperRequestGrace || backstop != helperBackstop {
		t.Fatalf("Go mirrors grace %s, backstop %s; the helper has %s, %s", helperRequestGrace, helperBackstop, grace, backstop)
	}
	if quiet := grace + fade + pause + closeMargin; DefaultCloseTimeout < quiet {
		t.Errorf("DefaultCloseTimeout %s < grace + fade + pause wait + margin %s", DefaultCloseTimeout, quiet)
	}
	if DefaultCloseTimeout <= grace+backstop {
		t.Errorf("DefaultCloseTimeout %s does not outlast the helper's own backstop (%s)", DefaultCloseTimeout, grace+backstop)
	}
	if fade+pause >= backstop {
		t.Errorf("the quiet exit (%s) does not fit the helper's backstop (%s)", fade+pause, backstop)
	}
}

func TestStartDefaultsToTheDocumentedCloseTimeout(t *testing.T) {
	c := startFake(t, "standard", Options{})
	if c.closeTimeout != DefaultCloseTimeout {
		t.Fatalf("closeTimeout = %s, want %s", c.closeTimeout, DefaultCloseTimeout)
	}
}
