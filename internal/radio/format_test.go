package radio

import (
	"testing"
	"time"
)

func TestFrequency(t *testing.T) {
	tests := []struct {
		name  string
		index int
		want  string
	}{
		{"first station", 0, "088.1"},
		{"second station", 1, "089.7"},
		{"tenth station", 9, "102.5"},
		{"past the FM band", 13, "108.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frequency(tt.index); got != tt.want {
				t.Errorf("frequency(%d) = %q, want %q", tt.index, got, tt.want)
			}
		})
	}
}

func TestFormatClock(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "00:00"},
		{"negative clamps", -5 * time.Second, "00:00"},
		{"seconds only", 9 * time.Second, "00:09"},
		{"minutes and seconds", 3*time.Minute + 45*time.Second, "03:45"},
		{"truncates fractions", 61*time.Second + 900*time.Millisecond, "01:01"},
		{"over an hour keeps counting minutes", 75*time.Minute + 2*time.Second, "75:02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatClock(tt.d); got != tt.want {
				t.Errorf("formatClock(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestProgressBar(t *testing.T) {
	tests := []struct {
		name     string
		pos, dur time.Duration
		width    int
		want     string
	}{
		{"unknown duration is empty", 10 * time.Second, 0, 4, "▯▯▯▯"},
		{"start", 0, time.Minute, 5, "▯▯▯▯▯"},
		{"half", 30 * time.Second, time.Minute, 4, "▮▮▯▯"},
		{"end", time.Minute, time.Minute, 3, "▮▮▮"},
		{"past the end clamps", 2 * time.Minute, time.Minute, 3, "▮▮▮"},
		{"negative position clamps", -time.Second, time.Minute, 3, "▯▯▯"},
		{"zero width", 30 * time.Second, time.Minute, 0, ""},
		{"negative width", 30 * time.Second, time.Minute, -3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := progressBar(tt.pos, tt.dur, tt.width); got != tt.want {
				t.Errorf("progressBar = %q, want %q", got, tt.want)
			}
		})
	}
}
