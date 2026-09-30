package radio

import (
	"cmp"
	"math"
	"slices"
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
// Midway between two bursts a slow text wave runs: for waveMin..waveMax
// about waveShare of the letters on screen sweep into scrambled glyphs,
// like the title on a song change, each cell keeping its own style, hold,
// and type themselves back in one by one; the words stay partly readable.
//
// Everything is drawn over the finished frame (see Model.decorate), cell
// for cell, so no line changes width and the clickable zones, laid out
// from the undecorated frame, stay where they are. Timings and glyphs come
// from the seed, the injected clock and the frame counter: tests replay
// them exactly. No timer is added: the animation tick sleeps until the
// next effect is due, and runs fast only while one animates (see
// effects.interval).
const (
	burstGapMin  = 20 * time.Second
	burstGapMax  = 45 * time.Second
	burstMin     = 600 * time.Millisecond
	burstMax     = 1000 * time.Millisecond
	noSignalOdds = 4
	// A text wave scrambles waveShare of the letters for waveMin..waveMax.
	// Each word starts at its own moment before waveRamp of the wave,
	// sooner on the left so the wave sweeps across, holds, and resolves
	// at its own moment from waveResolve on, most early, a few lingering.
	// A word's letters turn one after another, left to right, over
	// waveLetters of the wave; a scrambled cell shows a new glyph every
	// waveGlyph, each cell at its own phase.
	waveMin     = 1000 * time.Millisecond
	waveMax     = 2000 * time.Millisecond
	waveShare   = 0.40
	waveRamp    = 0.35
	waveResolve = 0.5
	waveLetters = 0.1
	waveGlyph   = 160 * time.Millisecond
	// burstTick is the frame time during a burst (about 15 fps, playing
	// or not), waveTick during a text wave (10 fps; while a song plays it
	// rides its 10 fps tick). minWake keeps a late tick from spinning.
	burstTick = 66 * time.Millisecond
	waveTick  = 100 * time.Millisecond
	minWake   = 10 * time.Millisecond
)

// Salts keep the pseudo-random streams of the effects apart.
const (
	saltBurstGap uint64 = iota + 101
	saltBurstLen
	saltNoSignal
	// 104 and 105 belonged to the retired status line alerts; skipping
	// them keeps the streams below, and so every frame, as they were.
	saltBurst uint64 = iota + 103
	saltWaveLen
	saltWave
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
	// waveSeq numbers the text waves; the latest runs from waveStart to
	// waveEnd. nextWave, midway between the last burst (or the start)
	// and the next, is zero once that wave ran.
	waveSeq            uint64
	nextWave           time.Time
	waveStart, waveEnd time.Time
}

// advance moves the schedule to now. While the effects are not active
// (off, typing or the tiny layout) nothing runs and nothing is pending,
// so becoming active again never fires what was missed at once.
func (e effects) advance(now time.Time, seed uint64, active bool) effects {
	if !active {
		e.nextBurst, e.burstEnd = time.Time{}, time.Time{}
		e.nextWave, e.waveEnd = time.Time{}, time.Time{}
		return e
	}
	if e.nextBurst.IsZero() {
		e.nextBurst = now.Add(span(mix(seed, saltBurstGap, e.burstSeq), burstGapMin, burstGapMax))
		e.nextWave = midway(now, e.nextBurst)
	}
	switch {
	case !now.Before(e.nextBurst):
		// A wave still pending (a very late tick) gives way to the burst.
		e.burstSeq++
		e.burstStart = now
		e.burstEnd = now.Add(span(mix(seed, saltBurstLen, e.burstSeq), burstMin, burstMax))
		e.noSignal = mix(seed, saltNoSignal, e.burstSeq)%noSignalOdds == 0
		e.nextBurst = e.burstEnd.Add(span(mix(seed, saltBurstGap, e.burstSeq), burstGapMin, burstGapMax))
		e.nextWave = midway(e.burstEnd, e.nextBurst)
	case !e.nextWave.IsZero() && !now.Before(e.nextWave):
		e.waveSeq++
		e.waveStart = now
		e.waveEnd = now.Add(span(mix(seed, saltWaveLen, e.waveSeq), waveMin, waveMax))
		e.nextWave = time.Time{}
	}
	return e
}

func (e effects) bursting(now time.Time) bool {
	return !now.Before(e.burstStart) && now.Before(e.burstEnd)
}

func (e effects) waving(now time.Time) bool {
	return !now.Before(e.waveStart) && now.Before(e.waveEnd)
}

// interval is the time to the next frame, d without the effects:
// burstTick during a burst; during a text wave d when it is already as
// fast as waveTick (the playing tick), else waveTick; either cut short at
// the effect's end. Otherwise d cut short so that the next wave or burst
// starts on time.
func (e effects) interval(now time.Time, d time.Duration) time.Duration {
	if e.bursting(now) {
		return max(min(burstTick, e.burstEnd.Sub(now)), minWake)
	}
	if e.waving(now) {
		if d <= waveTick {
			return d
		}
		return max(min(waveTick, e.waveEnd.Sub(now)), minWake)
	}
	for _, next := range []time.Time{e.nextWave, e.nextBurst} {
		if !next.IsZero() {
			d = min(d, max(next.Sub(now), minWake))
		}
	}
	return d
}

// midway is the time halfway from a to b.
func midway(a, b time.Time) time.Time { return a.Add(b.Sub(a) / 2) }

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
	spared := -1
	frame := out
	if m.status != "" && len(out) > 2 {
		// A real status stays readable: text waves spare the status line
		// and bursts the status and hint lines while one is showing.
		spared = len(out) - 2
		frame = out[:len(out)-2]
	}
	if m.fx.waving(now) {
		m.textWave(out, spared, now)
	}
	if m.fx.bursting(now) {
		m.burst(frame)
	}
	return out
}

// textRun is a word on line y: the cells of xs, next to each other.
type textRun struct {
	y  int
	xs []int
}

// textWords finds the words in lines, but on line spared: the runs of
// text cells with nothing between them. A text cell is one printable, one
// cell wide character that is not a space, a border or a shade; a wide or
// combined character ends a word, so it is never split. A line whose
// characters do not add up to its width is left out.
func textWords(lines []string, spared int) []textRun {
	var words []textRun
	for y, line := range lines {
		if y == spared {
			continue
		}
		var found []textRun
		cur := textRun{y: y}
		flush := func() {
			if len(cur.xs) > 0 {
				found = append(found, cur)
			}
			cur = textRun{y: y}
		}
		rs := []rune(ansi.Strip(line))
		x := 0
		for i, r := range rs {
			w := runeWidth(r)
			combined := i+1 < len(rs) && runeWidth(rs[i+1]) == 0
			switch {
			case w == 0:
			case w == 1 && textRune(r) && !combined:
				cur.xs = append(cur.xs, x)
			default:
				flush()
			}
			x += w
		}
		flush()
		if x == ansi.StringWidth(line) {
			words = append(words, found...)
		}
	}
	return words
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

// textWave scrambles the letters of the live text wave in lines, but on
// line spared. All the text cells are ranked by a hash of the wave and
// their place and the first waveShare of them scramble, so words stay
// partly readable. A chosen cell turns in its word's window (see
// waveWindow), the word's letters one after another; it holds a noise
// glyph or a letter, a new one every waveGlyph. Only the characters of the
// chosen cells change: every cell keeps its style, and the others their
// text too.
func (m Model) textWave(lines []string, spared int, now time.Time) {
	words := textWords(lines, spared)
	if len(words) == 0 {
		return
	}
	type pick struct {
		word, letter int
		h            uint64
	}
	var picks []pick
	for i, w := range words {
		for j, x := range w.xs {
			picks = append(picks, pick{i, j, mix(m.seed, saltWave, m.fx.waveSeq, uint64(w.y), uint64(x), 1)})
		}
	}
	slices.SortFunc(picks, func(a, b pick) int { return cmp.Compare(a.h, b.h) })
	chosen := make([][]bool, len(words))
	for i, w := range words {
		chosen[i] = make([]bool, len(w.xs))
	}
	for _, pk := range picks[:int(math.Ceil(waveShare*float64(len(picks))))] {
		chosen[pk.word][pk.letter] = true
	}

	elapsed := now.Sub(m.fx.waveStart)
	p := float64(elapsed) / float64(m.fx.waveEnd.Sub(m.fx.waveStart))
	width := float64(max(m.width, 1))
	repl := map[int]map[int]rune{}
	for i, w := range words {
		h := mix(m.seed, saltWave, m.fx.waveSeq, uint64(w.y), uint64(w.xs[0]))
		on, off := waveWindow(h, min(float64(w.xs[0])/width, 1))
		n := float64(len(w.xs))
		for j, x := range w.xs {
			// The letters turn in, and back, one after another.
			lag := waveLetters * float64(j) / n
			if !chosen[i][j] || p < on+lag || p >= off+lag {
				continue
			}
			// Each cell changes glyph every waveGlyph at its own phase,
			// so the flicker ripples instead of jumping all at once.
			phase := time.Duration(mix(h, 2, uint64(j)) % uint64(waveGlyph))
			c := mix(h, uint64((elapsed+phase)/waveGlyph), uint64(j))
			glyph := glitchGlyphs[c>>16%uint64(len(glitchGlyphs))]
			if c>>8%3 == 0 {
				glyph = rune('A' + c>>24%26)
			}
			if repl[w.y] == nil {
				repl[w.y] = map[int]rune{}
			}
			repl[w.y][x] = glyph
		}
	}
	for y, cells := range repl {
		lines[y] = setCells(lines[y], cells)
	}
}

// waveWindow is when, as shares of the wave, the first letter of a word
// with hash h starting at column share col scrambles and resolves. The
// start falls before waveRamp (less the letters' lag), mostly by column
// so the wave sweeps from the left, a few words at once. The resolve falls
// in [waveResolve, 1) (less the lag), eased: a power of the draw bunches
// most words early in the window and leaves a few to linger.
func waveWindow(h uint64, col float64) (on, off float64) {
	sweep := min(max(0.75*col+0.35*unit(mix(h, 3))-0.1, 0), 1)
	on = (waveRamp - waveLetters) * sweep
	u := 0.7*unit(mix(h, 1)) + 0.3*col
	off = waveResolve + (1-waveResolve-waveLetters)*math.Pow(u, 1.6)
	return on, off
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
