package radio

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The boot splash is nu11signal's own boot screen: from the first frame,
// for bootDur on the injected clock, the body is the emblem (the logo) with
// the BOOTING line under it (see splash), whatever the Apple Music link
// is doing; the header keeps LINKING while it links. The boot starts at
// the first size, the first frame drawn, so the time New waits for the
// terminal is not taken from it. Then the normal UI scrambles in (see
// withIntro), access linked or not.
//
// With the signal effects on the splash glitches from the first frame, a
// burst that never stops while the boot lasts (see splashGlitch), and the
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

// splashText is the line under the emblem: SHUTTING DOWN while
// nu11signal shuts down (see shutdown.go), else BOOTING.
func (m Model) splashText() string {
	if m.shutdown {
		return shutdownText
	}
	return bootText
}

// splashGlitching reports whether the splash glitch draws: the boot on
// (access not refused) or the shutdown on, the signal effects on, and
// room for the splash (not the tiny layout).
func (m Model) splashGlitching() bool {
	return ((m.boot && m.auth != authFailed) || m.shutdown) && m.fx.on &&
		m.width >= tinyMinWidth && m.height >= tinyMinHeight
}

// splashInterval cuts d, the time to the next frame, to the splash:
// burstTick while its glitch draws, and the boot's or the shutdown's end
// at the latest. A shutdown past its end waiting for the player's close
// keeps d (the close's answer quits, see onClosed), never a busy minWake.
func (m Model) splashInterval(d time.Duration) time.Duration {
	switch {
	case m.shutdown:
		if m.splashGlitching() {
			d = min(d, burstTick)
		}
		if left := m.shutdownEnd.Sub(m.now()); left > 0 {
			d = min(d, max(left, minWake))
		}
		return d
	case !m.boot || m.bootEnd.IsZero():
		return d
	}
	if m.splashGlitching() {
		d = min(d, burstTick)
	}
	return min(d, max(m.bootEnd.Sub(m.now()), minWake))
}

// emblemNoiseGlyphs are the noise the emblem's glitches draw (the boot
// and shutdown splashes, the idle emblem, the swap's moving edge over
// it): dense Braille patterns, like the art they corrupt, so no stray
// letters, digits or block shades flash over the emblem.
var emblemNoiseGlyphs = []string{"⣿", "⣷", "⣯", "⣟", "⡿", "⢿", "⠿", "⣶", "⣾", "⣻"}

// emblemNoise is the noise glyph h picks for a cell showing under: never
// under itself, so every noise cell shows.
func emblemNoise(h uint64, under rune) string {
	n := uint64(len(emblemNoiseGlyphs))
	g := emblemNoiseGlyphs[h%n]
	if []rune(g)[0] == under {
		g = emblemNoiseGlyphs[(h+1)%n]
	}
	return g
}

// cellRune is the rune of the cell at x of the styled line, a space for
// none.
func cellRune(line string, x int) rune {
	if rs := []rune(ansi.Strip(ansi.Cut(line, x, x+1))); len(rs) > 0 {
		return rs[0]
	}
	return ' '
}

// The splash glitch corrupts bootNoiseMin..bootNoiseMin+bootNoiseSpan-1
// cells a frame, as many as a periodic burst (see effects.look).
const (
	bootNoiseMin  = 4
	bootNoiseSpan = 7
)

// splashGlitch draws a burst over the splash's body in lines (boot or
// shutdown, each from its own salt), one look per tick frame, from the
// seed, as intense as a periodic burst: 1..3 of its
// drawn rows torn 1..2 cells sideways, bootNoiseMin.. Braille noise
// cells (see emblemNoiseGlyphs) over the drawn cells of those rows, and on one frame
// in three a static bar across a row of the body. The header, the status
// line and the footer are left clean.
func (m Model) splashGlitch(lines []string) {
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
	salt := saltBoot
	if m.shutdown {
		salt = saltShutdown
	}
	r := mix(m.seed, salt, m.frame)
	n := uint64(len(drawn))
	for i := range 1 + r%3 {
		h := mix(r, 1, i)
		y := drawn[h%n]
		lines[y] = shift(lines[y], int(h>>8%2)+1, h>>16%2 == 0)
	}
	for i := range bootNoiseMin + r>>16%bootNoiseSpan {
		h := mix(r, 4, i)
		y := drawn[h%n]
		line := lines[y]
		w := ansi.StringWidth(line)
		x0 := w - ansi.StringWidth(strings.TrimLeft(ansi.Strip(line), " "))
		if w <= x0 {
			continue
		}
		x := x0 + int(h>>8%uint64(w-x0))
		glyph := emblemNoise(h>>32, cellRune(line, x))
		lines[y] = overlay(line, x, noiseStyles[h>>48%uint64(len(noiseStyles))].Render(glyph))
	}
	if r>>8%3 == 0 {
		y := lo + int(mix(r, 2)%uint64(hi-lo))
		lines[y] = staticBar(m.width, mix(r, 3))
	}
}
