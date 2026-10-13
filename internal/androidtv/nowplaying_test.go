package androidtv

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

func TestNowPlayingDedupAndSeek(t *testing.T) {
	var n nowPlayingTracker
	at := time.Unix(100, 0)
	long := strings.Repeat("é", 600)
	s := playback.State{Status: playback.StatusPlaying, SongID: "1", Title: long, Artist: long, Album: long, Duration: time.Minute}
	if !n.update(s, at) {
		t.Fatal("initial state omitted")
	}
	for _, text := range []string{n.message.Title, n.message.Artist, n.message.Album} {
		if len([]rune(text)) != 512 {
			t.Fatal("metadata not clamped")
		}
	}
	if n.update(s, at) {
		t.Fatal("duplicate published")
	}
	s.Position = time.Second
	if n.update(s, at.Add(time.Second)) {
		t.Fatal("ordinary position tick published")
	}
	s.Position = 20 * time.Second
	if !n.update(s, at.Add(2*time.Second)) {
		t.Fatal("seek omitted")
	}
	s.Status = playback.StatusPaused
	if !n.update(s, at.Add(3*time.Second)) || n.message.Playing {
		t.Fatal("pause omitted")
	}
	s.Position = -time.Second
	s.Duration = -time.Second
	n.update(s, at.Add(4*time.Second))
	if n.message.PositionMS != 0 || n.message.DurationMS != 0 {
		t.Fatal("negative times")
	}
}

func TestNowPlayingFollowsFakePlayer(t *testing.T) {
	fake := playbacktest.New()
	defer fake.Close()
	messages := make(chan webplayer.BridgeMessage, 32)
	p := &Player{}
	p.startSpectrum(fake.States(), nil, func(_ context.Context, m webplayer.BridgeMessage) error {
		messages <- m
		return nil
	})
	defer p.Close()
	var received time.Time
	next := func() webplayer.BridgeMessage {
		t.Helper()
		timeout := time.After(3 * time.Second)
		for {
			select {
			case m := <-messages:
				if m.Type == "nowplaying" {
					now := time.Now()
					if !received.IsZero() && now.Sub(received) < 900*time.Millisecond {
						t.Fatal("nowplaying exceeded one message per second")
					}
					received = now
					return m
				}
			case <-timeout:
				t.Fatal("no nowplaying")
				return webplayer.BridgeMessage{}
			}
		}
	}
	s := playback.State{Title: "First", Artist: "Artist", Album: "Album", SongID: "1", Status: playback.StatusPlaying, Duration: time.Minute}
	fake.PushState(s)
	if m := next(); m.Title != "First" || !m.Playing || m.DurationMS != 60000 {
		t.Fatalf("initial: %+v", m)
	}
	fake.PushState(s)
	select {
	case m := <-messages:
		if m.Type == "nowplaying" {
			t.Fatal("duplicate")
		}
	case <-time.After(100 * time.Millisecond):
	}
	s.Status = playback.StatusPaused
	fake.PushState(s)
	if m := next(); m.Playing {
		t.Fatal("pause not reported")
	}
	s.Position = 30 * time.Second
	fake.PushState(s)
	if m := next(); m.PositionMS != 30000 {
		t.Fatal("seek not reported")
	}
	s.Title = "Second"
	s.SongID = "2"
	s.Status = playback.StatusPlaying
	fake.PushState(s)
	if m := next(); m.Title != "Second" || !m.Playing {
		t.Fatal("song/resume not reported")
	}
	// Drain the earlier UI snapshots before using forwarding as a barrier.
	for len(p.spectrumStates) > 0 {
		<-p.States()
	}
	// Rapid song changes coalesce to the latest snapshot without blocking states.
	for _, title := range []string{"Third", "Fourth", "Latest"} {
		s.Title = title
		fake.PushState(s)
		select {
		case <-p.States():
		case <-time.After(time.Second):
			t.Fatal("state forwarding blocked")
		}
	}
	if m := next(); m.Title != "Latest" {
		t.Fatalf("did not coalesce: %+v", m)
	}
}
