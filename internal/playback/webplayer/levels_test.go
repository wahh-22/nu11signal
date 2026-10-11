package webplayer

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/spectrum"
)

func TestCaptureFollowsStateAndDeliversLevels(t *testing.T) {
	starts, stops := make(chan struct{}, 4), make(chan struct{}, 4)
	p := New(nil, func(p *Player) {
		p.capture = func(ctx context.Context, samples func([]float32, int)) (func(), error) {
			pcm := make([]float32, spectrum.FFTSize)
			for i := range pcm {
				pcm[i] = float32(0.5 * math.Sin(2*math.Pi*1000*float64(i)/48000))
			}
			samples(pcm, 48000)
			starts <- struct{}{}
			return func() { stops <- struct{}{} }, nil
		}
	})
	defer p.Close()
	state := func(s playback.Status) {
		p.mu.Lock()
		p.publish(playback.State{Status: s}, time.Now())
		p.mu.Unlock()
	}
	wait := func(c <-chan struct{}) {
		t.Helper()
		select {
		case <-c:
		case <-time.After(time.Second):
			t.Fatal("capture transition timed out")
		}
	}
	state(playback.StatusPlaying)
	wait(starts)
	select {
	case s := <-p.Levels():
		if len(s.Bands) != 24 || len(s.Wave) != 64 {
			t.Fatal("missing spectrum")
		}
		peak := 0.0
		for _, v := range s.Bands {
			peak = math.Max(peak, v)
		}
		if peak == 0 {
			t.Fatal("tone produced no levels")
		}
	case <-time.After(time.Second):
		t.Fatal("no levels")
	}
	select {
	case <-p.Levels():
		t.Fatal("stale PCM emitted another reading")
	case <-time.After(150 * time.Millisecond):
	}
	state(playback.StatusPaused)
	wait(stops)
	state(playback.StatusPlaying)
	wait(starts)
	state(playback.StatusStopped)
	wait(stops)
	state(playback.StatusPlaying)
	wait(starts)
	p.Close()
	wait(stops)
	for range p.Levels() {
	}
}

func TestCaptureFailureStaysDecorative(t *testing.T) {
	attempts := make(chan struct{}, 10)
	p := New(nil, func(p *Player) {
		p.capture = func(context.Context, func([]float32, int)) (func(), error) {
			attempts <- struct{}{}
			return nil, errors.New("no server")
		}
	})
	defer p.Close()
	p.mu.Lock()
	p.publish(playback.State{Status: playback.StatusPlaying}, time.Now())
	p.mu.Unlock()
	select {
	case <-attempts:
	case <-time.After(time.Second):
		t.Fatal("no attempt")
	}
	p.mu.Lock()
	p.publish(playback.State{Status: playback.StatusPlaying}, time.Now())
	p.mu.Unlock()
	select {
	case <-p.Levels():
		t.Fatal("failure emitted levels")
	case <-attempts:
		t.Fatal("retried during same play")
	case <-time.After(100 * time.Millisecond):
	}
}
