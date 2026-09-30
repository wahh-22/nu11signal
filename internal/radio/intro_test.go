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

// introModel is fxModel with the clock past the intros of its first
// frames.
func introModel(t *testing.T, c *clock) Model {
	t.Helper()
	m := fxModel(t, c)
	c.advance(introDur)
	return m
}

// scrambledAt lists, by row, the cells where lines differ from base.
func scrambledAt(base, lines []string) map[int][]int {
	out := map[int][]int{}
	for y := range base {
		b, l := cells(base[y]), cells(lines[y])
		for x := range min(len(b), len(l)) {
			if b[x] != l[x] {
				out[y] = append(out[y], x)
			}
		}
	}
	return out
}

func TestATabSwitchIntrosTheNewText(t *testing.T) {
	c := newClock()
	m := introModel(t, c)
	seq := m.intro.seq
	m, _ = press(t, m, "/")
	if m.intro.seq == seq {
		t.Fatal("opening SEARCH started no intro")
	}
	base, zs := m.baseLayout()
	lines, got := m.layout()
	if !reflect.DeepEqual(zs, got) {
		t.Fatal("the intro moved the zones")
	}
	scrambled := scrambledAt(base, lines)
	if len(scrambled) == 0 {
		t.Fatal("nothing scrambles at the start of the intro")
	}
	listW := listPanelWidthFor(m.width)
	input, _ := zs.find(zoneInput)
	for y, xs := range scrambled {
		if y < 3 || y >= m.height-3 || y == input.y {
			t.Errorf("row %d scrambled, outside the list panel's rows or on the input", y)
		}
		for _, x := range xs {
			if x < 1 || x >= listW-1 {
				t.Errorf("cell %d,%d scrambled, outside the list panel", x, y)
			}
			if !textCell(cells(base[y])[x]) {
				t.Errorf("cell %d,%d scrambled, not text: %q", x, y, cells(base[y])[x])
			}
		}
	}
	for y := range base {
		if skeleton(lines[y]) != skeleton(base[y]) {
			t.Fatalf("row %d changed its styles or width:\n%q\n%q", y, skeleton(lines[y]), skeleton(base[y]))
		}
	}
	// Left to right: late in the intro the left cells have resolved and
	// some on the right still scramble.
	c.advance(introDur * 7 / 10)
	late := scrambledAt(base, first(m.layout()))
	lo, hi := listW, 0
	for _, xs := range late {
		for _, x := range xs {
			lo, hi = min(lo, x), max(hi, x)
		}
	}
	if len(late) == 0 || lo < listW/4 {
		t.Fatalf("at 70%% the scrambled cells span columns %d..%d", lo, hi)
	}
	// Resolved at the end, to the very frame.
	c.advance(introDur * 3 / 10)
	if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
		t.Fatal("the intro did not resolve to the frame")
	}
}

func TestAPageLoadingIntrosItsRows(t *testing.T) {
	c := newClock()
	m := introModel(t, c)
	seq := m.intro.seq
	m = openStation(t, m, 0)
	// The push shows the loading page; its tracks arriving intro again.
	if m.top().kind != viewPlaylist || m.intro.seq < seq+2 {
		t.Fatalf("opening a playlist started %d intros; want the push and the tracks", m.intro.seq-seq)
	}
	base, _ := m.baseLayout()
	if len(scrambledAt(base, first(m.layout()))) == 0 {
		t.Fatal("the tracks did not intro")
	}
	if !strings.Contains(ansi.Strip(strings.Join(base, "\n")), "NIGHTCALL") {
		t.Fatalf("no tracks:\n%s", plain(m))
	}
}

func TestAnimationsNeverIntro(t *testing.T) {
	c := newClock()
	m := introModel(t, c)
	seq := m.intro.seq
	// The clock, the progress, the bars and the visualizer move; the
	// position and the volume change.
	for range 12 {
		c.advance(fastTick)
		m = tick(t, m)
	}
	m, _ = step(t, m, stateMsg{state: playing(150*time.Second, 225*time.Second)})
	m = withVolume(t, m, 0.3)
	m, _ = press(t, m, "down", "down", "up")
	if m.intro.seq != seq {
		t.Fatalf("animations, a seek, the volume or the cursor started %d intros", m.intro.seq-seq)
	}
}

func TestANewArtistIntrosInNowPlaying(t *testing.T) {
	c := newClock()
	m := introModel(t, c)
	next := playing(0, 200*time.Second)
	next.SongID, next.Title, next.Artist, next.Album = "s-2", "Other Song", "Other Artist", "Other Album"
	m, _ = step(t, m, stateMsg{state: next})
	base, _ := m.baseLayout()
	scrambled := scrambledAt(base, first(m.layout()))
	if len(scrambled) == 0 {
		t.Fatal("a new artist and album did not intro")
	}
	panelTop := 2
	for y := range scrambled {
		row := y - panelTop - 1
		if !slicesContains(nowPlayingFieldRows, row) {
			t.Errorf("row %d (panel row %d) scrambled; only the field rows %v may", y, row, nowPlayingFieldRows)
		}
	}
	plainLines := strings.Split(ansi.Strip(strings.Join(base, "\n")), "\n")
	for i, want := range []string{"OTHER ARTIST", "OTHER ALBUM", "CATALOG FEED"} {
		if !strings.Contains(plainLines[panelTop+1+nowPlayingFieldRows[i]], want) {
			t.Errorf("field row %d does not show %q", nowPlayingFieldRows[i], want)
		}
	}
}

func slicesContains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestTypingNeverScramblesTheInput(t *testing.T) {
	c := newClock()
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.PlaylistsResult = stations()
	m := New(f, Options{Now: c.now, Seed: 2077, Effects: true})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	m, _ = press(t, m, "/")
	c.advance(introDur)
	seq := m.intro.seq
	m = typeText(t, m, "daft")
	if m.intro.seq != seq {
		t.Fatalf("keystrokes started %d intros", m.intro.seq-seq)
	}
	m, cmd = step(t, m, searchDebounceMsg{seq: m.search.seq})
	m, _ = step(t, m, run(t, cmd))
	if m.intro.seq != seq+1 {
		t.Fatalf("the results arriving started %d intros; want 1", m.intro.seq-seq)
	}
	base, zs := m.baseLayout()
	input, _ := zs.find(zoneInput)
	for _, d := range []time.Duration{0, introDur / 4, introDur / 2} {
		c.advance(d)
		lines, _ := m.layout()
		if lines[input.y] != base[input.y] {
			t.Fatalf("the input scrambled: %q", lines[input.y])
		}
		if d == 0 && len(scrambledAt(base, lines)) == 0 {
			t.Fatal("the results did not intro")
		}
	}
}

func TestCalmMeansNoIntros(t *testing.T) {
	c := newClock()
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	m := loaded(t, f, c) // effects off, as with --calm
	m, _ = press(t, m, "/")
	if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
		t.Fatal("an intro was drawn with the effects off")
	}
	// x turns the effects off mid-intro.
	m = introModel(t, c)
	m, _ = press(t, m, "/", "esc", "x")
	if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
		t.Fatal("an intro was drawn after x")
	}
}

func TestAnIntroRaisesTheTickOnlyWhileItRuns(t *testing.T) {
	c := newClock()
	m := introModel(t, c)
	paused := playing(90*time.Second, 225*time.Second)
	paused.Status = playback.StatusPaused
	m, _ = step(t, m, stateMsg{state: paused})
	for i := 0; m.tickFast; i++ {
		if i == 30 {
			t.Fatal("never idle")
		}
		c.advance(fastTick)
		m = tick(t, m)
	}
	m, cmd := press(t, m, "/")
	if cmd == nil || !m.tickFast || m.tickInterval() > introTick {
		t.Fatalf("intro ticks at %v (fast %v)", m.tickInterval(), m.tickFast)
	}
	for i := 0; m.tickFast; i++ {
		if i == 30 {
			t.Fatal("still fast after the intro")
		}
		c.advance(introTick)
		m = tick(t, m)
	}
	if m.intro.running(c.t) {
		t.Fatal("slowed down during the intro")
	}
}
