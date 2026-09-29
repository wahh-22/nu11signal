package radio

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// setRepeatCalls are the modes SetRepeat was called with, in order.
func setRepeatCalls(f *playbacktest.Fake) []any {
	var out []any
	for _, c := range callsOf(f, "SetRepeat") {
		out = append(out, c.Args[0])
	}
	return out
}

// loopLabel is the text of the LOOP button as drawn.
func loopLabel(t *testing.T, m Model) string {
	t.Helper()
	return strings.TrimSpace(strings.Trim(textAt(m, zoneOf(t, m, zoneLoop)), "╱ ▸"))
}

func TestLoopKeyCyclesOffAllOne(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	if got := loopLabel(t, m); got != "↻ LOOP OFF" {
		t.Fatalf("LOOP reads %q; want ↻ LOOP OFF", got)
	}
	for _, want := range []struct {
		mode  playback.RepeatMode
		label string
	}{
		{playback.RepeatAll, "↻ LOOP ALL"},
		{playback.RepeatOne, "↻ LOOP ONE"},
		{playback.RepeatOff, "↻ LOOP OFF"},
	} {
		next, c := press(t, m, keyLoop)
		// Optimistic: the button shows the mode before the player answers.
		if got := loopLabel(t, next); got != want.label {
			t.Fatalf("after %s LOOP reads %q; want %q", keyLoop, got, want.label)
		}
		m = settle(t, next, c)
		if got := f.RepeatMode(); got != want.mode {
			t.Fatalf("player repeat %q; want %q", got, want.mode)
		}
	}
	if got, want := setRepeatCalls(f), []any{playback.RepeatAll, playback.RepeatOne, playback.RepeatOff}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SetRepeat calls %v; want %v", got, want)
	}
}

func TestLoopFollowsThePlayerOnceItAnswers(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	// Set elsewhere (the Music app): the state decides.
	s := playing(time.Minute, 3*time.Minute)
	s.Repeat = playback.RepeatOne
	m, _ = step(t, m, stateMsg{state: s})
	if got := loopLabel(t, m); got != "↻ LOOP ONE" {
		t.Fatalf("LOOP reads %q; want the player's ONE", got)
	}
	// ONE cycles to OFF; a state still reporting ONE (sent before the
	// change) does not undo what was asked.
	m, cmd := press(t, m, keyLoop)
	m, _ = step(t, m, stateMsg{state: s})
	if got := loopLabel(t, m); got != "↻ LOOP OFF" {
		t.Fatalf("a stale state reverted LOOP to %q", got)
	}
	m = settle(t, m, cmd)
	s.Repeat = playback.RepeatOff
	m, _ = step(t, m, stateMsg{state: s})
	if got := loopLabel(t, m); got != "↻ LOOP OFF" {
		t.Fatalf("LOOP reads %q; want OFF", got)
	}
	// Confirmed: the player's reports rule again.
	s.Repeat = playback.RepeatAll
	m, _ = step(t, m, stateMsg{state: s})
	if got := loopLabel(t, m); got != "↻ LOOP ALL" {
		t.Fatalf("LOOP reads %q; want the player's ALL", got)
	}
}

func TestARefusedLoopChangeRevertsToThePlayersMode(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"SetRepeat": errors.New("no queue")}
	m := playingModel(t, f)
	m, cmd := press(t, m, keyLoop)
	m = settle(t, m, cmd)
	if got := loopLabel(t, m); got != "↻ LOOP OFF" {
		t.Fatalf("LOOP reads %q after a refusal; want the player's OFF", got)
	}
	if status := strings.ToUpper(m.status); !strings.Contains(status, "LOOP FAILED") || !strings.Contains(status, "NO QUEUE") {
		t.Fatalf("status %q; want the refusal", m.status)
	}
}

func TestLoopButtonIsClickableAndInTheFocusOrder(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f)
	m, cmd := click(t, m, zoneLoop)
	m = settle(t, m, cmd)
	if got := setRepeatCalls(f); !reflect.DeepEqual(got, []any{playback.RepeatAll}) {
		t.Fatalf("click: SetRepeat calls %v; want [all]", got)
	}
	if !m.focused(ctlLoop) {
		t.Fatal("the click did not leave the focus on LOOP")
	}

	// Keyboard: from the list to PLAY, down to the volume row, down to LOOP.
	m = playingModel(t, f)
	m, _ = press(t, m, "right", "down", "down")
	if !m.focused(ctlLoop) {
		t.Fatalf("focus %v control %v; want LOOP under the volume row", m.focus, m.control)
	}
	if !strings.Contains(textAt(m, zoneOf(t, m, zoneLoop)), "▸") {
		t.Fatal("the focused LOOP button is not marked")
	}
	m, cmd = press(t, m, "enter")
	settle(t, m, cmd)
	if got := setRepeatCalls(f); len(got) != 2 {
		t.Fatalf("enter on LOOP: SetRepeat calls %v; want a second one", got)
	}
	// Alone on its row: → stays, ← leaves for the list as from the first
	// button of any row; ↑ goes back to the volume row.
	m, _ = press(t, m, "right")
	if !m.focused(ctlLoop) {
		t.Fatal("→ left LOOP, alone on its row")
	}
	m, _ = press(t, m, "up")
	if !m.focused(ctlVolDown) {
		t.Fatalf("up from LOOP focused %v; want VOL-", m.control)
	}
	m, _ = press(t, m, "down", "left")
	if m.focus != areaList {
		t.Fatalf("← from LOOP: focus %v; want the list", m.focus)
	}
}

func TestLoopKeyWorksFromThePlayerAndPages(t *testing.T) {
	f := playbacktest.New()
	m := openSong(t, f, 1)
	m, cmd := press(t, m, keyLoop)
	settle(t, m, cmd)
	if got := setRepeatCalls(f); !reflect.DeepEqual(got, []any{playback.RepeatAll}) {
		t.Fatalf("on a page: SetRepeat calls %v; want [all]", got)
	}
}

func TestTheCompactLayoutEndsTheArtistLineWithLoop(t *testing.T) {
	for _, w := range []int{59, 40} {
		f := playbacktest.New()
		m := withVolume(t, playingModel(t, f), 0.5)
		m, _ = step(t, m, tea.WindowSizeMsg{Width: w, Height: 14})
		z := zoneOf(t, m, zoneLoop)
		if line := strings.Split(plain(m), "\n")[z.y]; !strings.HasPrefix(line, "SAMURAI") {
			t.Fatalf("width %d: LOOP drawn on %q; want the artist line", w, line)
		}
		// Reachable from the keyboard whether the volume row fits or not.
		m, _ = press(t, m, "right", "down", "down")
		if !m.focused(ctlLoop) {
			t.Fatalf("width %d: focus on %v; want LOOP", w, m.control)
		}
		m, cmd := click(t, m, zoneLoop)
		settle(t, m, cmd)
		if got := setRepeatCalls(f); !reflect.DeepEqual(got, []any{playback.RepeatAll}) {
			t.Fatalf("width %d: SetRepeat calls %v; want [all]", w, got)
		}
	}
}

func TestThePlayingHeartIsInTheFocusOrder(t *testing.T) {
	f := playbacktest.New()
	m := playingModel(t, f) // Chippin' In, c1, seekable
	// PLAY, up to the bar, up to the ♥ over it.
	m, _ = press(t, m, "right", "up", "up")
	if !m.focused(ctlFav) {
		t.Fatalf("focus %v control %v bar %v; want the ♥ above the bar", m.focus, m.control, m.onBar)
	}
	if got := textAt(m, zoneOf(t, m, zoneFavPlaying)); !strings.Contains(got, "▸") {
		t.Fatalf("focused ♥ shows %q; want the ▸ marker", got)
	}
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	assertCall(t, f, "SetFavorite", "c1", true)
	if !m.focused(ctlFav) {
		t.Fatal("enter moved the focus off the ♥")
	}
	// Up again reaches the tabs; down goes back to the bar.
	m, _ = press(t, m, "down")
	if !m.barFocused() {
		t.Fatal("down from the ♥ did not reach the bar")
	}
	m, _ = press(t, m, "up", "up")
	if m.focus != areaTabs {
		t.Fatalf("up from the ♥: focus %v; want the tabs", m.focus)
	}
	// A click focuses it too.
	m, _ = press(t, m, "down")
	m, cmd = click(t, m, zoneFavPlaying)
	m = settle(t, m, cmd)
	if !m.focused(ctlFav) {
		t.Fatal("a click on the ♥ did not focus it")
	}
	assertCall(t, f, "SetFavorite", "c1", false)
}

func TestWithNothingPlayingTheFocusSkipsTheHeart(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, "right", "up")
	if m.focus != areaTabs {
		t.Fatalf("up from PLAY with no song: focus %v control %v; want the tabs", m.focus, m.control)
	}
}

func TestAnUnconfirmedLoopChangeGivesWayToThePlayer(t *testing.T) {
	f := playbacktest.New()
	c := newClock()
	m := loaded(t, f, c)
	s := playing(time.Minute, 3*time.Minute)
	m, _ = step(t, m, stateMsg{state: s})
	// Accepted, but the player keeps reporting OFF (it did not take it).
	m, cmd := press(t, m, keyLoop)
	m = settle(t, m, cmd)
	m, _ = step(t, m, stateMsg{state: s})
	if got := loopLabel(t, m); got != "↻ LOOP ALL" {
		t.Fatalf("LOOP reads %q right after the change; want ALL", got)
	}
	c.advance(loopHold)
	m, _ = step(t, m, stateMsg{state: s})
	if got := loopLabel(t, m); got != "↻ LOOP OFF" {
		t.Fatalf("LOOP reads %q %v after the change; want the player's OFF", got, loopHold)
	}
}
