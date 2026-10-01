package radio

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// The idle emblem: while no music plays (paused, stopped, nothing loaded,
// the signal lost: not isPlaying, the rain's own Playing input) the
// spectrum area at the bottom of NOW PLAYING shows the null emblem with
// its text, centered in the area, instead of the rain (see idleArtFor for
// the sizes it falls back through). The rain keeps its drops, held still
// while paused, and comes back as playback does.
//
// With the signal effects active (see fxActive) the emblem glitches now
// and then, on a schedule of its own, apart from the bursts: every
// idleGapMin..idleGapMax a glitch of idleGlitchMin..idleGlitchMax tears
// one emblem row 1 cell sideways and draws 1..3 block noise cells
// (bootNoiseGlyphs) over the emblem's drawn cells, a new look every tick
// frame, from the seed, the glitch number and the frame. It is drawn
// inside the area, so nothing else moves. Like the bursts it adds no
// timer: the tick sleeps until the next glitch starts, runs at burstTick
// while it draws and lands on its end (see idleGlitch.interval); between
// glitches the paused tick stays at idleTick.
const (
	idleGapMin    = 4 * time.Second
	idleGapMax    = 7 * time.Second
	idleGlitchMin = 150 * time.Millisecond
	idleGlitchMax = 250 * time.Millisecond
)

// Salts keep the idle glitch's streams apart from the other effects'.
const (
	saltIdleGap uint64 = iota + 601
	saltIdleLen
	saltIdle
)

// idleGlitch is the idle emblem's glitch schedule, a value like effects:
// glitch number seq runs from start to end, and the next starts at next,
// zero while the glitch is not active.
type idleGlitch struct {
	seq              uint64
	next, start, end time.Time
}

// advance moves the schedule to now. While not active nothing runs and
// nothing is pending, so becoming active never fires a glitch at once.
func (g idleGlitch) advance(now time.Time, seed uint64, active bool) idleGlitch {
	if !active {
		g.next, g.end = time.Time{}, time.Time{}
		return g
	}
	if g.next.IsZero() {
		g.next = now.Add(span(mix(seed, saltIdleGap, g.seq), idleGapMin, idleGapMax))
	}
	if !now.Before(g.next) {
		g.seq++
		g.start = now
		g.end = now.Add(span(mix(seed, saltIdleLen, g.seq), idleGlitchMin, idleGlitchMax))
		g.next = g.end.Add(span(mix(seed, saltIdleGap, g.seq), idleGapMin, idleGapMax))
	}
	return g
}

// on reports whether a glitch draws at now.
func (g idleGlitch) on(now time.Time) bool {
	return !now.Before(g.start) && now.Before(g.end)
}

// interval cuts d, the time to the next frame, to the glitch: burstTick
// while one draws, landing on its end; else the next one's start.
func (g idleGlitch) interval(now time.Time, d time.Duration) time.Duration {
	if g.on(now) {
		return min(d, max(min(burstTick, g.end.Sub(now)), minWake))
	}
	if !g.next.IsZero() {
		d = min(d, max(g.next.Sub(now), minWake))
	}
	return d
}

// idleShown reports whether the idle emblem is on screen: no music
// playing, the full layout's NOW PLAYING drawn (no splash, KEYS or
// SETTINGS over it) and room for some emblem in its spectrum area.
func (m Model) idleShown() bool {
	if m.isPlaying() || m.boot || m.shutdown || m.help || m.settings {
		return false
	}
	_, ok := idleArtFor(m.vizSize())
	return ok
}

// idleActive reports whether the idle glitch runs: the emblem shown and
// the signal effects active.
func (m Model) idleActive() bool { return m.fxActive() && m.idleShown() }

// idleArt is the emblem the spectrum area shows while idle, with its text
// or not, its block's top left x, y cells into the area.
type idleArt struct {
	e    emblem
	text bool
	x, y int
}

// idleArtFor places the idle emblem in an area w x h, centered: the large
// emblem with its text where it fits, else the compact one with its text,
// else the large then the compact emblem alone; false where none fits.
func idleArtFor(w, h int) (idleArt, bool) {
	for _, text := range []bool{true, false} {
		for _, e := range []emblem{emblemLarge, emblemCompact} {
			bw := e.width()
			if text {
				bw = e.blockWidth()
			}
			if bw <= w && len(e.rows) <= h {
				return idleArt{e: e, text: text, x: (w - bw) / 2, y: (h - len(e.rows)) / 2}, true
			}
		}
	}
	return idleArt{}, false
}

// block is the art's styled lines, one per emblem row, not padded.
func (a idleArt) block() []string {
	if a.text {
		return a.e.block()
	}
	lines := make([]string, len(a.e.rows))
	for i, row := range a.e.rows {
		lines[i] = paintRow(row, a.e.mask[i])
	}
	return lines
}

// plain is the art's lines without their styles.
func (a idleArt) plain() []string {
	lines := a.block()
	for i, l := range lines {
		lines[i] = ansi.Strip(l)
	}
	return lines
}

// lines renders the area w x h: the art at its place, every row padded
// to w cells.
func (a idleArt) lines(w, h int) []string {
	out := make([]string, h)
	block := a.block()
	pad := strings.Repeat(" ", a.x)
	for y := range out {
		line := ""
		if i := y - a.y; i >= 0 && i < len(block) {
			line = pad + block[i]
		}
		out[y] = line + strings.Repeat(" ", max(w-ansi.StringWidth(line), 0))
	}
	return out
}

// idleRows is the spectrum area w x h while idle: the emblem (see
// idleArtFor), glitched while the idle glitch draws; blank where no
// emblem fits.
func (m Model) idleRows(w, h int) []string {
	a, ok := idleArtFor(w, h)
	if !ok {
		out := make([]string, h)
		for i := range out {
			out[i] = strings.Repeat(" ", max(w, 0))
		}
		return out
	}
	lines := a.lines(w, h)
	if m.idleActive() && m.idle.on(m.now()) {
		glitchArt(lines, a, mix(m.seed, saltIdle, m.idle.seq, m.frame))
	}
	return lines
}

// glitchArt draws an emblem glitch's look over lines, an area holding art
// a, from r (the seed, the glitch's salt and number, and the frame): 1..3
// block noise cells over the emblem's drawn cells (its mask's), then one
// emblem row torn 1 cell sideways, as wide as its line. The idle emblem
// tears its whole area row, the NOW PLAYING one its block only (see
// npemblem.go).
func glitchArt(lines []string, a idleArt, r uint64) {
	type cell struct{ x, y int }
	var drawn []cell
	for i, mask := range a.e.mask {
		for x, k := range []rune(mask) {
			if k != ' ' {
				drawn = append(drawn, cell{a.x + x, a.y + i})
			}
		}
	}
	n := uint64(len(drawn))
	for i := range 1 + r%3 {
		h := mix(r, 4, i)
		c := drawn[h%n]
		glyph := bootNoiseGlyphs[h>>32%uint64(len(bootNoiseGlyphs))]
		lines[c.y] = overlay(lines[c.y], c.x, noiseStyles[h>>48%uint64(len(noiseStyles))].Render(glyph))
	}
	y := a.y + int(r>>8%uint64(len(a.e.rows)))
	lines[y] = shift(lines[y], 1, r>>16%2 == 0)
}
