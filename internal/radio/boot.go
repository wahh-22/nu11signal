package radio

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The boot splash is nu11signal's own boot screen: from the first frame,
// for bootDur on the injected clock, the body is the null emblem with
// the BOOTING line under it (see splash), whatever the Apple Music link
// is doing; the header keeps LINKING while it links. The boot starts at
// the first size, the first frame drawn, so the time New waits for the
// terminal is not taken from it. Then the normal UI scrambles in (see
// withIntro), access linked or not.
//
// With the signal effects on the splash glitches from the first frame, a
// burst that never stops while the boot lasts (see bootGlitch), and the
// tick runs at burstTick; with them off (x, --calm) it shows still and a
// single tick ends it on time. No timer is added: tickInterval cuts the
// tick chain to the boot's frames and its end.
//
// Any key skips it, and does nothing else but q and ctrl+c, which open
// the quit modal as usual; a click skips it too. Refused access wins at
// once: the auth error screen takes the boot's place (see onAuth).
// Options.SkipBoot starts past it.
const (
	bootDur  = 1500 * time.Millisecond
	bootText = "BOOTING NU11SIGNAL..."
)

// saltBoot keeps the boot glitch's stream apart from the other effects'.
const saltBoot uint64 = 501

// startBoot starts the boot clock at the first size, once.
func (m Model) startBoot() Model {
	if m.boot && m.bootEnd.IsZero() {
		m.bootEnd = m.now().Add(bootDur)
	}
	return m
}

// endBootOnTime ends the boot once its time is up.
func (m Model) endBootOnTime() Model {
	if m.boot && !m.bootEnd.IsZero() && !m.now().Before(m.bootEnd) {
		m.boot = false
	}
	return m
}

// bootKey handles a key press during the boot: it skips the boot, and q
// and ctrl+c open the quit modal besides.
func (m Model) bootKey(k string) (Model, tea.Cmd) {
	m.boot = false
	if k == keyQuit || k == keyCtrlC {
		return m.askQuit(), nil
	}
	return m, nil
}

// bootGlitching reports whether the boot glitch draws: the boot on, the
// signal effects on, and room for the splash (not the tiny layout).
func (m Model) bootGlitching() bool {
	return m.boot && m.fx.on && m.auth != authFailed && m.width >= tinyMinWidth && m.height >= tinyMinHeight
}

// bootInterval cuts d, the time to the next frame, to the boot: burstTick
// while its glitch draws, and the boot's end at the latest.
func (m Model) bootInterval(d time.Duration) time.Duration {
	if !m.boot || m.bootEnd.IsZero() {
		return d
	}
	if m.bootGlitching() {
		d = min(d, burstTick)
	}
	return min(d, max(m.bootEnd.Sub(m.now()), minWake))
}

// bootGlitch draws a burst over the splash's body in lines, one look per
// tick frame, from the seed: 1..3 of its drawn rows torn 1..2 cells
// sideways, 6..12 noise cells over the drawn cells of those rows, and on
// one frame in three a static bar across a row of the body. The header,
// the status line and the footer are left clean.
func (m Model) bootGlitch(lines []string) {
	lo, hi := splashTop, len(lines)-2
	var drawn []int
	for y := lo; y < hi; y++ {
		if strings.TrimSpace(ansi.Strip(lines[y])) != "" {
			drawn = append(drawn, y)
		}
	}
	if len(drawn) == 0 {
		return
	}
	r := mix(m.seed, saltBoot, m.frame)
	n := uint64(len(drawn))
	for i := range 1 + r%3 {
		h := mix(r, 1, i)
		y := drawn[h%n]
		lines[y] = shift(lines[y], int(h>>8%2)+1, h>>16%2 == 0)
	}
	for i := range 6 + r>>16%7 {
		h := mix(r, 4, i)
		y := drawn[h%n]
		line := lines[y]
		w := ansi.StringWidth(line)
		x0 := w - ansi.StringWidth(strings.TrimLeft(ansi.Strip(line), " "))
		if w <= x0 {
			continue
		}
		glyph := noiseGlyphs[h>>32%uint64(len(noiseGlyphs))]
		lines[y] = overlay(line, x0+int(h>>8%uint64(w-x0)), noiseStyles[h>>48%uint64(len(noiseStyles))].Render(glyph))
	}
	if r>>8%3 == 0 {
		y := lo + int(mix(r, 2)%uint64(hi-lo))
		lines[y] = staticBar(m.width, mix(r, 3))
	}
}
