package radio

import (
	"math"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Content intros: text that was not on screen — another tab or page, a
// list or search results arriving, the playlist picker or editor opening,
// the KEYS or SETTINGS overlay opening or closing, a new artist, album
// or feed in NOW PLAYING — scrambles in softly, about
// introShare of its cells in light glyphs (introGlyphs: letters, digits,
// a few thin symbols), and resolves left to right, easing out, over
// introDur, each cell keeping its style.
//
// What is new is found by comparing frames, but only where content lives
// and only when a message could have brought some. After every message
// but those of introQuiet (see there), the frames before and after it are
// laid out at the same instant, so the clock, the progress and the visualizer read the same in
// both, and only the intro regions are compared: the list panel's inside
// and the artist and album rows of NOW PLAYING and the feed at its head
// (the status tag beside the feed is left out, the title glitches
// on its own; the progress, transport, volume and visualizer rows are
// left out) or, while it is open, the KEYS or SETTINGS overlay's inside
// (the SETTINGS cursor and the ◉ of the theme applied only change marks,
// see rowText). A row of a region is new when its text, its text cells
// without the selection marks and borders, is on no row of that region
// before: a row that only moved (a scroll) or changed its marks or its
// end (the cursor, see sameRow) is not. The cells of a new row that
// changed scramble. The cells of an intro still running carry into the
// next frame only where they still hold the same text: a cell whose text
// moved away (a scroll, the cursor) stops scrambling.
//
// The SEARCH input and the NEW PLAYLIST name, found by their zones
// wherever they are drawn, never scramble, and a key typed into them is
// not compared at all: live results intro once, as
// they arrive. Intros follow the signal effects' switch (off with x or
// --calm) and skip the tiny layout and the auth error screen but, off the
// input line, they also run while typing. Like the other effects they
// draw over the finished frame with setCells, so no width or zone
// changes, and the tick runs at introTick only while one animates.
const (
	introDur = 900 * time.Millisecond
	// introShare of the new cells scramble, each cell's lot drawn from
	// the seed (see introScrambles); the others show their text at once.
	// A scrambled cell stays so for introHold of the intro, then resolves
	// as the eased front (see introResolve) passes its column, give or
	// take introJitter. It shows a new glyph every introGlyph, each at its
	// own phase, so the text drifts instead of jumping; the tick runs at
	// introTick, the renderer's frame, so every change and every step of
	// the front reaches the screen.
	introShare  = 0.5
	introHold   = 0.2
	introJitter = 0.1
	introGlyph  = 140 * time.Millisecond
	introTick   = 50 * time.Millisecond
)

// introGlyphs are what a scrambled cell shows: uppercase letters, digits
// and a few thin symbols, no blocks or shades (the title glitch and the
// bursts keep theirs, see glitchGlyphs and noiseGlyphs).
var introGlyphs = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+-=:/<>")

// introScrambles reports whether the new cell with hash h scrambles.
func introScrambles(h uint64) bool { return unit(mix(h, 2)) < introShare }

const saltIntro uint64 = 401

// introFieldRows are the rows of the NOW PLAYING inside that intro: the
// artist, the album and the head, as nowPlaying lays them out. Of the
// head only the feed's cells count, not the status tag right of them
// (see headFeedWidth).
var introFieldRows = []int{npArtistRow, npAlbumRow, npHeadRow}

// A row reading b is the row that read a (see sameRow) when they share at
// least sameRowMinShared runes of their start, and at least sameRowShare
// of the shorter one.
const (
	sameRowMinShared = 4
	sameRowShare     = 0.6
)

// intro is the latest content intro, number seq, from start: the cells
// that scramble, as columns by row, each once. It is a value: cells is
// never changed once made, only replaced.
type intro struct {
	seq   uint64
	start time.Time
	cells map[int][]int
}

func (in intro) running(now time.Time) bool {
	return !in.start.IsZero() && !now.Before(in.start) && now.Before(in.start.Add(introDur))
}

// introOn reports whether intros run: with the signal effects on, but not
// on the tiny layout or the auth error screen.
func (m Model) introOn() bool {
	return m.fx.on && m.auth != authFailed && m.width >= tinyMinWidth && m.height >= tinyMinHeight
}

// introAnimating reports whether an intro draws now.
func (m Model) introAnimating() bool { return m.introOn() && m.intro.running(m.now()) }

// introQuiet reports whether msg cannot bring intro content, so the
// frames are not laid out twice for it: the animation tick and a resize
// (which animate or reflow), and the replies of the player's controls,
// which change only the volume, the progress, LOOP or the status line,
// none of them compared. Any other message may, a state (a new artist,
// album or feed) included, and is compared.
func introQuiet(msg tea.Msg) bool {
	switch msg.(type) {
	case tickMsg, tea.WindowSizeMsg, volumeMsg, setVolumeMsg, seekMsg, loopMsg:
		return true
	}
	return false
}

// withIntro starts an intro for the text m shows that prev, the Model
// before msg, did not (see the top of this file). The cells of an intro
// still running go on scrambling where their text stayed, in the new one
// or, with nothing new, in the same one.
func (m Model) withIntro(prev Model, msg tea.Msg) Model {
	if introQuiet(msg) {
		return m
	}
	if _, ok := msg.(tea.KeyPressMsg); ok && prev.typing() && m.typing() {
		return m
	}
	if !m.introOn() || prev.width != m.width || prev.height != m.height {
		return m
	}
	before, _ := prev.baseLayout()
	after, zs := m.baseLayout()
	now := m.now()
	cells := map[int][]int{}
	for _, region := range m.introRegions(zs) {
		newCells(before, after, region, cells)
	}
	carried, dropped := m.intro.carried(now, before, after)
	if len(cells) == 0 {
		if dropped {
			// Same seq and start: what is left resolves on time.
			m.intro.cells = carried
		}
		return m
	}
	for y, xs := range carried {
		cells[y] = slices.Compact(slices.Sorted(slices.Values(append(cells[y], xs...))))
	}
	m.intro = intro{seq: m.intro.seq + 1, start: now, cells: cells}
	return m
}

// carried are the cells of in, if it runs at now, that still hold the
// text they held from before to after; dropped reports whether any did
// not (moved text, a scroll or the cursor), which must stop scrambling.
func (in intro) carried(now time.Time, before, after []string) (cells map[int][]int, dropped bool) {
	if !in.running(now) {
		return nil, false
	}
	cells = map[int][]int{}
	for y, xs := range in.cells {
		var b, a []rune
		if y < len(before) && y < len(after) {
			b, a = cellRunes(before[y]), cellRunes(after[y])
		}
		for _, x := range xs {
			if x < len(b) && x < len(a) && a[x] != 0 && a[x] == b[x] {
				cells[y] = append(cells[y], x)
			} else {
				dropped = true
			}
		}
	}
	return cells, dropped
}

// rowSpan is the cells x0 to x1 (excluded) of row y.
type rowSpan struct{ y, x0, x1 int }

// introRegions are the rows compared for new content: the KEYS or
// SETTINGS overlay's inside while one is open (it hides the rest);
// otherwise the list panel's inside, and the NOW PLAYING field rows in
// the full layout; never the rows of the SEARCH input or the NEW
// PLAYLIST name, where their zones put them.
func (m Model) introRegions(zs zones) [][]rowSpan {
	full := m.width >= fullMinWidth && m.height >= fullMinHeight
	var list, player, overlay []zone
	for _, z := range zs {
		switch z.id {
		case zonePanelList:
			list = append(list, z)
		case zonePanelPlayer:
			player = append(player, z)
		case zonePanelOverlay:
			overlay = append(overlay, z)
		}
	}
	if len(overlay) > 2 {
		var rows []rowSpan
		for _, z := range overlay[1 : len(overlay)-1] { // the frame
			rows = append(rows, rowSpan{z.y, z.x + 1, z.x + z.w - 1})
		}
		return [][]rowSpan{rows}
	}
	var inputs []int
	for _, id := range []string{zoneInput, zoneNameInput} {
		if z, ok := zs.find(id); ok {
			inputs = append(inputs, z.y)
		}
	}
	var rows []rowSpan
	if full && len(list) > 2 {
		list = list[1 : len(list)-1] // the frame
		for _, z := range list {
			rows = append(rows, rowSpan{z.y, z.x + 1, z.x + z.w - 1})
		}
	} else {
		for _, z := range list {
			rows = append(rows, rowSpan{z.y, z.x, z.x + z.w})
		}
	}
	rows = slices.DeleteFunc(rows, func(r rowSpan) bool { return slices.Contains(inputs, r.y) })
	regions := [][]rowSpan{rows}
	if full && len(player) > 0 {
		var fields []rowSpan
		z := player[0]
		for _, r := range introFieldRows {
			x0, x1 := z.x+1+nowPlayingMargin, z.x+z.w-1
			if r == npHeadRow {
				x1 = min(x1, x0+m.headFeedWidth(z.w-2))
			}
			fields = append(fields, rowSpan{z.y + 1 + r, x0, x1})
		}
		regions = append(regions, fields)
	}
	return regions
}

// newCells adds to cells the cells of region that scramble from before to
// after: the changed text cells of its new rows.
func newCells(before, after []string, region []rowSpan, cells map[int][]int) {
	var seen []string
	for _, r := range region {
		if r.y < len(before) {
			seen = append(seen, rowText(cellRunes(before[r.y]), r))
		}
	}
	for _, r := range region {
		if r.y >= len(after) {
			continue
		}
		now := cellRunes(after[r.y])
		if text := rowText(now, r); text == "" || slices.ContainsFunc(seen, func(old string) bool { return sameRow(old, text) }) {
			continue
		}
		var old []rune
		if r.y < len(before) {
			old = cellRunes(before[r.y])
		}
		for x := r.x0; x < min(r.x1, len(now)); x++ {
			if now[x] != 0 && (x >= len(old) || old[x] != now[x]) {
				cells[r.y] = append(cells[r.y], x)
			}
		}
	}
}

// cellRunes is line cell by cell: the rune of each text cell (see
// textRune), 0 for any other cell. A wide rune takes two cells.
func cellRunes(line string) []rune {
	rs := []rune(ansi.Strip(line))
	out := make([]rune, 0, len(rs))
	for i, r := range rs {
		switch w := runeWidth(r); {
		case w == 0:
			if len(out) > 0 {
				out[len(out)-1] = 0 // combined: not one plain cell
			}
		case w == 1 && textRune(r) && (i+1 >= len(rs) || runeWidth(rs[i+1]) != 0):
			out = append(out, r)
		default:
			for range w {
				out = append(out, 0)
			}
		}
	}
	return out
}

// sameRow reports whether a row reading b is the row that read a, moved
// or marked: the same text, or text that shares most of its start, in
// runes (the selected row's truncated title and trailing <3 +). The
// trade-off: two different rows that share most of their start (a
// "PART 1" and a "PART 2") count as one that moved, and do not intro.
func sameRow(a, b string) bool {
	if a == b {
		return true
	}
	ra, rb := []rune(a), []rune(b)
	short := min(len(ra), len(rb))
	n := 0
	for n < short && ra[n] == rb[n] {
		n++
	}
	return n >= sameRowMinShared && float64(n) >= sameRowShare*float64(short)
}

// rowText is the text of the cells of r, its words joined by spaces,
// without marks: the geometric shapes (U+25A0 to U+25FF) that mark the
// selected or playing row.
func rowText(cells []rune, r rowSpan) string {
	var b strings.Builder
	gap := false
	for x := r.x0; x < min(r.x1, len(cells)); x++ {
		if cells[x] == 0 || (cells[x] >= 0x25A0 && cells[x] <= 0x25FF) {
			gap = true
			continue
		}
		if gap && b.Len() > 0 {
			b.WriteByte(' ')
		}
		gap = false
		b.WriteRune(cells[x])
	}
	return b.String()
}

// drawIntro scrambles the cells of the running intro in lines, those
// whose lot says so (see introScrambles): each holds an intro glyph, a
// new one every introGlyph at its own phase, until its moment to
// resolve, the columns from the left to the right (see introResolve). A
// cell that no longer holds text is left alone.
func (m Model) drawIntro(lines []string) {
	now := m.now()
	if !m.intro.running(now) {
		return
	}
	lo, hi := -1, 0
	for _, xs := range m.intro.cells {
		for _, x := range xs {
			if lo < 0 || x < lo {
				lo = x
			}
			hi = max(hi, x)
		}
	}
	elapsed := now.Sub(m.intro.start)
	p := float64(elapsed) / float64(introDur)
	n := uint64(len(introGlyphs))
	for y, xs := range m.intro.cells {
		if y >= len(lines) {
			continue
		}
		row := cellRunes(lines[y])
		repl := map[int]rune{}
		for _, x := range xs {
			if x >= len(row) || row[x] == 0 {
				continue
			}
			h := mix(m.seed, saltIntro, m.intro.seq, uint64(y), uint64(x))
			if !introScrambles(h) || p >= introResolve(h, float64(x-lo)/float64(max(hi-lo, 1))) {
				continue
			}
			phase := time.Duration(h % uint64(introGlyph))
			repl[x] = introGlyphs[mix(h, uint64((elapsed+phase)/introGlyph))%n]
		}
		if len(repl) > 0 {
			lines[y] = setCells(lines[y], repl)
		}
	}
}

// introResolve is when, as a share of the intro, the cell with hash h at
// column share col resolves: after introHold, the further right the later.
// The front eases out, 1-(1-t)² of the columns resolved at share t of the
// resolve: it sweeps the left fast and settles gently on the right.
func introResolve(h uint64, col float64) float64 {
	eased := 1 - math.Sqrt(1-min(max(col, 0), 1))
	return introHold + (1-introHold-introJitter)*eased + introJitter*unit(mix(h, 1))
}
