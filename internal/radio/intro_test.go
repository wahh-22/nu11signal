package radio

import (
	"fmt"
	"reflect"
	"slices"
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
	start := m.intro.start
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
	c.t = start.Add(introDur * 7 / 10)
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
	c.t = start.Add(introDur)
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
		if !slices.Contains(introFieldRows, row) {
			t.Errorf("row %d (panel row %d) scrambled; only the field rows %v may", y, row, introFieldRows)
		}
	}
	plainLines := strings.Split(ansi.Strip(strings.Join(base, "\n")), "\n")
	for i, want := range []string{"OTHER ARTIST", "OTHER ALBUM", "CATALOG FEED"} {
		if !strings.Contains(plainLines[panelTop+1+introFieldRows[i]], want) {
			t.Errorf("field row %d does not show %q", introFieldRows[i], want)
		}
	}
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
	start := c.t
	for _, d := range []time.Duration{0, introDur / 4, introDur / 2} {
		c.t = start.Add(d) // d into the intro
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

// introModelSized is introModel at w x h with lists playlists.
func introModelSized(t *testing.T, c *clock, w, h int, lists []playback.Playlist) Model {
	t.Helper()
	f := playbacktest.New()
	f.PlaylistsResult = lists
	m := New(f, Options{Now: c.now, Seed: 2077, Effects: true})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	for _, msg := range runAll(t, cmd) {
		if _, ok := msg.(tickMsg); !ok {
			m, _ = step(t, m, msg)
		}
	}
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	for range glitchFrames + 1 {
		m = tick(t, m)
	}
	m.intro = intro{}
	c.advance(introDur)
	return m
}

func manyStations(n int) []playback.Playlist {
	out := make([]playback.Playlist, n)
	for i := range out {
		out[i] = playback.Playlist{ID: fmt.Sprintf("pl-%d", i), Name: fmt.Sprintf("Station %c%c", 'A'+i%26, 'a'+i*7%26)}
	}
	return out
}

// regionRows are the rows of the intro regions of m's frame.
func regionRows(m Model) (list, fields []int) {
	_, zs := m.baseLayout()
	regions := m.introRegions(zs)
	for _, r := range regions[0] {
		list = append(list, r.y)
	}
	if len(regions) > 1 {
		for _, r := range regions[1] {
			fields = append(fields, r.y)
		}
	}
	return list, fields
}

func TestTheNameInputRowIsFoundFromItsZone(t *testing.T) {
	c := newClock()
	m := introModel(t, c)
	m, _ = m.openName(playback.Song{}, false)
	m = typeText(t, m, "Mix")
	for _, size := range []struct{ w, h int }{{80, 24}, {50, 20}} {
		m.width, m.height = size.w, size.h
		base, zs := m.baseLayout()
		name, ok := zs.find(zoneNameInput)
		if !ok {
			t.Fatalf("%dx%d: no zone for the name input", size.w, size.h)
		}
		if got := cells(base[name.y]); !slices.Contains(got, "M") {
			t.Fatalf("%dx%d: the name zone's row %d does not show the name: %q", size.w, size.h, name.y, base[name.y])
		}
		list, _ := regionRows(m)
		if slices.Contains(list, name.y) {
			t.Errorf("%dx%d: the name row %d is compared", size.w, size.h, name.y)
		}
	}
	// Wherever the name is drawn: here on the third row of the list.
	m.width, m.height = 80, 24
	_, zs := m.baseLayout()
	var moved zones
	third := -1
	listRows := 0
	for _, z := range zs {
		switch z.id {
		case zoneNameInput:
			continue
		case zonePanelList:
			if listRows++; listRows == 4 {
				third = z.y
			}
		}
		moved = append(moved, z)
	}
	moved = append(moved, zone{id: zoneNameInput, x: 1, y: third, w: 10})
	var rows []int
	for _, r := range m.introRegions(moved)[0] {
		rows = append(rows, r.y)
	}
	if slices.Contains(rows, third) || !slices.Contains(rows, third-2) {
		t.Fatalf("with the name on row %d, the compared rows are %v", third, rows)
	}
}

func TestIntroFieldRowsAreNowPlayingsFields(t *testing.T) {
	for _, tt := range []struct {
		name     string
		w, h     int
		expanded bool
	}{
		{"full", 80, 24, false},
		{"full tall", 120, 40, false},
		{"expanded", 80, 24, true},
		{"compact", 50, 20, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newClock()
			m := introModel(t, c)
			m.width, m.height, m.expanded = tt.w, tt.h, tt.expanded
			base, zs := m.baseLayout()
			_, fields := regionRows(m)
			if tt.name == "compact" {
				if len(fields) != 0 {
					t.Fatalf("compact compares NOW PLAYING rows %v", fields)
				}
				return
			}
			want := []string{"SAMURAI", "NEVER FADE AWAY", "CATALOG FEED"}
			if len(fields) != len(want) {
				t.Fatalf("field rows %v", fields)
			}
			for i, y := range fields {
				if text := ansi.Strip(base[y]); !strings.Contains(text, want[i]) {
					t.Errorf("field row %d reads %q; want %q", y, text, want[i])
				}
			}
			for _, id := range []string{zoneSeek, zonePlay, zoneVolUp, zoneLoop} {
				if z, ok := zs.find(id); ok && slices.Contains(fields, z.y) {
					t.Errorf("the %s row %d is compared", id, z.y)
				}
			}
		})
	}
}

func TestSameRowComparesRunes(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"SAME", "SAME", true},
		{"NIGHT DRIVE", "NIGHT DRIVE ♥ +", true},
		// Six of nine runes shared: most of the start, the same row.
		{"ABCDEFÑÑÑ", "ABCDEFÉÉÉ", true},
		// Five of nine: another row, however many bytes the Ñ share.
		{"ÑÑÑÑÑABCD", "ÑÑÑÑÑWXYZ", false},
		{"ÁB", "ÁC", false},
	} {
		if got := sameRow(tt.a, tt.b); got != tt.want {
			t.Errorf("sameRow(%q, %q) = %v; want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestAScrollDropsTheIntroCellsOfMovedText(t *testing.T) {
	c := newClock()
	m := introModelSized(t, c, 80, 16, manyStations(30))
	// An intro of the whole list, as when it arrived.
	base, zs := m.baseLayout()
	running := map[int][]int{}
	for _, r := range m.introRegions(zs)[0] {
		row := cellRunes(base[r.y])
		for x := r.x0; x < min(r.x1, len(row)); x++ {
			if row[x] != 0 {
				running[r.y] = append(running[r.y], x)
			}
		}
	}
	m.intro = intro{seq: 1, start: c.t, cells: running}
	c.advance(introDur / 10)
	scrolled := false
	for range 20 {
		prev := m
		m, _ = press(t, m, "down")
		before, _ := prev.baseLayout()
		after, _ := m.baseLayout()
		fresh := map[int][]int{}
		for _, region := range m.introRegions(zs) {
			newCells(before, after, region, fresh)
		}
		for y, xs := range m.intro.cells {
			if len(slices.Compact(slices.Sorted(slices.Values(xs)))) != len(xs) {
				t.Fatalf("row %d holds a cell twice: %v", y, xs)
			}
			b, a := cellRunes(before[y]), cellRunes(after[y])
			for _, x := range xs {
				if slices.Contains(fresh[y], x) {
					continue
				}
				if x >= len(b) || x >= len(a) || b[x] != a[x] {
					t.Fatalf("cell %d,%d scrambles on moved text", x, y)
				}
			}
		}
		if !reflect.DeepEqual(before, after) && len(fresh) > 0 {
			scrolled = true
		}
	}
	if !scrolled {
		t.Fatal("the list never scrolled")
	}
}

func TestPlayerControlMessagesNeverStartAnIntro(t *testing.T) {
	c := newClock()
	prev := introModel(t, c)
	m, _ := press(t, prev, "/")
	m.intro = prev.intro
	for _, msg := range []tea.Msg{
		volumeMsg{level: 0.3, epoch: m.volumeEpoch},
		setVolumeMsg{level: 0.3, epoch: m.volumeEpoch},
		seekMsg{},
		loopMsg{},
		tickMsg{gen: m.tickGen},
		tea.WindowSizeMsg{Width: 80, Height: 24},
	} {
		// Even with new text on screen, these are not compared.
		if got := m.withIntro(prev, msg); got.intro.seq != prev.intro.seq {
			t.Errorf("%T started an intro", msg)
		}
	}
}

func TestIntrosStayOffOnTheTinyLayoutAndTheAuthScreen(t *testing.T) {
	c := newClock()
	tiny := introModel(t, c)
	tiny, _ = step(t, tiny, tea.WindowSizeMsg{Width: 15, Height: 4})
	seq := tiny.intro.seq
	tiny, _ = press(t, tiny, "/")
	tiny, _ = step(t, tiny, stateMsg{state: playing(0, 200*time.Second)})
	if tiny.intro.seq != seq || tiny.introAnimating() {
		t.Fatal("an intro started on the tiny layout")
	}
	if lines, _ := tiny.layout(); !reflect.DeepEqual(lines, first(tiny.baseLayout())) {
		t.Fatal("an intro was drawn on the tiny layout")
	}

	f := playbacktest.New()
	f.AuthStatus = playback.AuthDenied
	m := New(f, Options{Now: c.now, Seed: 2077, Effects: true})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	prev := m
	m, _ = step(t, m, run(t, m.authorizeCmd()))
	if m.auth != authFailed {
		t.Fatalf("auth %v; want the error screen", m.auth)
	}
	if m.intro.seq != prev.intro.seq || m.introAnimating() {
		t.Fatal("the auth error screen intro'd")
	}
	m, _ = press(t, m, "/")
	if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
		t.Fatal("an intro was drawn on the auth error screen")
	}
}
