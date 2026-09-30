package radio

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Signal effects: the screen now and then seems to lose the signal. Every
// burstGapMin..burstGapMax a burst of burstMin..burstMax tears a few rows
// sideways, corrupts a few cells and may run a static bar; one burst in
// noSignalOdds also flashes NO SIGNAL. Midway between two bursts a text
// wave runs: for waveMin..waveMax about waveShare of the words on screen
// scramble at once, like the title on a song change, each cell keeping its
// own style, and resolve back word by word. Night City alerts take the
// empty status line now and then.
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
	burstMin     = 200 * time.Millisecond
	burstMax     = 600 * time.Millisecond
	noSignalOdds = 4
	alertGapMin  = 30 * time.Second
	alertGapMax  = 60 * time.Second
	alertShow    = 4 * time.Second
	alertBlink   = 500 * time.Millisecond
	// A text wave scrambles waveShare of the words for waveMin..waveMax;
	// all of them hold for waveHold of its time, then each resolves at
	// its own moment, the ones on the left somewhat sooner.
	waveMin   = 600 * time.Millisecond
	waveMax   = 1000 * time.Millisecond
	waveShare = 0.75
	waveHold  = 0.3
	// burstTick is the frame time during a burst (about 15 fps), waveTick
	// during a text wave (10 fps); minWake keeps a late tick from
	// spinning.
	burstTick = 66 * time.Millisecond
	waveTick  = 100 * time.Millisecond
	minWake   = 10 * time.Millisecond
)

// Salts keep the pseudo-random streams of the effects apart.
const (
	saltBurstGap uint64 = iota + 101
	saltBurstLen
	saltNoSignal
	saltAlertGap
	saltAlert
	saltBurst
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
	alertSeq             uint64
	nextAlert            time.Time
	alertStart, alertEnd time.Time
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
		e.nextBurst, e.burstEnd, e.nextAlert, e.alertEnd = time.Time{}, time.Time{}, time.Time{}, time.Time{}
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
	if e.nextAlert.IsZero() {
		e.nextAlert = now.Add(span(mix(seed, saltAlertGap, e.alertSeq), alertGapMin, alertGapMax))
	}
	if !now.Before(e.nextAlert) {
		e.alertSeq++
		e.alertStart, e.alertEnd = now, now.Add(alertShow)
		e.nextAlert = e.alertEnd.Add(span(mix(seed, saltAlertGap, e.alertSeq), alertGapMin, alertGapMax))
	}
	return e
}

func (e effects) bursting(now time.Time) bool {
	return !now.Before(e.burstStart) && now.Before(e.burstEnd)
}

func (e effects) alerting(now time.Time) bool {
	return !now.Before(e.alertStart) && now.Before(e.alertEnd)
}

func (e effects) waving(now time.Time) bool {
	return !now.Before(e.waveStart) && now.Before(e.waveEnd)
}

// interval is the time to the next frame, d without the effects:
// burstTick during a burst; during a text wave d when it is already as
// fast as waveTick (the playing tick), else waveTick, cut short at the
// wave's end; and otherwise d cut short so that the ▲ of an alert blinks
// and the next wave or burst starts on time.
func (e effects) interval(now time.Time, d time.Duration) time.Duration {
	if e.bursting(now) {
		return burstTick
	}
	if e.waving(now) {
		if d <= waveTick {
			return d
		}
		return max(min(waveTick, e.waveEnd.Sub(now)), minWake)
	}
	if e.alerting(now) {
		d = min(d, alertBlink)
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

// alert is one status line alert; a %d in text takes a number in lo..hi.
type alert struct {
	text   string
	lo, hi int
}

var alerts = []alert{
	{text: "SIGNAL DEGRADED // RETUNING"},
	{text: "ICE TRACE DETECTED"},
	{text: "PACKET LOSS %d%%", lo: 12, hi: 79},
	{text: "NETWATCH PING"},
	{text: "CARRIER DRIFT +0.%d MHZ", lo: 1, hi: 9},
	{text: "BLACKWALL NOISE // FILTERING"},
	{text: "DAEMON SWEEP // PORT %d", lo: 1024, hi: 9999},
	{text: "UPLINK JITTER %dMS", lo: 40, hi: 400},
}

// alertText is the text of alert number seq.
func alertText(seq, seed uint64) string {
	h := mix(seed, saltAlert, seq)
	a := alerts[h%uint64(len(alerts))]
	if a.hi == 0 {
		return a.text
	}
	return fmt.Sprintf(a.text, a.lo+int(mix(h, 1)%uint64(a.hi-a.lo+1)))
}

// alertLine is the status line while an alert shows, its ▲ blinking.
func (m Model) alertLine(w int) (string, bool) {
	now := m.now()
	if !m.fxActive() || !m.fx.alerting(now) {
		return "", false
	}
	mark := stYellowB.Render("▲")
	if now.Sub(m.fx.alertStart)/alertBlink%2 == 1 {
		mark = " "
	}
	return fit(mark+" "+stRed.Render(alertText(m.fx.alertSeq, m.seed)), w), true
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
// drawing border or a block shade.
func textRune(r rune) bool {
	return unicode.IsPrint(r) && !unicode.IsSpace(r) && (r < 0x2500 || r > 0x259F)
}

// textWave scrambles the words of the live text wave in lines, but on
// line spared. The words are ranked by a hash of the wave and their place
// and the first waveShare of them scramble: their cells hold noise glyphs
// or letters, new ones every waveTick, until the word's own moment to
// resolve, after waveHold of the wave and before its end, somewhat sooner
// on the left. Only the characters of the scrambled cells change: every
// cell keeps its style, and the others their text too.
func (m Model) textWave(lines []string, spared int, now time.Time) {
	words := textWords(lines, spared)
	if len(words) == 0 {
		return
	}
	type pick struct {
		w textRun
		h uint64
	}
	picks := make([]pick, len(words))
	for i, w := range words {
		picks[i] = pick{w, mix(m.seed, saltWave, m.fx.waveSeq, uint64(w.y), uint64(w.xs[0]))}
	}
	slices.SortFunc(picks, func(a, b pick) int { return cmp.Compare(a.h, b.h) })
	picks = picks[:int(math.Ceil(waveShare*float64(len(picks))))]

	d := m.fx.waveEnd.Sub(m.fx.waveStart)
	p := float64(now.Sub(m.fx.waveStart)) / float64(d)
	bucket := uint64(now.Sub(m.fx.waveStart) / waveTick)
	width := float64(max(m.width, 1))
	repl := map[int]map[int]rune{}
	for _, pk := range picks {
		// Resolve somewhere in [waveHold, 1): 60% chance, 40% the column.
		at := 0.6*unit(mix(pk.h, 1)) + 0.4*min(float64(pk.w.xs[0])/width, 1)
		if p >= waveHold+(1-waveHold)*at {
			continue
		}
		for j, x := range pk.w.xs {
			c := mix(pk.h, bucket, uint64(j))
			glyph := glitchGlyphs[c>>16%uint64(len(glitchGlyphs))]
			if c>>8%3 == 0 {
				glyph = rune('A' + c>>24%26)
			}
			if repl[pk.w.y] == nil {
				repl[pk.w.y] = map[int]rune{}
			}
			repl[pk.w.y][x] = glyph
		}
	}
	for y, cells := range repl {
		lines[y] = setCells(lines[y], cells)
	}
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

// burst tears a few rows sideways by 1..3 cells, corrupts a handful of
// cells, may run a static bar across one row and, on a NO SIGNAL burst,
// flashes the sign in the middle. It changes every frame.
func (m Model) burst(lines []string) {
	if len(lines) == 0 {
		return
	}
	r := mix(m.seed, saltBurst, m.fx.burstSeq, m.frame)
	n := uint64(len(lines))
	for i := range 2 + r%3 {
		h := mix(r, 1, i)
		y := h % n
		lines[y] = shift(lines[y], int(h>>8%3)+1, h>>16%2 == 0)
	}
	if r>>8%2 == 0 {
		y := mix(r, 2) % n
		lines[y] = staticBar(ansi.StringWidth(lines[y]), mix(r, 3))
	}
	noise := 6 + r>>16%9
	if m.fx.noSignal {
		noise *= 3
	}
	for i := range noise {
		h := mix(r, 4, i)
		y := h % n
		if w := ansi.StringWidth(lines[y]); w > 0 {
			glyph := noiseGlyphs[h>>32%uint64(len(noiseGlyphs))]
			lines[y] = overlay(lines[y], int(h>>8%uint64(w)), noiseStyles[h>>48%uint64(len(noiseStyles))].Render(glyph))
		}
	}
	if m.fx.noSignal {
		m.noSignalFlash(lines, r)
	}
}

// noSignalFlash draws the NO SIGNAL sign, framed in static, in the middle
// of the frame:
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
