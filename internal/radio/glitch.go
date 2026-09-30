package radio

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Signal effects: the screen now and then seems to lose the signal. Every
// burstGapMin..burstGapMax a burst of burstMin..burstMax tears a few rows
// sideways, corrupts a few cells and may run a static bar; one burst in
// noSignalOdds also flashes NO SIGNAL. Between bursts, data rain (changing
// codes) runs in the free space only, and Night City alerts take the empty
// status line now and then.
//
// Everything is drawn over the finished frame (see Model.decorate), cell
// for cell, so no line changes width and the clickable zones, laid out
// from the undecorated frame, stay where they are. Timings and glyphs come
// from the seed, the injected clock and the frame counter: tests replay
// them exactly. No timer is added: the animation tick runs at rainTick
// while the effects are on, and at burstTick during a burst only.
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
	// burstTick is the frame time during a burst (about 15 fps), rainTick
	// the slowest one while the effects are on, which steps the data rain.
	burstTick = 66 * time.Millisecond
	rainTick  = 500 * time.Millisecond
	// rainMinRun is the shortest run of free cells that takes a code.
	rainMinRun = 12
)

// Salts keep the pseudo-random streams of the effects apart.
const (
	saltBurstGap uint64 = iota + 101
	saltBurstLen
	saltNoSignal
	saltAlertGap
	saltAlert
	saltBurst
	saltRain
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
}

// advance moves the schedule to now. While the effects are not active
// (off, typing or the tiny layout) nothing runs and nothing is pending,
// so becoming active again never fires what was missed at once.
func (e effects) advance(now time.Time, seed uint64, active bool) effects {
	if !active {
		e.nextBurst, e.burstEnd, e.nextAlert, e.alertEnd = time.Time{}, time.Time{}, time.Time{}, time.Time{}
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
	m.rain(out, now)
	if m.fx.bursting(now) {
		frame := out
		if m.status != "" && len(out) > 2 {
			// A real status stays readable: the burst spares the status
			// and hint lines while one is showing.
			frame = out[:len(out)-2]
		}
		m.burst(frame)
	}
	return out
}

// rain writes changing codes into the free space of the frame: long runs
// of the border and rule lines, the gap in the header, and, in the full
// layout, the blank rows of NOW PLAYING and the empty rows under the
// list. The status and hint lines, and every cell with text, stay as they
// are.
func (m Model) rain(lines []string, now time.Time) {
	bucket := uint64(now.UnixMilli() / rainTick.Milliseconds())
	last := len(lines) - 2 // the status and hint lines take no rain
	for y := 0; y < last; y++ {
		for _, r := range runsOf(lines[y], '─', 0, -1) {
			if mix(m.seed, saltRain, uint64(y), uint64(r[0]), bucket/8)%2 == 0 {
				lines[y] = m.rainCode(lines[y], y, r, bucket, stFrameDim)
			}
		}
	}
	if last > 0 {
		// The header gap, between the wordmark and the clock.
		for _, r := range runsOf(lines[0], ' ', 0, -1) {
			lines[0] = m.rainCode(lines[0], 0, r, bucket, stMuted)
		}
	}
	if m.width < fullMinWidth || m.height < fullMinHeight {
		return
	}
	top, bodyH := 2, m.height-4
	rows := func(x0, x1 int, trailingOnly bool) {
		first := top + 1
		if trailingOnly {
			for y := min(top+bodyH-2, last-1); y > top; y-- {
				if !blank(lines[y], x0, x1) {
					first = y + 1
					break
				}
			}
		}
		for y := first; y < top+bodyH-1 && y < last; y++ {
			if blank(lines[y], x0, x1) && mix(m.seed, saltRain, uint64(y), uint64(x0), bucket/8)%4 == 0 {
				lines[y] = m.rainCode(lines[y], y, [2]int{x0, x1}, bucket, stFrameDim)
			}
		}
	}
	if m.expanded {
		rows(1, m.width-1, false)
		return
	}
	leftW := listPanelWidthFor(m.width)
	rows(1, leftW-1, true)
	rows(leftW+2, m.width-1, false)
}

// rainCode writes one code into the run r of line y, two cells in from
// its ends, where it moves every few seconds; its text changes every
// rainTick.
func (m Model) rainCode(line string, y int, r [2]int, bucket uint64, style lipgloss.Style) string {
	room := r[1] - r[0] - 4
	h := mix(m.seed, saltRain, uint64(y), uint64(r[0]), bucket)
	code := rainText(h)
	if len(code) > room {
		code = fmt.Sprintf("%04X", h>>48)
	}
	if len(code) > room {
		return line
	}
	x := r[0] + 2 + int(mix(m.seed, uint64(y), uint64(r[0]), bucket/8)%uint64(room-len(code)+1))
	return overlay(line, x, style.Render(code))
}

// rainText is a code for the data rain: hex, coordinates or a frequency.
func rainText(h uint64) string {
	switch h % 6 {
	case 0:
		return fmt.Sprintf("0x%04X", h>>48)
	case 1:
		return fmt.Sprintf("%02X:%02X:%02X", h>>56, h>>48&0xFF, h>>40&0xFF)
	case 2:
		return fmt.Sprintf("%02d.%02dN %03d.%02dW", h>>8%90, h>>16%100, h>>24%180, h>>32%100)
	case 3:
		return fmt.Sprintf("NC-%03X", h>>52)
	case 4:
		return fmt.Sprintf("%03d.%d MHZ", 88+h>>8%20, h>>16%10)
	}
	return fmt.Sprintf("%08b", h>>56)
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

// blank reports whether the cells [x0, x1) of line are all spaces.
func blank(line string, x0, x1 int) bool {
	runs := runsOf(line, ' ', x0, x1)
	return len(runs) == 1 && runs[0] == [2]int{x0, x1}
}

// runsOf returns the runs of at least rainMinRun cells of line equal to
// fill within the cells [x0, x1) (x1 < 0 for the end of the line), as
// [start, end) pairs.
func runsOf(line string, fill rune, x0, x1 int) [][2]int {
	var runs [][2]int
	x, start := 0, -1
	flush := func(end int) {
		if start >= 0 && end-start >= rainMinRun {
			runs = append(runs, [2]int{start, end})
		}
		start = -1
	}
	for _, c := range ansi.Strip(line) {
		w := ansi.StringWidth(string(c))
		if w == 0 {
			continue
		}
		inside := x >= x0 && (x1 < 0 || x+w <= x1)
		switch {
		case inside && c == fill:
			if start < 0 {
				start = x
			}
		default:
			flush(x)
		}
		x += w
	}
	if x1 >= 0 {
		x = min(x, x1)
	}
	flush(x)
	return runs
}
