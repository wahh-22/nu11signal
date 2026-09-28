package radio

import (
	"fmt"
	"strings"
	"time"
)

// Stations are laid out on a pseudo FM dial: 88.1 MHz, then every 1.6 MHz.
// Tenths of a MHz keep the arithmetic exact.
const (
	dialStartTenths = 881
	dialStepTenths  = 16
)

// frequency returns the dial label of the station at index, e.g. "089.7".
func frequency(index int) string {
	t := dialStartTenths + dialStepTenths*index
	return fmt.Sprintf("%03d.%d", t/10, t%10)
}

// formatClock renders d as mm:ss; minutes keep counting past an hour.
func formatClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d / time.Second)
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

// progressBar renders pos/dur as width segments of ▮ (elapsed) and ▯.
func progressBar(pos, dur time.Duration, width int) string {
	if width <= 0 {
		return ""
	}
	filled := 0
	if dur > 0 {
		pos = clampDuration(pos, 0, dur)
		filled = int((int64(pos)*int64(width) + int64(dur)/2) / int64(dur))
	}
	return strings.Repeat("▮", filled) + strings.Repeat("▯", width-filled)
}

func clampDuration(d, lo, hi time.Duration) time.Duration {
	return max(lo, min(d, hi))
}
