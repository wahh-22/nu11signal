package radio

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

func startupStep(t *testing.T, s Startup, msg tea.Msg) (Startup, tea.Cmd) {
	t.Helper()
	next, cmd := s.Update(msg)
	ns, ok := next.(Startup)
	if !ok {
		t.Fatalf("Update returned %T, want radio.Startup", next)
	}
	return ns, cmd
}

func startupScreen(s Startup) string {
	return plain(s.m)
}

// The startup splash is the boot splash, and it stays for the whole wait,
// however long past bootDur the player takes to open.
func TestStartupHoldsTheBootUntilReady(t *testing.T) {
	for _, effects := range []bool{false, true} {
		c := newClock()
		s := NewStartup(Options{Now: c.now, Seed: 2077, Effects: effects}, make(chan Model))
		s, cmd := startupStep(t, s, tea.WindowSizeMsg{Width: 80, Height: 24})
		if cmd == nil {
			t.Fatal("the startup splash started without a tick")
		}
		for elapsed := time.Duration(0); elapsed < 10*bootDur; {
			d := s.m.tickInterval()
			c.advance(d)
			elapsed += d
			s, cmd = startupStep(t, s, tickMsg{gen: s.m.tickGen})
			if cmd == nil {
				t.Fatal("the startup tick chain stopped")
			}
		}
		screen := startupScreen(s)
		if !strings.Contains(screen, bootLine) || strings.Contains(screen, "NOW PLAYING") {
			t.Fatalf("effects=%v: the splash ended before the player was ready:\n%s", effects, screen)
		}
		for _, want := range []string{strings.TrimRight(emblemLogo.rows[0], " "), "LINKING"} {
			if !strings.Contains(screen, want) {
				t.Fatalf("effects=%v: startup screen lacks %q:\n%s", effects, want, screen)
			}
		}
	}
}

// Keys and clicks during the wait do nothing: there is no player yet to
// quit or to act on.
func TestStartupIgnoresInput(t *testing.T) {
	c := newClock()
	s := NewStartup(Options{Now: c.now, Seed: 2077}, make(chan Model))
	s, _ = startupStep(t, s, tea.WindowSizeMsg{Width: 80, Height: 24})
	before := startupScreen(s)
	for _, k := range []string{"q", "ctrl+c", "enter", "down", " "} {
		var cmd tea.Cmd
		s, cmd = startupStep(t, s, key(k))
		if cmd != nil {
			t.Fatalf("key %q returned a command during startup", k)
		}
	}
	if after := startupScreen(s); after != before {
		t.Fatalf("input changed the startup splash:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// Once the frontend hands over the Model built on its player, the program
// runs that Model: at the size already known, started, its boot running
// on as the same splash.
func TestStartupHandsOverToTheModel(t *testing.T) {
	c := newClock()
	next := make(chan Model, 1)
	s := NewStartup(Options{Now: c.now, Seed: 2077}, next)
	s, _ = startupStep(t, s, tea.WindowSizeMsg{Width: 100, Height: 30})
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	next <- New(f, Options{Now: c.now, Seed: 2077})
	msg := run(t, s.Init())
	model, cmd := s.Update(msg)
	m, ok := model.(Model)
	if !ok {
		t.Fatalf("handover returned %T, want radio.Model", model)
	}
	if cmd == nil {
		t.Fatal("handover did not start the Model")
	}
	if m.width != 100 || m.height != 30 {
		t.Fatalf("handed-over size %dx%d; want 100x30", m.width, m.height)
	}
	if !m.boot || m.bootEnd.IsZero() || !strings.Contains(plain(m), bootLine) {
		t.Fatalf("the Model did not continue the boot splash:\n%s", plain(m))
	}
	c.advance(bootDur)
	m, _ = step(t, m, tickMsg{gen: m.tickGen})
	if strings.Contains(plain(m), bootLine) || !strings.Contains(plain(m), "NOW PLAYING") {
		t.Fatalf("the normal UI did not follow the boot:\n%s", plain(m))
	}
}

// A frontend that gives up (the player failed to open) closes the channel:
// the startup program quits rather than waiting forever.
func TestStartupQuitsWhenAbandoned(t *testing.T) {
	next := make(chan Model)
	s := NewStartup(Options{Now: newClock().now, Seed: 2077}, next)
	close(next)
	_, cmd := s.Update(run(t, s.Init()))
	if cmd == nil {
		t.Fatal("abandoned startup returned no command")
	}
	if _, ok := run(t, cmd).(tea.QuitMsg); !ok {
		t.Fatal("abandoned startup did not quit")
	}
}
