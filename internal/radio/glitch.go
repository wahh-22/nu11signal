package radio

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Signal effects: the screen now and then seems to lose the signal. Every
// burstGapMin..burstGapMax a burst of burstMin..burstMax tears a few rows
// sideways, corrupts a few cells and may run a static bar; one burst in
// noSignalOdds also flashes NO SIGNAL. Between bursts, the text on screen
// keeps glitching like the title on a song change: every
// microGapMin..microGapMax one to microMaxN spans of existing text, on any
// line, scramble and resolve back over microMin..microMax. Night City
// alerts take the empty status line now and then.
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
	// A micro-glitch scrambles microSpanMin..microSpanMax text cells;
	// microHold is the share of its time before its cells start to
	// resolve, left to right.
	microGapMin  = 500 * time.Millisecond
	microGapMax  = 2 * time.Second
	microMin     = 150 * time.Millisecond
	microMax     = 300 * time.Millisecond
	microMaxN    = 3
	microSpanMin = 3
	microSpanMax = 12
	microHold    = 0.3
	// burstTick is the frame time during a burst (about 15 fps), microTick
	// while a micro-glitch resolves; minWake keeps a late tick from
	// spinning.
	burstTick = 66 * time.Millisecond
	microTick = 100 * time.Millisecond
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
	saltMicroGap
	saltMicroN
	saltMicroLen
	saltMicro
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
	// microSeq numbers the micro-glitches; the latest runs microN spans
	// from microStart until microEnd, the end of its longest one.
	microSeq             uint64
	microN               int
	nextMicro            time.Time
	microStart, microEnd time.Time
}

// advance moves the schedule to now. While the effects are not active
// (off, typing or the tiny layout) nothing runs and nothing is pending,
// so becoming active again never fires what was missed at once.
func (e effects) advance(now time.Time, seed uint64, active bool) effects {
	if !active {
		e.nextBurst, e.burstEnd, e.nextAlert, e.alertEnd = time.Time{}, time.Time{}, time.Time{}, time.Time{}
		e.nextMicro, e.microEnd = time.Time{}, time.Time{}
		return e
	}
	if e.nextMicro.IsZero() {
		e.nextMicro = now.Add(span(mix(seed, saltMicroGap, e.microSeq), microGapMin, microGapMax))
	}
	if !now.Before(e.nextMicro) {
		e.microSeq++
		e.microStart, e.microEnd = now, now
		e.microN = microCount(mix(seed, saltMicroN, e.microSeq))
		for i := range e.microN {
			if end := now.Add(microDuration(seed, e.microSeq, i)); end.After(e.microEnd) {
				e.microEnd = end
			}
		}
		e.nextMicro = now.Add(span(mix(seed, saltMicroGap, e.microSeq), microGapMin, microGapMax))
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

func (e effects) micro(now time.Time) bool {
	return !now.Before(e.microStart) && now.Before(e.microEnd)
}

// interval is the time to the next frame, d without the effects:
// burstTick during a burst; d itself when it is no slower than microTick;
// else microTick while a micro-glitch resolves (its last frame on its
// end), and d cut short so that the ▲ of an alert blinks and the next
// burst or micro-glitch starts on time.
func (e effects) interval(now time.Time, d time.Duration) time.Duration {
	if e.bursting(now) {
		return burstTick
	}
	if d <= microTick {
		// Already as fast as a micro-glitch needs: it rides the frames
		// that run anyway, a few milliseconds late at most.
		return d
	}
	if e.micro(now) {
		return max(min(microTick, e.microEnd.Sub(now)), minWake)
	}
	if e.alerting(now) {
		d = min(d, alertBlink)
	}
	for _, next := range []time.Time{e.nextMicro, e.nextBurst} {
		if !next.IsZero() {
			d = min(d, max(next.Sub(now), minWake))
		}
	}
	return d
}

// microCount is how many spans a micro-glitch scrambles at once: mostly
// one, now and then two or three.
func microCount(h uint64) int {
	switch h % 8 {
	case 0:
		return 3
	case 1, 2:
		return 2
	}
	return 1
}

// microDuration is how long span i of micro-glitch seq takes to resolve.
func microDuration(seed, seq uint64, i int) time.Duration {
	return span(mix(seed, saltMicroLen, seq, uint64(i)), microMin, microMax)
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
		// A real status stays readable: micro-glitches spare the status
		// line and bursts the status and hint lines while one is showing.
		spared = len(out) - 2
		frame = out[:len(out)-2]
	}
	if m.fx.micro(now) {
		m.microGlitch(out, spared, now)
	}
	if m.fx.bursting(now) {
		m.burst(frame)
	}
	return out
}

// textRun is a run of text cells on line y: the cells of xs, in order,
// with at most one space between two of them.
type textRun struct {
	y  int
	xs []int
}

// textRuns finds the runs of at least microSpanMin text cells in lines,
// but on line spared. A text cell is one printable, one cell wide
// character that is not a space, a border or a shade; a wide or combined
// character ends a run, so it is never split. A line whose characters do
// not add up to its width is left out.
func textRuns(lines []string, spared int) []textRun {
	var runs []textRun
	for y, line := range lines {
		if y == spared {
			continue
		}
		var found []textRun
		cur := textRun{y: y}
		flush := func() {
			if len(cur.xs) >= microSpanMin {
				found = append(found, cur)
			}
			cur = textRun{y: y}
		}
		rs := []rune(ansi.Strip(line))
		x, gap := 0, false
		for i, r := range rs {
			w := runeWidth(r)
			combined := i+1 < len(rs) && runeWidth(rs[i+1]) == 0
			switch {
			case w == 0:
			case w == 1 && textRune(r) && !combined:
				cur.xs = append(cur.xs, x)
				gap = false
			case r == ' ' && !gap && len(cur.xs) > 0:
				gap = true
			default:
				flush()
				gap = false
			}
			x += w
		}
		flush()
		if x == ansi.StringWidth(line) {
			runs = append(runs, found...)
		}
	}
	return runs
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

// microStyles color the scrambled cells.
var microStyles = []lipgloss.Style{stCyanBold, stRedBold, stYellowB}

// microGlitch scrambles the spans of the live micro-glitch in lines, but
// on line spared. Each span is microSpanMin..microSpanMax cells of one
// text run, picked from the seed; its cells hold noise glyphs or letters,
// changing every microTick, and after microHold of its time they resolve
// left to right back to the text, like the title on a song change. Only
// scrambled cells change; the others keep their text and style.
func (m Model) microGlitch(lines []string, spared int, now time.Time) {
	runs := textRuns(lines, spared)
	if len(runs) == 0 {
		return
	}
	elapsed := now.Sub(m.fx.microStart)
	for i := range m.fx.microN {
		d := microDuration(m.seed, m.fx.microSeq, i)
		if elapsed >= d {
			continue
		}
		h := mix(m.seed, saltMicro, m.fx.microSeq, uint64(i))
		r := runs[h%uint64(len(runs))]
		n := min(microSpanMin+int(h>>8%(microSpanMax-microSpanMin+1)), len(r.xs))
		xs := r.xs[int(h>>16%uint64(len(r.xs)-n+1)):][:n]
		resolved := 0
		if p := float64(elapsed) / float64(d); p > microHold {
			resolved = int((p - microHold) / (1 - microHold) * float64(n))
		}
		bucket := uint64(elapsed / microTick)
		for j := resolved; j < n; j++ {
			c := mix(h, bucket, uint64(j))
			if j > resolved && unit(c) >= 0.75 {
				continue // the first unresolved cell always flickers
			}
			glyph := string(glitchGlyphs[c>>16%uint64(len(glitchGlyphs))])
			if c>>8%3 == 0 {
				glyph = string(rune('A' + c>>24%26))
			}
			style := microStyles[c>>40%uint64(len(microStyles))]
			lines[r.y] = overlay(lines[r.y], xs[j], style.Render(glyph))
		}
	}
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
