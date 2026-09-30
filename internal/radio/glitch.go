package radio

import (
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Signal effects: the screen now and then seems to lose the signal. Every
// burstGapMin..burstGapMax a sharp burst of burstMin..burstMax tears a few
// rows sideways, corrupts a handful of cells and may run a static bar;
// one burst in noSignalOdds also flashes NO SIGNAL, framed in red static.
//
// Everything is drawn over the finished frame (see Model.decorate), cell
// for cell, so no line changes width and the clickable zones, laid out
// from the undecorated frame, stay where they are. Timings and glyphs come
// from the seed, the injected clock and the frame counter: tests replay
// them exactly. No timer is added: the animation tick sleeps until the
// next burst is due, and runs fast only while one animates (see
// effects.interval).
const (
	burstGapMin  = 10 * time.Second
	burstGapMax  = 22 * time.Second
	burstMin     = 600 * time.Millisecond
	burstMax     = 1000 * time.Millisecond
	noSignalOdds = 4
	// burstTick is the frame time during a burst (about 15 fps, playing
	// or not). minWake keeps a late tick from spinning.
	burstTick = 66 * time.Millisecond
	minWake   = 10 * time.Millisecond
)

// Salts keep the pseudo-random streams of the effects apart.
const (
	saltBurstGap uint64 = iota + 101
	saltBurstLen
	saltNoSignal
	// 104 and 105 belonged to the retired status line alerts; skipping
	// them keeps the burst stream, and so every frame, as it was (107
	// and 108 were the retired text wave's).
	saltBurst uint64 = iota + 103
)

// effects is the schedule of the signal effects. It is a value type:
// advance returns a new schedule.
type effects struct {
	// on is the user's setting: Options.Effects, flipped by keyEffects.
	on bool

	// burstSeq numbers the bursts; the latest one runs from burstStart
	// to burstEnd, flashing NO SIGNAL when noSignal. nextBurst is when
	// the next starts, zero until the effects are active.
	burstSeq             uint64
	nextBurst            time.Time
	burstStart, burstEnd time.Time
	noSignal             bool
}

// advance moves the schedule to now. While the effects are not active
// (off, typing or the tiny layout) nothing runs and nothing is pending,
// so becoming active again never fires what was missed at once.
func (e effects) advance(now time.Time, seed uint64, active bool) effects {
	if !active {
		e.nextBurst, e.burstEnd = time.Time{}, time.Time{}
		return e
	}
	if e.nextBurst.IsZero() {
		e.nextBurst = now.Add(span(mix(seed, saltBurstGap, e.burstSeq), burstGapMin, burstGapMax))
	}
	if !now.Before(e.nextBurst) {
		e.burstSeq++
		e.burstStart = now
		e.burstEnd = now.Add(span(mix(seed, saltBurstLen, e.burstSeq), burstMin, burstMax))
		e.noSignal = mix(seed, saltNoSignal, e.burstSeq)%noSignalOdds == 0
		e.nextBurst = e.burstEnd.Add(span(mix(seed, saltBurstGap, e.burstSeq), burstGapMin, burstGapMax))
	}
	return e
}

func (e effects) bursting(now time.Time) bool {
	return !now.Before(e.burstStart) && now.Before(e.burstEnd)
}

// interval is the time to the next frame, d without the effects:
// burstTick during a burst, cut short at its end; otherwise d cut short
// so that the next burst starts on time.
func (e effects) interval(now time.Time, d time.Duration) time.Duration {
	if e.bursting(now) {
		return max(min(burstTick, e.burstEnd.Sub(now)), minWake)
	}
	if !e.nextBurst.IsZero() {
		d = min(d, max(e.nextBurst.Sub(now), minWake))
	}
	return d
}

// span maps a hash to a duration in [lo, hi).
func span(h uint64, lo, hi time.Duration) time.Duration {
	return lo + time.Duration(unit(h)*float64(hi-lo))
}

// fxActive reports whether the effects run now: on, not while the SEARCH
// input or the NEW PLAYLIST name takes the keys, and not on the tiny
// layout or the auth error screen.
func (m Model) fxActive() bool {
	return m.fx.on && !m.typing() && m.auth != authFailed &&
		m.width >= tinyMinWidth && m.height >= tinyMinHeight
}

// typing reports whether a text input takes the keys.
func (m Model) typing() bool {
	return m.editor.mode == editName || (m.top().kind == viewSearch && m.input.Focused())
}

// toggleEffects flips the signal effects and says so.
func (m Model) toggleEffects() Model {
	m.fx.on = !m.fx.on
	if m.fx.on {
		m.setStatus("SIGNAL FX ON")
	} else {
		m.setStatus("SIGNAL FX OFF // CALM")
	}
	return m
}

// decorate draws the active effects over the finished frame, keeping every
// line's width.
func (m Model) decorate(lines []string) []string {
	now := m.now()
	out := append([]string(nil), lines...)
	frame := out
	if m.status != "" && len(out) > 2 {
		// A real status stays readable: bursts spare the status and hint
		// lines while one is showing.
		frame = out[:len(out)-2]
	}
	if m.fx.bursting(now) {
		m.burst(frame)
	}
	return out
}

// runeWidth is the cell width of r, printable ASCII without a lookup.
func runeWidth(r rune) int {
	if r >= ' ' && r < 0x7F {
		return 1
	}
	return ansi.StringWidth(string(r))
}

// textRune reports whether r is text: printable, not a space, not a box
// drawing border or a block shade, and not braille (the oscilloscope's
// trace, see scopeViz).
func textRune(r rune) bool {
	return unicode.IsPrint(r) && !unicode.IsSpace(r) && (r < 0x2500 || r > 0x259F) && (r < 0x2800 || r > 0x28FF)
}

// setCells replaces the characters of the one cell wide cells of line at
// the keys of cells with their one cell wide runes, keeping every escape
// sequence where it is: each cell keeps the style it had.
func setCells(line string, cells map[int]rune) string {
	var b strings.Builder
	b.Grow(len(line))
	var state byte
	x := 0
	for rest := line; len(rest) > 0; {
		seq, w, n, next := ansi.DecodeSequence(rest, state, nil)
		if r, ok := cells[x]; ok && w == 1 {
			b.WriteRune(r)
		} else {
			b.WriteString(seq)
		}
		x += w
		rest, state = rest[n:], next
	}
	return b.String()
}

// noiseGlyphs replace cells during a burst.
var noiseGlyphs = []string{"░", "▒", "▓", "█", "▚", "▞", "0", "1", "3", "7", "A", "C", "E", "F"}

var noiseStyles = []lipgloss.Style{stRed, stCyan, stYellow, stFrameDim}

// tear is row y torn k cells sideways.
type tear struct {
	y, k  int
	right bool
}

// burstLook is what a burst draws on one frame: torn rows, a static bar
// on row bar (-1 for none) with its own noise barSeed, one hash per noise
// cell (its row, column, glyph and style), and whether the NO SIGNAL sign
// flashes.
type burstLook struct {
	tears   []tear
	bar     int
	barSeed uint64
	noise   []uint64
	sign    bool
}

// look is what the latest burst draws on animation frame frame of a frame
// of n lines: 2..4 rows torn 1..3 cells sideways, a static bar on half
// the frames, 6..14 corrupted cells (three times as many with NO SIGNAL)
// and, on a NO SIGNAL burst, the sign. It changes every frame.
func (e effects) look(seed, frame uint64, n int) burstLook {
	l := burstLook{bar: -1, sign: e.noSignal}
	if n <= 0 {
		return l
	}
	r := mix(seed, saltBurst, e.burstSeq, frame)
	rows := uint64(n)
	for i := range 2 + r%3 {
		h := mix(r, 1, i)
		l.tears = append(l.tears, tear{y: int(h % rows), k: int(h>>8%3) + 1, right: h>>16%2 == 0})
	}
	if r>>8%2 == 0 {
		l.bar, l.barSeed = int(mix(r, 2)%rows), mix(r, 3)
	}
	noise := 6 + r>>16%9
	if e.noSignal {
		noise *= 3
	}
	for i := range noise {
		l.noise = append(l.noise, mix(r, 4, i))
	}
	return l
}

// burst draws the look of the live burst on this frame over lines (see
// look), the NO SIGNAL sign on top.
func (m Model) burst(lines []string) {
	if len(lines) == 0 {
		return
	}
	l := m.fx.look(m.seed, m.frame, len(lines))
	for _, tr := range l.tears {
		lines[tr.y] = shift(lines[tr.y], tr.k, tr.right)
	}
	if l.bar >= 0 {
		lines[l.bar] = staticBar(ansi.StringWidth(lines[l.bar]), l.barSeed)
	}
	for _, h := range l.noise {
		y := h % uint64(len(lines))
		if w := ansi.StringWidth(lines[y]); w > 0 {
			glyph := noiseGlyphs[h>>32%uint64(len(noiseGlyphs))]
			lines[y] = overlay(lines[y], int(h>>8%uint64(w)), noiseStyles[h>>48%uint64(len(noiseStyles))].Render(glyph))
		}
	}
	if l.sign {
		m.noSignalFlash(lines, mix(m.seed, saltBurst, m.fx.burstSeq, m.frame))
	}
}

// noSignalFlash draws the NO SIGNAL sign in bold red, framed in red
// static, in the middle of the frame:
//
//	▓▒░▒▓░▒▓▒░▓▒░▒▓░▒▓▒░▓▒░▒▓
//	▒▓   N O   S I G N A L  ▓▒
//	▓▒░▒▓░▒▓▒░▓▒░▒▓░▒▓▒░▓▒░▒▓
func (m Model) noSignalFlash(lines []string, r uint64) {
	sign := "  " + spaced("NO SIGNAL") + "  "
	signW := ansi.StringWidth(sign) + 4
	x := max((m.width-signW)/2, 0)
	y := max(len(lines)/2-1, 0)
	rows := []string{
		stRed.Render(static(signW, mix(r, 5))),
		stRed.Render("▒▓") + stRedBold.Render(sign) + stRed.Render("▓▒"),
		stRed.Render(static(signW, mix(r, 6))),
	}
	for i, row := range rows {
		if y+i < len(lines) {
			lines[y+i] = overlay(lines[y+i], x, row)
		}
	}
}

// static is w cells of shaded noise.
func static(w int, h uint64) string {
	shades := []string{"░", "▒", "▓"}
	var b strings.Builder
	for i := range w {
		b.WriteString(shades[mix(h, uint64(i))%3])
	}
	return b.String()
}

func staticBar(w int, h uint64) string { return stFrameDim.Render(static(w, h)) }

// overlay writes the styled s over the cells of line from x, clipped to
// the line: the result is as wide as line, else line is returned as it
// was (s would split a wide character).
func overlay(line string, x int, s string) string {
	lw := ansi.StringWidth(line)
	if x < 0 || x >= lw {
		return line
	}
	sw := ansi.StringWidth(s)
	if x+sw > lw {
		s = ansi.Truncate(s, lw-x, "")
		sw = ansi.StringWidth(s)
	}
	out := ansi.Cut(line, 0, x) + ansi.ResetStyle + s + ansi.Cut(line, x+sw, lw)
	if ansi.StringWidth(out) != lw {
		return line
	}
	return out
}

// shift tears line k cells to the right (or left), keeping its width.
func shift(line string, k int, right bool) string {
	lw := ansi.StringWidth(line)
	if lw <= k {
		return line
	}
	pad := strings.Repeat(" ", k)
	var out string
	if right {
		out = pad + ansi.ResetStyle + ansi.Truncate(line, lw-k, "")
	} else {
		out = ansi.Cut(line, k, lw) + ansi.ResetStyle + pad
	}
	if ansi.StringWidth(out) != lw {
		return line
	}
	return out
}
