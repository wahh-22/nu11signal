//go:build audiosmoke

package local

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// TestSmokeRealDevice plays a generated tone on the real default output
// device through New (the oto sink), so it only runs with
// `go test -tags audiosmoke -run Smoke ./internal/playback/local/...`. CI
// runs it on Linux against a PulseAudio null sink; on a machine with
// speakers it is audible.
//
// The device must consume audio in real time: ALSA's null device takes it
// as fast as it is rendered (the song ends at once), and an ALSA default
// device without a sound card blocks after the first buffers.
func TestSmokeRealDevice(t *testing.T) {
	const (
		rate = 48000
		hz   = 1000
	)
	root := t.TempDir()
	writeWAV(t, filepath.Join(root, "Smoke", "tone.wav"), rate, 2, 3*rate, wavPCM16, sine(hz, 0.5, rate))
	lib, err := Scan(ctx, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := lib.Playlist(lib.Playlists()[0].ID)
	if !ok || len(d.Tracks) != 1 {
		t.Fatalf("library = %+v", d)
	}

	p := New(lib, Options{SampleRate: rate})
	closed := false
	defer func() {
		if !closed {
			_ = p.Close()
		}
	}()
	if _, err := p.PlaySongs(ctx, []string{d.Tracks[0].ID}, 0); err != nil {
		t.Fatalf("PlaySongs on the real device: %v", err)
	}
	_ = p.SetVolume(ctx, 0.2) // levels are measured before the volume

	// Play for about a second, keeping the furthest position and the
	// latest levels reading.
	var (
		last     playback.State
		reading  playback.Spectrum
		readings int
	)
	deadline := time.After(1200 * time.Millisecond)
loop:
	for {
		select {
		case s, ok := <-p.States():
			if !ok {
				t.Fatal("States closed while playing")
			}
			if s.Position >= last.Position {
				last = s
			}
		case r, ok := <-p.Levels():
			if !ok {
				t.Fatal("Levels closed while playing")
			}
			reading, readings = r, readings+1
		case err := <-p.Errors():
			t.Fatalf("player error: %v", err)
		case <-deadline:
			break loop
		}
	}

	if last.Status != playback.StatusPlaying {
		t.Errorf("status = %v, want playing", last.Status)
	}
	// The position follows the frames the device pulled: a stalled device
	// leaves it at zero.
	if last.Position < 500*time.Millisecond || last.Position > 2*time.Second {
		t.Errorf("position after ~1.2 s = %v, want 0.5-2 s; a stalled position means the device stopped pulling "+
			"(oto finds PulseAudio at $XDG_RUNTIME_DIR/pulse/native or $PULSE_SERVER, else it falls back to ALSA; "+
			"XDG_RUNTIME_DIR=%q PULSE_SERVER=%q)", last.Position, os.Getenv("XDG_RUNTIME_DIR"), os.Getenv("PULSE_SERVER"))
	}
	if readings < 5 {
		t.Fatalf("%d level readings in ~1.2 s, want at least 5 (15 Hz)", readings)
	}
	if len(reading.Bands) != numBands {
		t.Fatalf("reading has %d bands, want %d", len(reading.Bands), numBands)
	}
	band := bandOf(hz, numBands, minHz, maxHz)
	for i, b := range reading.Bands {
		if b > reading.Bands[band] {
			t.Errorf("band %d (%.2f) is louder than the tone's band %d (%.2f)", i, b, band, reading.Bands[band])
		}
	}
	if reading.Bands[band] < 0.5 || peak32(reading.Wave) < 0.3 {
		t.Errorf("tone band %d = %.2f, wave peak %.2f; want >= 0.5 and >= 0.3", band, reading.Bands[band], peak32(reading.Wave))
	}
	t.Logf("position %v, %d readings, tone band %d = %.2f", last.Position, readings, band, reading.Bands[band])

	start := time.Now()
	closed = true
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Close took %v", took)
	}
	// Close closed the channels: draining what was buffered ends.
	for range p.States() {
	}
}
