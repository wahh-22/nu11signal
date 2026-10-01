package radio

import (
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// The swap: when the spectrum area flips between the idle emblem and the
// rain (playback starts or stops, see isPlaying) it dissolves from one
// to the other over swapDur instead of cutting, both ways, so the emblem
// breaks up into the rain when music starts and the rain settles into
// the emblem when it stops.
//
// Every cell of the area has a seeded threshold in [0, 1) (see
// swapThreshold), lower toward the area's edges: at the swap's eased
// progress p a cell under p shows the rain, the others the emblem, so the
// emblem leaves from its edges inward and comes back from its middle
// outward. The cells within swapBand of p, the moving edge, glitch:
// Braille noise (emblemNoiseGlyphs) over the emblem's drawn cells,
// else, on one cell in two each frame, a rain glyph in a bright rain
// ink. Both
// pictures stay live underneath: the rain keeps stepping at its pace
// while playing and the emblem keeps its soft glitch (see idleRows).
//
// The tick runs at burstTick while a swap does, cut to its end, then
// back to its usual pace; the rain still steps only at the animation's
// (see animDue). A flip in the middle of a swap turns it around from
// where it is, no jump. With the effects off, during the boot, on the
// player's first state or where there is no spectrum area (the compact
// and tiny layouts) the area cuts as before. The swap draws inside the area only: no zones, no intro.
const (
	swapDur = 700 * time.Millisecond
	// swapBand is how close to the progress a cell's threshold glitches,
	// on either side.
	swapBand = 0.12
	// swapEdgeBias is the share of a threshold the cell's distance from
	// the area's edge sets; the rest is the seed's.
	swapEdgeBias = 0.5
)

// vizSwap is where the swap stands: toward the rain or the emblem, and
// its level from (0 the emblem, 1 the rain, linear) at.
type vizSwap struct {
	rain bool
	from float64
	at   time.Time
}

// level is the swap's linear level at now: from, moved toward its side
// by the time since at over swapDur, within [0, 1].
func (s vizSwap) level(now time.Time) float64 {
	d := float64(now.Sub(s.at)) / float64(swapDur)
	if s.rain {
		return min(s.from+d, 1)
	}
	return max(s.from-d, 0)
}

// end is when the swap reaches its side.
func (s vizSwap) end() time.Time {
	left := s.from
	if s.rain {
		left = 1 - s.from
	}
	return s.at.Add(time.Duration(left * float64(swapDur)))
}

// withSwap follows a flip of the area to the side isPlaying calls for,
// prev being the model before the msg: a swap from the level shown where
// it animates (see the doc above), else a cut. The player's first state
// cuts too: it tells what was already on, music does not start. It
// reports whether a swap started.
func (m Model) withSwap(prev Model) (Model, bool) {
	rain := m.isPlaying()
	if rain == m.swap.rain {
		return m, false
	}
	now := m.now()
	w, h := m.vizSize()
	if !m.fxActive() || m.boot || !prev.hasState || w <= 0 || h <= 0 {
		m.swap = vizSwap{rain: rain, from: side(rain), at: now}
		return m, false
	}
	m.swap = vizSwap{rain: rain, from: m.swap.level(now), at: now}
	return m, true
}

// side is a swap's level at rest: 1 for the rain, 0 for the emblem.
func side(rain bool) float64 {
	if rain {
		return 1
	}
	return 0
}

// swapLevel is the level the area shows now (0 the emblem, 1 the rain):
// the swap's, or its side's at rest with the effects off; isPlaying's
// side when the swap has not followed it (a model changed outside
// Update).
func (m Model) swapLevel() float64 {
	if m.swap.rain != m.isPlaying() {
		return side(m.isPlaying())
	}
	if !m.fxActive() {
		return side(m.swap.rain)
	}
	return m.swap.level(m.now())
}

// swapAnimating reports whether a swap is under way: the level shown
// short of its side, from the instant the swap starts.
func (m Model) swapAnimating() bool { return m.swapLevel() != side(m.swap.rain) }

// swapInterval cuts d to burstTick while a swap runs, and to its end.
func (m Model) swapInterval(d time.Duration) time.Duration {
	if !m.swapAnimating() {
		return d
	}
	return min(d, max(min(burstTick, m.swap.end().Sub(m.now())), minWake))
}

// swapThreshold is the level at which cell x, y of an area w x h turns
// to the rain, in [0, 1): swapEdgeBias of it the cell's closeness to the
// area's middle (an ellipse over the area), the rest from the seed.
func swapThreshold(seed uint64, x, y, w, h int) float64 {
	dx := (float64(x) + 0.5 - float64(w)/2) / (float64(w) / 2)
	dy := (float64(y) + 0.5 - float64(h)/2) / (float64(h) / 2)
	d := min(math.Sqrt((dx*dx+dy*dy)/2), 1)
	r := float64(mix(seed, saltSwap, uint64(x), uint64(y))>>11) / (1 << 53)
	return min(swapEdgeBias*(1-d)+(1-swapEdgeBias)*r, math.Nextafter(1, 0))
}

// spectrumRows is the spectrum area w x h: the emblem, the rain, or a
// swap between them.
func (m Model) spectrumRows(w, h int) []string {
	switch l := m.swapLevel(); {
	case l <= 0:
		return m.idleRows(w, h)
	case l >= 1:
		return m.rain.Render(w, h)
	default:
		return m.swapRows(w, h, l)
	}
}

// swapRows dissolves the emblem into the rain at level l (see the doc
// above): smoothstep-eased, then stretched over [-swapBand, 1+swapBand]
// so the edge enters and leaves the area whole.
func (m Model) swapRows(w, h int, l float64) []string {
	p := l*l*(3-2*l)*(1+2*swapBand) - swapBand
	emblem, rain := m.idleRows(w, h), m.rain.Render(w, h)
	frame := uint64(m.now().UnixNano() / int64(burstTick))
	out := make([]string, h)
	var b strings.Builder
	for y := range h {
		b.Reset()
		plain := []rune(ansi.Strip(emblem[y]))
		// Runs of cells from the same picture are cut whole.
		src, run := "", 0
		flush := func(x int) {
			if x > run && src != "" {
				b.WriteString(ansi.ResetStyle + ansi.Cut(src, run, x))
			}
			run = x
		}
		for x := range w {
			th := swapThreshold(m.seed, x, y, w, h)
			if math.Abs(th-p) < swapBand {
				under := ' '
				if x < len(plain) {
					under = plain[x]
				}
				if g, ok := m.swapGlitch(frame, x, y, under); ok {
					flush(x)
					b.WriteString(ansi.ResetStyle + g)
					run, src = x+1, ""
					continue
				}
			}
			want := emblem[y]
			if th < p {
				want = rain[y]
			}
			if want != src {
				flush(x)
				src = want
			}
		}
		flush(w)
		out[y] = b.String() + ansi.ResetStyle
	}
	return out
}

// swapGlitch is the glyph cell x, y of the moving edge draws on frame
// over under, the emblem's cell there: Braille noise over a drawn emblem
// cell, else a rain glyph in a bright rain ink on one frame in two;
// false when it draws its picture.
func (m Model) swapGlitch(frame uint64, x, y int, under rune) (string, bool) {
	h := mix(m.seed, saltSwap, frame, uint64(x), uint64(y))
	if under != ' ' {
		g := emblemNoise(h>>8, under)
		return noiseStyles[h>>24%uint64(len(noiseStyles))].Render(g), true
	}
	if h%2 == 0 {
		return "", false
	}
	k := rainRamp[h>>8%3] // tip, bright or body
	g := rainGlyphs[h>>16%uint64(len(rainGlyphs))]
	return vizInks[k].pre + string(g) + vizInks[k].post, true
}
