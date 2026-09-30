package radio

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// transportIDs are the zones of the transport row, left to right.
var transportIDs = []string{zonePrev, zonePlay, zoneNext, zoneLoop, zoneExpand}

// barText is the unstyled text a row draws under each of its zones, by ID.
func barText(bar string, zs zones) map[string]string {
	plainBar := ansi.Strip(bar)
	out := map[string]string{}
	for _, z := range zs {
		out[z.id] = ansi.Cut(plainBar, z.x, z.x+z.w)
	}
	return out
}

// assertWholeButtons checks that every zone of a row covers one whole
// bracket button, left to right without overlapping, and that the row
// fits in w cells.
func assertWholeButtons(t *testing.T, bar string, zs zones, w int) {
	t.Helper()
	if got := ansi.StringWidth(bar); got > w {
		t.Fatalf("row is %d cells in %d: %q", got, w, ansi.Strip(bar))
	}
	end := 0
	for id, text := range barText(bar, zs) {
		if !(strings.HasPrefix(text, "[") || strings.HasPrefix(text, "▸")) || !strings.HasSuffix(text, "]") {
			t.Errorf("width %d: %s zone covers %q; want a whole [button]", w, id, text)
		}
	}
	for _, z := range zs {
		if z.x < end {
			t.Errorf("width %d: %s zone at %d overlaps the one before (ends at %d)", w, z.id, z.x, end)
		}
		end = z.x + z.w
	}
}

func TestTransportRowAt60Cells(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	bar, zs := m.transportBar(60, false)
	got := ansi.Strip(bar)
	const lead = "[◀◀]  [ ❚❚ PAUSE ]  [▶▶]      [↻ OFF]"
	if !strings.HasPrefix(got, lead) || !strings.HasSuffix(got, "[⤢]") || ansi.StringWidth(got) != 60 {
		t.Fatalf("transport row %q; want %q with [⤢] at the right edge of 60 cells", got, lead)
	}
	want := map[string]string{
		zonePrev: "[◀◀]", zonePlay: "[ ❚❚ PAUSE ]", zoneNext: "[▶▶]", zoneLoop: "[↻ OFF]", zoneExpand: "[⤢]",
	}
	if texts := barText(bar, zs); !reflect.DeepEqual(texts, want) {
		t.Fatalf("zones cover %v; want %v", texts, want)
	}
	if z, _ := zs.find(zoneExpand); z.x+z.w != 60 {
		t.Fatalf("EXPAND ends at %d; want the right edge, 60", z.x+z.w)
	}

	// Paused, looping the queue and expanded.
	s := playing(time.Minute, 3*time.Minute)
	s.Status, s.Repeat = playback.StatusPaused, playback.RepeatAll
	m, _ = step(t, m, stateMsg{state: s})
	m, _ = press(t, m, "f")
	m, _ = press(t, m, "left", "left") // off PLAY: its marker would hide the bracket
	bar, zs = m.transportBar(60, false)
	texts := barText(bar, zs)
	if texts[zonePlay] != "[ ▶ PLAY ]" || texts[zoneLoop] != "[↻ ALL]" || texts[zoneExpand] != "[⤡]" {
		t.Fatalf("paused, looping, expanded: zones cover %v", texts)
	}
}

func TestTransportRowDegradesWithoutOverflow(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	for w := 0; w <= 70; w++ {
		bar, zs := m.transportBar(w, false)
		assertWholeButtons(t, bar, zs, w)
		packed, pz := m.transportBar(w, true)
		assertWholeButtons(t, packed, pz, w)
	}
	// The label goes first, then the room around the buttons; all five
	// stay down to the narrowest NOW PLAYING.
	for _, tt := range []struct {
		w    int
		play string
	}{
		{42, "[ ❚❚ PAUSE ]"},
		{36, "[ ❚❚ ]"},
		{26, "[❚❚]"},
	} {
		bar, zs := m.transportBar(tt.w, false)
		texts := barText(bar, zs)
		if len(zs) != len(transportIDs) || texts[zonePlay] != tt.play {
			t.Errorf("width %d: row %q; want all five buttons, PLAY as %s", tt.w, ansi.Strip(bar), tt.play)
		}
	}
	// Narrower, the last buttons give way; PREV, PLAY and NEXT stay.
	bar, zs := m.transportBar(16, false)
	for _, id := range transportIDs[:3] {
		if _, ok := zs.find(id); !ok {
			t.Errorf("width 16: row %q lacks %s", ansi.Strip(bar), id)
		}
	}
}

func TestLoopSharesTheTransportRowAndVolumeSitsUnderIt(t *testing.T) {
	for _, keys := range [][]string{nil, {"f"}} {
		m := withVolume(t, playingModel(t, playbacktest.New()), 0.9)
		m, _ = press(t, m, keys...)
		play := zoneOf(t, m, zonePlay)
		for _, id := range transportIDs {
			if z := zoneOf(t, m, id); z.y != play.y {
				t.Errorf("keys %v: %s on row %d; want the transport row %d", keys, id, z.y, play.y)
			}
		}
		down, up := zoneOf(t, m, zoneVolDown), zoneOf(t, m, zoneVolUp)
		if down.y != play.y+1 || up.y != play.y+1 {
			t.Fatalf("keys %v: volume buttons on rows %d, %d; want %d", keys, down.y, up.y, play.y+1)
		}
		row := strings.Split(plain(m), "\n")[down.y]
		if !strings.Contains(row, "VOL [−] ▮") || !strings.Contains(row, "[+] 90%") {
			t.Fatalf("keys %v: volume row %q; want VOL [−] <meter> [+] 90%%", keys, row)
		}
		if textAt(m, down) != "[−]" || textAt(m, up) != "[+]" {
			t.Fatalf("volume zones cover %q and %q", textAt(m, down), textAt(m, up))
		}
		// Nothing of the old LOOP row is left under the volume row.
		if next := strings.Split(plain(m), "\n")[down.y+1]; strings.Contains(next, "↻") {
			t.Fatalf("keys %v: LOOP still drawn under the volume: %q", keys, next)
		}
	}
}

func TestVolumeRowMeterFillsTheRoom(t *testing.T) {
	m := withVolume(t, loaded(t, playbacktest.New(), newClock()), 0.5)
	for _, w := range []int{16, 20, 27, 37, 60} {
		bar, zs := m.volumeBar(w)
		assertWholeButtons(t, bar, zs, w)
		if len(zs) != 2 {
			t.Errorf("width %d: volume row %q has zones %v; want [−] and [+]", w, ansi.Strip(bar), zs)
		}
	}
	bar, _ := m.volumeBar(60)
	if got := strings.Count(ansi.Strip(bar), "▮") + strings.Count(ansi.Strip(bar), "▯"); got != volumeMeterMax {
		t.Fatalf("meter %d cells at 60; want the widest, %d", got, volumeMeterMax)
	}
	// Unknown: the buttons stay, where the known level puts them.
	unknown := loaded(t, playbacktest.New(), newClock())
	ub, uz := unknown.volumeBar(27)
	_, kz := m.volumeBar(27)
	if !strings.HasPrefix(ansi.Strip(ub), "VOL [−] --") || !reflect.DeepEqual(uz, kz) {
		t.Fatalf("unknown row %q zones %v; want VOL [−] -- with the buttons at %v", ansi.Strip(ub), uz, kz)
	}
}

func TestExpandedTransportZonesDriveThePlayer(t *testing.T) {
	for _, tt := range []struct {
		id     string
		method string
	}{
		{zonePrev, "Previous"},
		{zonePlay, "Pause"},
		{zoneNext, "Next"},
		{zoneLoop, "SetRepeat"},
		{zoneVolUp, "SetVolume"},
		{zoneVolDown, "SetVolume"},
	} {
		t.Run(tt.id, func(t *testing.T) {
			f := playbacktest.New()
			m := withVolume(t, playingModel(t, f), 0.5)
			m, _ = press(t, m, "f")
			// The last cell of the zone acts as well as the first.
			z := zoneOf(t, m, tt.id)
			m, cmd := step(t, m, tea.MouseClickMsg{X: z.x + z.w - 1, Y: z.y, Button: tea.MouseLeft})
			settle(t, m, cmd)
			if len(callsOf(f, tt.method)) != 1 {
				t.Fatalf("click on %s: calls %v; want one %s", tt.id, f.Calls(), tt.method)
			}
		})
	}
	m := playingModel(t, playbacktest.New())
	m, _ = press(t, m, "f")
	z := zoneOf(t, m, zoneExpand)
	m, _ = step(t, m, tea.MouseClickMsg{X: z.x + z.w - 1, Y: z.y, Button: tea.MouseLeft})
	if m.expanded {
		t.Fatal("a click on RESTORE's last cell did not restore the player")
	}
}

func TestArrowsWalkTheHUDRows(t *testing.T) {
	m := withVolume(t, playingModel(t, playbacktest.New()), 0.5)
	steps := []struct {
		key string
		ctl playerControl
		bar bool
	}{
		{"right", ctlPlay, false},
		{"right", ctlNext, false},
		{"right", ctlLoop, false},
		{"right", ctlExpand, false},
		{"right", ctlExpand, false}, // the last button stays
		{"left", ctlLoop, false},
		{"down", ctlVolUp, false},
		{"down", ctlVolUp, false}, // the bottom row stays
		{"up", ctlNext, false},
		{"up", ctlNext, true}, // the progress bar
		{"down", ctlNext, false},
		{"left", ctlPlay, false},
		{"down", ctlVolDown, false},
		{"up", ctlPrev, false},
	}
	for i, s := range steps {
		m, _ = press(t, m, s.key)
		if m.focus != areaPlayer || m.control != s.ctl || m.onBar != s.bar {
			t.Fatalf("step %d (%s): focus %v control %v bar %v; want %v bar %v",
				i, s.key, m.focus, m.control, m.onBar, s.ctl, s.bar)
		}
	}
}

func TestFocusedHUDButtonsAreMarked(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	m, _ = press(t, m, "right") // PLAY, filled already: the marker shows it
	if got := textAt(m, zoneOf(t, m, zonePlay)); !strings.HasPrefix(got, "▸") {
		t.Fatalf("focused PLAY shows %q; want the ▸ marker", got)
	}
	m, _ = press(t, m, "right", "right") // LOOP
	if got := textAt(m, zoneOf(t, m, zoneLoop)); got != "▸↻ OFF]" {
		t.Fatalf("focused LOOP shows %q; want ▸↻ OFF]", got)
	}
	if got := textAt(m, zoneOf(t, m, zonePlay)); strings.Contains(got, "▸") {
		t.Fatalf("PLAY still marked: %q", got)
	}
}

func TestCompactTransportRowHoldsLoop(t *testing.T) {
	for _, w := range []int{59, 40, 24} {
		f := playbacktest.New()
		m := withVolume(t, playingModel(t, f), 0.5)
		m, _ = step(t, m, tea.WindowSizeMsg{Width: w, Height: 14})
		play := zoneOf(t, m, zonePlay)
		if z, ok := func() (zone, bool) { _, zs := m.layout(); return zs.find(zoneLoop) }(); ok && z.y != play.y {
			t.Fatalf("width %d: LOOP on row %d; want the transport row %d", w, z.y, play.y)
		}
		for _, l := range strings.Split(plain(m), "\n") {
			if ansi.StringWidth(l) > w {
				t.Fatalf("width %d: line %q overflows", w, l)
			}
		}
	}
	// Wide enough, LOOP is there and works.
	f := playbacktest.New()
	m := withVolume(t, playingModel(t, f), 0.5)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 40, Height: 14})
	m, _ = press(t, m, "right", "right", "right")
	if !m.focused(ctlLoop) {
		t.Fatalf("compact: focus on %v; want LOOP", m.control)
	}
	m, cmd := click(t, m, zoneLoop)
	settle(t, m, cmd)
	if got := setRepeatCalls(f); !reflect.DeepEqual(got, []any{playback.RepeatAll}) {
		t.Fatalf("compact: SetRepeat calls %v; want [all]", got)
	}
}
