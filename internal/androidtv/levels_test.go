package androidtv

import (
	"context"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/composite"
	"github.com/wahh-22/nu11signal/internal/playback/demo"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

func TestPlayerLevelSource(t *testing.T) {
	if _, ok := any(&Player{}).(playback.LevelSource); !ok {
		t.Fatal("Android player does not expose real levels")
	}
	select {
	case _, ok := <-(&Player{}).Levels():
		if ok {
			t.Fatal("wrapper without capture reported a level")
		}
	default:
		t.Fatal("wrapper without capture must expose a closed channel")
	}
}

func spectrumPlayer(t *testing.T) (*Player, chan playback.State, chan webplayer.BridgeMessage, chan bool) {
	t.Helper()
	states := make(chan playback.State)
	readings := make(chan webplayer.BridgeMessage)
	controls := make(chan bool, 16)
	p := &Player{Player: webplayer.New(nil)}
	p.startSpectrum(states, readings, func(_ context.Context, m webplayer.BridgeMessage) error {
		if m.Type == "levels" {
			controls <- m.On
		}
		return nil
	})
	t.Cleanup(func() { p.Close() })
	return p, states, readings, controls
}

func stateBarrier(t *testing.T, p *Player, states chan playback.State, status playback.Status) {
	t.Helper()
	select {
	case states <- playback.State{Status: status}:
	case <-time.After(time.Second):
		t.Fatal("state worker blocked")
	}
	select {
	case s := <-p.States():
		if s.Status != status {
			t.Fatalf("state = %s", s.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("no forwarded state")
	}
}

func TestPlayerLevelsTransitionsAndSlowConsumer(t *testing.T) {
	p, states, readings, controls := spectrumPlayer(t)
	for _, status := range []playback.Status{playback.StatusPlaying, playback.StatusPlaying, playback.StatusPaused, playback.StatusPaused, playback.StatusStopped, playback.StatusPlaying, playback.StatusStopped, playback.StatusPlaying} {
		stateBarrier(t, p, states, status)
	}
	for _, want := range []bool{true, false, true, false, true} {
		if got := <-controls; got != want {
			t.Fatalf("control = %t; want %t", got, want)
		}
	}
	select {
	case v := <-controls:
		t.Fatalf("duplicate control: %t", v)
	default:
	}
	// No level consumer during the burst: the worker must not block and must
	// replace the unread reading. The next state is a processing barrier.
	for i := 0; i < 100; i++ {
		select {
		case readings <- webplayer.BridgeMessage{Bands: []int{i}, Wave: []int{-100, 100}}:
		case <-time.After(time.Second):
			t.Fatal("slow consumer blocked producer")
		}
	}
	stateBarrier(t, p, states, playback.StatusPlaying)
	s := <-p.Levels()
	if len(s.Bands) != 1 || s.Bands[0] != .99 || s.Wave[0] != -1 || s.Wave[1] != 1 {
		t.Fatalf("spectrum = %+v", s)
	}
	stateBarrier(t, p, states, playback.StatusPaused)
	<-controls
	readings <- webplayer.BridgeMessage{Bands: []int{100}}
	stateBarrier(t, p, states, playback.StatusPaused)
	select {
	case s := <-p.Levels():
		t.Fatalf("paused levels: %+v", s)
	default:
	}
	stateBarrier(t, p, states, playback.StatusPlaying)
	<-controls
	p.Close()
	if v := <-controls; v {
		t.Fatal("exit did not disable capture")
	}
	if _, ok := <-p.Levels(); ok {
		t.Fatal("levels not closed")
	}
}

func TestCompositeAndroidPrimaryLevels(t *testing.T) {
	p, states, readings, controls := spectrumPlayer(t)
	c := composite.New(p, demo.New(demo.Options{}))
	t.Cleanup(func() { c.Close() })
	states <- playback.State{Status: playback.StatusPlaying}
	<-controls
	readings <- webplayer.BridgeMessage{Bands: []int{75}, Wave: []int{-50}}
	select {
	case s := <-c.Levels():
		if s.Bands[0] != .75 || s.Wave[0] != -.5 {
			t.Fatalf("composite spectrum = %+v", s)
		}
	case <-time.After(time.Second):
		t.Fatal("composite did not expose Android levels")
	}
}

func TestRuntimeLevelsReaderDropsStale(t *testing.T) {
	r := &Runtime{readings: make(chan webplayer.BridgeMessage, 1)}
	for i := 0; i < 100; i++ {
		r.onMessage(webplayer.BridgeMessage{Type: "levels", Bands: []int{i}})
	}
	if got := <-r.readings; got.Bands[0] != 99 {
		t.Fatalf("latest = %+v", got)
	}
}
