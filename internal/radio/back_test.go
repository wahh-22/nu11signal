package radio

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// loadedWithRootBack is loaded at a phone's portrait size, counting the
// esc presses that found nothing left to go back to.
func loadedWithRootBack(t *testing.T, count *int) Model {
	t.Helper()
	c := newClock()
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	m := New(f, Options{SkipBoot: true, Now: c.now, Seed: 2077, OnRootBack: func() { *count++ }})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 56, Height: 60})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	return m
}

// Android's Back is esc. On a phone, a Back with nothing left to close
// leaves the app the way other apps do; until then it keeps navigating.
func TestEscOnTheRootReportsNothingLeftToGoBackTo(t *testing.T) {
	var n int
	m := loadedWithRootBack(t, &n)
	m, _ = press(t, m, "esc")
	if n != 1 {
		t.Fatalf("root esc reported %d times, want 1", n)
	}
	if len(m.stack) != 1 || m.focus != areaList {
		t.Fatalf("root esc changed the view: stack %d, focus %v", len(m.stack), m.focus)
	}
}

func TestEscOnAnOpenPlaylistGoesBackFirst(t *testing.T) {
	var n int
	m := loadedWithRootBack(t, &n)
	m, _ = press(t, m, "enter")
	if len(m.stack) != 2 {
		t.Fatalf("enter left %d views, want the playlist over the root", len(m.stack))
	}
	m, _ = press(t, m, "esc")
	if n != 0 || len(m.stack) != 1 {
		t.Fatalf("first esc: reported %d, stack %d; want 0 and back on the root", n, len(m.stack))
	}
	m, _ = press(t, m, "esc")
	if n != 1 {
		t.Fatalf("second esc reported %d times, want 1", n)
	}
}

func TestEscOnThePlayerReturnsToTheListFirst(t *testing.T) {
	var n int
	m := loadedWithRootBack(t, &n)
	m, _ = press(t, m, "right")
	if m.focus != areaPlayer {
		t.Fatalf("right left the focus on %v, want the player", m.focus)
	}
	m, _ = press(t, m, "esc")
	if n != 0 || m.focus != areaList {
		t.Fatalf("player esc: reported %d, focus %v; want 0 and the list", n, m.focus)
	}
}

// Desktop and TV pass no OnRootBack: esc on the root still does nothing.
func TestEscOnTheRootWithoutOnRootBackStaysPut(t *testing.T) {
	m := loadedAt(t, 56, 60)
	before := plain(m)
	m, cmd := press(t, m, "esc")
	if cmd != nil {
		t.Fatalf("root esc returned a command: %v", run(t, cmd))
	}
	if len(m.stack) != 1 || m.focus != areaList || plain(m) != before {
		t.Fatal("root esc without OnRootBack changed the screen")
	}
}
