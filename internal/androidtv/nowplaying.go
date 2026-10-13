package androidtv

import (
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

// nowPlayingTracker distinguishes clock progression from a seek. Position is
// an anchor for Android's extrapolating PlaybackState, not a ticking UI feed.
// MusicKit's polled position can jitter; discontinuities over 500 ms are seeks.
// The spectrum worker coalesces changed anchors to at most one per second.
type nowPlayingTracker struct {
	previous    playback.State
	at          time.Time
	initialized bool
	message     webplayer.BridgeMessage
}

func boundedText(s string) string {
	r := []rune(s)
	if len(r) > 512 {
		r = r[:512]
	}
	return string(r)
}

func (n *nowPlayingTracker) update(s playback.State, at time.Time) bool {
	previous := n.previous
	changed := !n.initialized || s.SongID != previous.SongID || s.Title != previous.Title || s.Artist != previous.Artist || s.Album != previous.Album || s.Status != previous.Status || s.Duration != previous.Duration
	expected := previous.Position
	if previous.Status == playback.StatusPlaying {
		expected += at.Sub(n.at)
	}
	delta := s.Position - expected
	if delta > 500*time.Millisecond || delta < -500*time.Millisecond {
		changed = true
	}
	n.previous, n.at, n.initialized = s, at, true
	if changed {
		message := webplayer.BridgeMessage{
			Type: "nowplaying", Title: boundedText(s.Title), Artist: boundedText(s.Artist), Album: boundedText(s.Album),
			Playing:    s.Status == playback.StatusPlaying,
			PositionMS: max(0, s.Position.Milliseconds()), DurationMS: max(0, s.Duration.Milliseconds()),
		}
		if sameNowPlaying(message, n.message) {
			return false
		}
		n.message = message
	}
	return changed
}

func sameNowPlaying(a, b webplayer.BridgeMessage) bool {
	return a.Type == b.Type && a.Title == b.Title && a.Artist == b.Artist && a.Album == b.Album && a.Playing == b.Playing && a.PositionMS == b.PositionMS && a.DurationMS == b.DurationMS
}
