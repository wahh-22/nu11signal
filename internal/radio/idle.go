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
// With the signal effects active (see fxActive) the emblem glitches all
// the time, softly: every idle frame (idleFrameTick, from the clock, not
// the tick count) draws 0..idleNoiseMax block noise cells
// (bootNoiseGlyphs) over the emblem's drawn cells and, on one frame in
// idleTearOdds, tears one emblem row 1 cell sideways, inside its box (see
// idleArt.box); most frames change one or two cells and never draw
// letters over it. Behind the emblem, never in its box, a sparse field
// of the rain's glyphs (about idleFieldPerMille per mille of the cells,
// see idleFieldCell) flickers, each cell appearing, changing and going
// out on a life of its own. Everything comes from the seed and the idle
// frame, drawn inside the area, so nothing else moves.
//
// The tick runs at idleFrameTick while it does, landing on each frame
// (see idleFrameWait): about 6.7 wakeups a second instead of idleTick's
// one, fewer than playing's fastTick and slow enough to read as a quiet
// hum, not as motion. With the effects off the emblem and the field are
// static (frame 0) and the tick stays at idleTick.
const (
	idleFrameTick = 150 * time.Millisecond
	// idleNoiseMax is the most noise cells a frame draws over the emblem;
	// one frame in idleTearOdds also tears a row.
	idleNoiseMax = 2
	idleTearOdds = 6
	// idleFieldPerMille of the cells behind the emblem show a glyph at a
	// time; each cell rolls anew every idleFieldLifeMin..idleFieldLifeMax
	// frames, its own phase and life, so the field flickers unevenly.
	idleFieldPerMille = 60
	idleFieldLifeMin  = 4
	idleFieldLifeMax  = 12
)

// Salts keep the idle look's streams apart from the other effects'.
const (
	saltIdle uint64 = iota + 601
	saltIdleField
)

// idleFrame is the idle frame at now: the number of idleFrameTicks since
// the Unix epoch.
func idleFrame(now time.Time) uint64 {
	return uint64(now.UnixNano() / int64(idleFrameTick))
}

// idleFrameWait is the time from now to the next idle frame, in
// (0, idleFrameTick].
func idleFrameWait(now time.Time) time.Duration {
	q := int64(idleFrameTick)
	ns := now.UnixNano()
	return time.Duration((ns/q+1)*q - ns)
}

// idleFieldCell is cell x, y of the glyph field on idle frame f: its
// glyph (one of rainGlyphs) and ink, mostly inkDim, now and then inkMuted,
// rarely inkBody, when it shows one. Each cell keeps a glyph for its own
// life of frames, from its own phase, then rolls again.
func idleFieldCell(seed, f uint64, x, y int) (rune, uint8, bool) {
	c := mix(seed, saltIdleField, uint64(x), uint64(y))
	life := idleFieldLifeMin + c%(idleFieldLifeMax-idleFieldLifeMin+1)
	h := mix(c, (f+c>>16%life)/life)
	if h%1000 >= idleFieldPerMille {
		return 0, inkNone, false
	}
	k := inkDim
	switch h >> 40 % 20 {
	case 0:
		k = inkBody
	case 1, 2, 3, 4, 5:
		k = inkMuted
	}
	return rainGlyphs[h>>16%uint64(len(rainGlyphs))], k, true
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

// idleActive reports whether the idle look moves: the emblem shown and
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

// box is the columns [x0, x1) of an area w wide that the emblem's rows
// keep to themselves: the art and a cell on either side, the room a tear
// moves it into. The field never draws there.
func (a idleArt) box(w int) (x0, x1 int) {
	bw := a.e.width()
	if a.text {
		bw = a.e.blockWidth()
	}
	return max(a.x-1, 0), min(a.x+bw+1, w)
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

// idleRows is the spectrum area w x h while idle: the glyph field with
// the emblem over it, centered in its box (see idleArtFor, idleArt.box),
// glitched while the effects run; blank where no emblem fits. With the
// effects off it draws idle frame 0, the same every time.
func (m Model) idleRows(w, h int) []string {
	a, ok := idleArtFor(w, h)
	if !ok {
		out := make([]string, h)
		for i := range out {
			out[i] = strings.Repeat(" ", max(w, 0))
		}
		return out
	}
	live := m.idleActive()
	f := uint64(0)
	if live {
		f = idleFrame(m.now())
	}
	x0, x1 := a.box(w)
	c := newCanvas(w, h)
	for y := range h {
		inRows := y >= a.y && y < a.y+len(a.e.rows)
		for x := range w {
			if inRows && x >= x0 && x < x1 {
				continue
			}
			if g, k, ok := idleFieldCell(m.seed, f, x, y); ok {
				c.set(x, y, g, k)
			}
		}
	}
	lines := c.lines()
	segs := make([]string, len(a.e.rows))
	for i, l := range a.block() {
		seg := strings.Repeat(" ", a.x-x0) + l
		segs[i] = seg + strings.Repeat(" ", max(x1-x0-ansi.StringWidth(seg), 0))
	}
	if live {
		idleGlitchDraw(segs, a, x0, mix(m.seed, saltIdle, f))
	}
	for i, seg := range segs {
		lines[a.y+i] = overlay(lines[a.y+i], x0, seg)
	}
	return lines
}

// idleGlitchDraw draws a frame's soft glitch, from r, over segs, the
// emblem's rows in its box from column x0 of the area: on one frame in
// idleTearOdds one row torn 1 cell sideways, then 0..idleNoiseMax block
// noise cells over the emblem's drawn cells (its mask's).
func idleGlitchDraw(segs []string, a idleArt, x0 int, r uint64) {
	if r>>8%idleTearOdds == 0 {
		i := int(r >> 16 % uint64(len(segs)))
		segs[i] = shift(segs[i], 1, r>>24%2 == 0)
	}
	type cell struct{ x, i int }
	var drawn []cell
	for i, mask := range a.e.mask {
		for x, k := range []rune(mask) {
			if k != ' ' {
				drawn = append(drawn, cell{a.x - x0 + x, i})
			}
		}
	}
	n := uint64(len(drawn))
	for j := range r % (idleNoiseMax + 1) {
		h := mix(r, 4, j)
		c := drawn[h%n]
		glyph := bootNoiseGlyphs[h>>32%uint64(len(bootNoiseGlyphs))]
		segs[c.i] = overlay(segs[c.i], c.x, noiseStyles[h>>48%uint64(len(noiseStyles))].Render(glyph))
	}
}
