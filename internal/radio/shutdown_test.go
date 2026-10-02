package radio

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// shutdownLine is the line under the emblem while nu11signal shuts down,
// spaced out.
const shutdownLine = "S H U T T I N G   D O W N . . ."

// collect runs cmd and, for a batch, every command in it at once, and
// returns the messages that arrived until a closedMsg did (or 2 s
// passed), and a short grace after it. The tick chain's commands, which
// sleep, are left running: a tea.Quit among them would answer at once.
func collect(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	ch := make(chan tea.Msg, 16)
	var launch func(tea.Cmd)
	launch = func(c tea.Cmd) {
		go func() {
			msg := c()
			if b, ok := msg.(tea.BatchMsg); ok {
				for _, sub := range b {
					if sub != nil {
						launch(sub)
					}
				}
				return
			}
			ch <- msg
		}()
	}
	launch(cmd)
	var msgs []tea.Msg
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			msgs = append(msgs, msg)
			if _, ok := msg.(closedMsg); ok {
				grace := time.After(20 * time.Millisecond)
				for {
					select {
					case msg := <-ch:
						msgs = append(msgs, msg)
					case <-grace:
						return msgs
					}
				}
			}
		case <-deadline:
			t.Fatalf("no closedMsg within 2 s; got %v", msgs)
			return msgs
		}
	}
}

// quits reports whether cmd is tea.Quit: it answers at once, so a command
// still running after a short wait (a tick) is not.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		_, ok := msg.(tea.QuitMsg)
		return ok
	case <-time.After(200 * time.Millisecond):
		return false
	}
}

// confirmQuit opens the quit modal on m and confirms it with y: the
// shutdown splash shows, the modal closed, and cmd closes the player
// without quitting yet. It returns the model shutting down, its close
// already delivered when deliver is set.
func confirmQuit(t *testing.T, f *playbacktest.Fake, m Model, deliver bool) Model {
	t.Helper()
	m, _ = press(t, m, keyQuit)
	m, cmd := press(t, m, "y")
	return assertShutsDown(t, f, m, cmd, deliver)
}

// assertShutsDown fails unless m shows the shutdown splash and cmd closes
// the player without quitting; with deliver, the close's message is
// stepped into m, which must not quit yet either.
func assertShutsDown(t *testing.T, f *playbacktest.Fake, m Model, cmd tea.Cmd, deliver bool) Model {
	t.Helper()
	if !m.shutdown || m.quitAsk {
		t.Fatalf("shutdown %v quit modal %v; want shutting down, the modal closed", m.shutdown, m.quitAsk)
	}
	// The frame under any intro scrambling the splash in.
	base := ansi.Strip(strings.Join(first(m.baseLayout()), "\n"))
	if !strings.Contains(base, shutdownLine) && !strings.Contains(base, shutdownText) {
		t.Fatalf("the shutdown splash lacks its line:\n%s", base)
	}
	msgs := collect(t, cmd)
	for _, msg := range msgs {
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatal("confirming quit quit at once; want the shutdown splash first")
		}
	}
	if f != nil && !f.Closed() {
		t.Fatal("the player was not closed")
	}
	if deliver {
		var quit tea.Cmd
		m, quit = step(t, m, closedMsg{})
		if quits(quit) {
			t.Fatal("the close quit before the minimum display time")
		}
	}
	return m
}

// assertQuits fails unless confirming quit on m with cmd runs the
// shutdown: the splash, the player closed, then tea.Quit once the
// minimum display time has passed.
func assertQuits(t *testing.T, f *playbacktest.Fake, m Model, cmd tea.Cmd) {
	t.Helper()
	m = assertShutsDown(t, f, m, cmd, true)
	end := m.shutdownEnd
	m.now = func() time.Time { return end }
	if _, quit := step(t, m, tickMsg{gen: m.tickGen}); !quits(quit) {
		t.Fatal("the shutdown did not quit once closed and shown")
	}
}

func TestConfirmingQuitShowsTheShutdownSplash(t *testing.T) {
	f := playbacktest.New()
	m := confirmQuit(t, f, loaded(t, f, newClock()), false)
	screen := plain(m)
	for _, want := range append(slices.Clone(emblemLogo.rows), shutdownLine) {
		if !strings.Contains(screen, strings.TrimRight(want, " ")) {
			t.Errorf("shutdown splash lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, bootLine) || strings.Contains(screen, "NOW PLAYING") || strings.Contains(screen, quitQuestion) {
		t.Fatalf("shutdown splash draws the boot line, the panels or the modal:\n%s", screen)
	}
	if got := strings.TrimSpace(hintsOf(m)); got != "" {
		t.Fatalf("footer during shutdown reads %q; want it empty", got)
	}
	_, zs := m.baseLayout()
	for _, z := range zs {
		if z.y > 1 {
			t.Errorf("zone %q at row %d in the shutdown splash", z.id, z.y)
		}
	}
}

// tea.Quit waits for both the close (or its timeout) and the minimum
// display time, whichever comes last.
func TestShutdownQuitsWhenClosedAndShown(t *testing.T) {
	t.Run("close first", func(t *testing.T) {
		c := newClock()
		f := playbacktest.New()
		m := confirmQuit(t, f, loaded(t, f, c), true)
		c.advance(shutdownMin - time.Millisecond)
		m, cmd := step(t, m, tickMsg{gen: m.tickGen})
		if quits(cmd) {
			t.Fatal("quit before the minimum display time")
		}
		c.advance(time.Millisecond)
		if _, cmd = step(t, m, tickMsg{gen: m.tickGen}); !quits(cmd) {
			t.Fatal("did not quit once closed and shown")
		}
	})
	t.Run("time first", func(t *testing.T) {
		c := newClock()
		f := playbacktest.New()
		m := confirmQuit(t, f, loaded(t, f, c), false)
		c.advance(shutdownMin)
		m, cmd := step(t, m, tickMsg{gen: m.tickGen})
		if quits(cmd) {
			t.Fatal("quit with the close pending")
		}
		if d := m.tickInterval(); d <= minWake {
			t.Fatalf("waiting for the close ticks every %v; want no busy loop", d)
		}
		if _, cmd = step(t, m, closedMsg{}); !quits(cmd) {
			t.Fatal("did not quit once the close finished after the minimum time")
		}
	})
	t.Run("stale tick", func(t *testing.T) {
		c := newClock()
		f := playbacktest.New()
		m := confirmQuit(t, f, loaded(t, f, c), true)
		c.advance(shutdownMin)
		if _, cmd := step(t, m, tickMsg{gen: m.tickGen - 1}); quits(cmd) {
			t.Fatal("a superseded tick quit")
		}
	})
}

// The calm shutdown ticks once, at the minimum display time; with the
// effects on it ticks at burstTick for the glitch.
func TestShutdownTicksToItsEnd(t *testing.T) {
	c := newClock()
	f := playbacktest.New()
	m := confirmQuit(t, f, loaded(t, f, c), false)
	if d := m.tickInterval(); d != shutdownMin {
		t.Fatalf("calm shutdown ticks after %v; want %v", d, shutdownMin)
	}
	c.advance(400 * time.Millisecond)
	if d := m.tickInterval(); d != shutdownMin-400*time.Millisecond {
		t.Fatalf("calm shutdown ticks after %v; want the rest of %v", d, shutdownMin)
	}
}

// A player whose Close never returns still lets nu11signal quit: the
// close gives up after closeTimeout and the shutdown ends.
func TestShutdownQuitsOnACloseTimeout(t *testing.T) {
	c := newClock()
	p := blockingCloser{Fake: playbacktest.New(), release: make(chan struct{})}
	defer close(p.release)
	m := New(p, Options{SkipBoot: true, Now: c.now, CloseTimeout: 50 * time.Millisecond})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := press(t, m, keyQuit, "y")
	m = assertShutsDown(t, nil, m, cmd, true)
	c.advance(shutdownMin)
	if _, cmd = step(t, m, tickMsg{gen: m.tickGen}); !quits(cmd) {
		t.Fatal("a stuck player kept the shutdown from quitting")
	}
}

// ctrl+c during the shutdown quits at once, the escape hatch.
func TestCtrlCDuringShutdownQuitsAtOnce(t *testing.T) {
	f := playbacktest.New()
	m := confirmQuit(t, f, loaded(t, f, newClock()), false)
	if _, cmd := press(t, m, keyCtrlC); !quits(cmd) {
		t.Fatal("ctrl+c during the shutdown did not quit at once")
	}
}

// Every other key and the mouse do nothing during the shutdown.
func TestShutdownIgnoresKeysAndTheMouse(t *testing.T) {
	f := playbacktest.New()
	m := confirmQuit(t, f, loaded(t, f, newClock()), false)
	view := plain(m)
	before := len(f.Calls())
	for _, k := range []string{keyQuit, "y", "n", keyEsc, keyEnter, "space", "x", "?", "s", "down", "tab"} {
		next, cmd := press(t, m, k)
		if cmd != nil || !next.shutdown || next.quitAsk || next.help || next.settings || next.fx.on != m.fx.on || plain(next) != view {
			t.Fatalf("%q acted during the shutdown", k)
		}
	}
	for _, msg := range []tea.Msg{
		tea.MouseClickMsg{X: 4, Y: 1, Button: tea.MouseLeft},
		tea.MouseClickMsg{X: 40, Y: 10, Button: tea.MouseLeft},
		tea.MouseWheelMsg{X: 1, Y: 5, Button: tea.MouseWheelDown},
	} {
		next, cmd := step(t, m, msg)
		if cmd != nil || !next.shutdown || plain(next) != view {
			t.Fatalf("%T acted during the shutdown", msg)
		}
	}
	if got := f.Calls()[before:]; len(got) != 0 {
		t.Fatalf("input reached the player during the shutdown: %v", got)
	}
}

// calmlessModel is an authorized model at 80x24 past the boot with the effects
// on, on the clock c.
func calmlessModel(t *testing.T, f *playbacktest.Fake, c *clock) Model {
	t.Helper()
	f.PlaylistsResult = stations()
	m := New(f, Options{SkipBoot: true, Now: c.now, Seed: 2077, Effects: true})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	c.advance(introDur)
	m, _ = step(t, m, tickMsg{gen: m.tickGen})
	return m
}

// With the effects on the shutdown splash glitches like the boot's, a
// burst of tears, Braille noise and static over the body only, at
// burstTick, drawing no letter or digit of its own; calm, it shows still.
func TestShutdownGlitches(t *testing.T) {
	c := newClock()
	f := playbacktest.New()
	m := confirmQuit(t, f, calmlessModel(t, f, c), false)
	c.advance(introDur)
	m, _ = step(t, m, tickMsg{gen: m.tickGen})
	if d := m.tickInterval(); d != burstTick {
		t.Fatalf("shutdown ticks every %v; want %v", d, burstTick)
	}
	frames := map[string]bool{}
	for range 4 {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
		base, _ := m.baseLayout()
		lines, _ := m.layout()
		if reflect.DeepEqual(lines, base) {
			t.Fatalf("frame %d of the shutdown drew no glitch", m.frame)
		}
		for _, y := range []int{0, 1, 22, 23} {
			if lines[y] != base[y] {
				t.Fatalf("frame %d glitched row %d outside the body", m.frame, y)
			}
		}
		if emblemNoiseOver(lines, base) == 0 {
			t.Fatalf("frame %d of the shutdown has no Braille noise cells", m.frame)
		}
		for y := range lines {
			for _, r := range ansi.Strip(lines[y]) {
				if (unicode.IsLetter(r) || unicode.IsDigit(r)) && !strings.ContainsRune(ansi.Strip(base[y]), r) {
					t.Fatalf("frame %d drew %q on row %d, not in the splash", m.frame, r, y)
				}
			}
		}
		frames[strings.Join(lines, "\n")] = true
	}
	if len(frames) < 2 {
		t.Fatal("the shutdown glitch does not move")
	}

	cf := playbacktest.New()
	calm := confirmQuit(t, cf, loaded(t, cf, newClock()), false)
	if lines, _ := calm.layout(); !reflect.DeepEqual(lines, first(calm.baseLayout())) {
		t.Fatal("the calm shutdown drew effects")
	}
}

// Every confirm path of the quit modal shuts down: y, enter, q or
// ctrl+c again, and the QUIT button.
func TestEveryQuitConfirmShutsDown(t *testing.T) {
	for _, k := range []string{"y", keyEnter, keyQuit, keyCtrlC} {
		t.Run(k, func(t *testing.T) {
			f := playbacktest.New()
			m, _ := press(t, loaded(t, f, newClock()), keyQuit)
			m, cmd := press(t, m, k)
			assertShutsDown(t, f, m, cmd, false)
		})
	}
	t.Run("click", func(t *testing.T) {
		f := playbacktest.New()
		m, _ := press(t, loaded(t, f, newClock()), keyQuit)
		m, cmd := click(t, m, zoneQuitYes)
		assertShutsDown(t, f, m, cmd, false)
	})
}

// Quitting from the auth error screen shows the shutdown splash too.
func TestShutdownFromTheAuthErrorScreen(t *testing.T) {
	f := playbacktest.New()
	f.AuthStatus = playback.AuthDenied
	m := newModel(t, f, newClock())
	m, _ = step(t, m, run(t, m.authorizeCmd()))
	m = confirmQuit(t, f, m, false)
	if screen := plain(m); strings.Contains(screen, "AUTH // ERROR") || !strings.Contains(screen, shutdownLine) {
		t.Fatalf("shutdown from the auth error screen:\n%s", screen)
	}
}

// The BOOTING and SHUTTING DOWN lines are drawn bright (stHiBold), not
// muted, in every theme.
func TestSplashLinesAreBright(t *testing.T) {
	for _, theme := range []string{"REDSHIFT", "BLUESHIFT"} {
		t.Run(theme, func(t *testing.T) {
			boot := bootModel(t, newClock(), 80, 24, false)
			f := playbacktest.New()
			shut := confirmQuit(t, f, loaded(t, f, newClock()), false)
			useTheme(t, theme)
			for _, tt := range []struct {
				m    Model
				line string
			}{{boot, bootLine}, {shut, shutdownLine}} {
				screen := tt.m.render()
				if !strings.Contains(screen, stHiBold.Render(tt.line)) {
					t.Errorf("%q is not drawn in stHiBold:\n%q", tt.line, screen)
				}
				if strings.Contains(screen, stMuted.Render(tt.line)) {
					t.Errorf("%q is still muted", tt.line)
				}
			}
		})
	}
}

func TestShutdownGolden80x24(t *testing.T) {
	f := playbacktest.New()
	m := confirmQuit(t, f, loaded(t, f, newClock()), false)
	assertGolden(t, "shutdown_80x24.golden", plain(m))
}
