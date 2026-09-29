package helper

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

func TestEncodeRequestRejectsReservedArgs(t *testing.T) {
	for _, key := range []string{"id", "cmd"} {
		if _, err := encodeRequest("1", "x", map[string]any{key: "v"}); err == nil {
			t.Errorf("encodeRequest accepted reserved argument %q", key)
		}
	}
	line, err := encodeRequest("7", "seek", map[string]any{"seconds": 1.5})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(line, &got); err != nil || got["id"] != "7" || got["cmd"] != "seek" || got["seconds"] != 1.5 {
		t.Fatalf("encodeRequest = %s, %v", line, err)
	}
}

func TestSecondsToDuration(t *testing.T) {
	tests := []struct {
		in   float64
		want time.Duration
	}{
		{0, 0},
		{12.25, 12*time.Second + 250*time.Millisecond},
		{0.1, 100 * time.Millisecond},
	}
	for _, tt := range tests {
		if got := seconds(tt.in); got != tt.want {
			t.Errorf("seconds(%v) = %v; want %v", tt.in, got, tt.want)
		}
	}
}

func TestStateRepeatModeDefaultsToOff(t *testing.T) {
	for _, tt := range []struct {
		wire string
		want playback.RepeatMode
	}{
		{"", playback.RepeatOff},
		{"off", playback.RepeatOff},
		{"all", playback.RepeatAll},
		{"one", playback.RepeatOne},
	} {
		if got := (wireState{Repeat: tt.wire}).toDomain().Repeat; got != tt.want {
			t.Errorf("repeat %q = %q; want %q", tt.wire, got, tt.want)
		}
	}
}
