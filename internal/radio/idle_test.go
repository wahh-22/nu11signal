package radio

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// rainArea is the spectrum area of m's NOW PLAYING, stripped: its rows
// without the margin, as nowPlaying lays them out.
func rainArea(t *testing.T, m Model) []string {
	t.Helper()
	w, h := m.vizSize()
	if w <= 0 || h <= 0 {
		t.Fatalf("no rain area at %dx%d", m.width, m.height)
	}
	lines, _ := m.nowPlaying(m.playerPanelWidth()-2, m.height-6)
	var out []string
	for _, l := range lines[len(lines)-h:] {
		out = append(out, ansi.Strip(strings.TrimPrefix(l, " ")))
	}
	return out
}

// emblemAt reports whether rows hold the stripped lines of art's block
// from column x, row y.
func emblemAt(rows []string, art idleArt) bool {
	for i, want := range art.plain() {
		y := art.y + i
		if y >= len(rows) {
			return false
		}
		got := strings.TrimRight(ansi.Cut(rows[y], art.x, art.x+ansi.StringWidth(want)), " ")
		if got != strings.TrimRight(want, " ") {
			return false
		}
	}
	return true
}

func pausedState() playback.State {
	s := playing(90*time.Second, 225*time.Second)
	s.Status = playback.StatusPaused
	return s
}

func TestIdleEmblemShowsCenteredInTheRainArea(t *testing.T) {
	c := newClock()
	f := playbacktest.New()
	for _, tc := range []struct {
		name  string
		state *playback.State
	}{
		{"nothing loaded", nil},
		{"paused", ptr(pausedState())},
		{"stopped", ptr(playback.State{Status: playback.StatusStopped})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := loaded(t, f, c)
			if tc.state != nil {
				m, _ = step(t, m, stateMsg{state: *tc.state})
			}
			w, h := m.vizSize()
			art, ok := idleArtFor(w, h)
			if !ok || art.e.rows[0] != emblemLarge.rows[0] || !art.text {
				t.Fatalf("idleArtFor(%d, %d) = %+v, %v; want the large emblem with its text", w, h, art, ok)
			}
			if wantX, wantY := (w-emblemLarge.blockWidth())/2, (h-len(emblemLarge.rows))/2; art.x != wantX || art.y != wantY {
				t.Fatalf("emblem at %d,%d, want centered at %d,%d", art.x, art.y, wantX, wantY)
			}
			rows := rainArea(t, m)
			if !emblemAt(rows, art) {
				t.Fatalf("rain area lacks the emblem at %d,%d:\n%s", art.x, art.y, strings.Join(rows, "\n"))
			}
			if !strings.Contains(m.render(), "S I G N A L") {
				t.Fatal("frame lacks the emblem's text")
			}
		})
	}
}

func TestPlayingBringsTheRainBack(t *testing.T) {
	c := newClock()
	m := loaded(t, playbacktest.New(), c)
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	for range 20 {
		c.advance(fastTick)
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	for _, r := range rainArea(t, m) {
		if strings.ContainsAny(r, "▄▀█") || strings.Contains(r, "N U 1 1") {
			t.Fatalf("playing rain area shows the emblem:\n%s", strings.Join(rainArea(t, m), "\n"))
		}
	}
	m, _ = step(t, m, stateMsg{state: pausedState()})
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	if !emblemAt(rainArea(t, m), art) {
		t.Fatal("pausing did not bring the emblem back")
	}
}

func TestIdleEmblemFallsBackBySize(t *testing.T) {
	lw, lh := emblemLarge.blockWidth(), len(emblemLarge.rows)
	cw, ch := emblemCompact.blockWidth(), len(emblemCompact.rows)
	for _, tc := range []struct {
		w, h int
		want *emblem
		text bool
	}{
		{lw, lh, &emblemLarge, true},
		{lw + 10, lh + 6, &emblemLarge, true},
		{lw - 1, lh, &emblemCompact, true},
		{lw, lh - 1, &emblemCompact, true},
		{cw - 1, lh, &emblemLarge, false},
		{cw - 1, lh - 1, &emblemCompact, false},
		{emblemLarge.width() - 1, lh, &emblemCompact, false},
		{emblemCompact.width(), ch, &emblemCompact, false},
		{emblemCompact.width() - 1, ch, nil, false},
		{cw, ch - 1, nil, false},
		{0, 0, nil, false},
	} {
		art, ok := idleArtFor(tc.w, tc.h)
		if tc.want == nil {
			if ok {
				t.Errorf("idleArtFor(%d, %d) = %+v, want nothing", tc.w, tc.h, art)
			}
			continue
		}
		if !ok || art.e.rows[0] != tc.want.rows[0] || art.text != tc.text {
			t.Errorf("idleArtFor(%d, %d) = %+v, %v; want emblem %q text %v", tc.w, tc.h, art, ok, tc.want.rows[0], tc.text)
			continue
		}
		bw := art.e.width()
		if art.text {
			bw = art.e.blockWidth()
		}
		if art.x != (tc.w-bw)/2 || art.y != (tc.h-len(art.e.rows))/2 {
			t.Errorf("idleArtFor(%d, %d) at %d,%d, want centered", tc.w, tc.h, art.x, art.y)
		}
		lines := art.lines(tc.w, tc.h)
		if len(lines) != tc.h {
			t.Errorf("idleArtFor(%d, %d) draws %d rows", tc.w, tc.h, len(lines))
		}
		for _, l := range lines {
			if ansi.StringWidth(l) != tc.w {
				t.Errorf("idleArtFor(%d, %d) row %q is %d wide", tc.w, tc.h, ansi.Strip(l), ansi.StringWidth(l))
			}
		}
		if !emblemAt(stripAll(lines), art) {
			t.Errorf("idleArtFor(%d, %d) lines lack the emblem", tc.w, tc.h)
		}
	}
}

func TestIdleEmblemLargeInTheExpandedPlayer(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m.expanded = true
	art, ok := idleArtFor(m.vizSize())
	if !ok || art.e.rows[0] != emblemLarge.rows[0] || !art.text || !emblemAt(rainArea(t, m), art) {
		t.Fatalf("expanded player: emblem %+v, %v", art, ok)
	}
}

func TestIdleSaltsAreItsOwn(t *testing.T) {
	salts := []uint64{saltBurstGap, saltBurstLen, saltNoSignal, saltBurst, saltRain, saltRainBurst, saltIntro, saltBoot, saltShutdown}
	for _, s := range []uint64{saltIdle, saltSwap} {
		if slices.Contains(salts, s) {
			t.Errorf("idle salt %d shared with another effect", s)
		}
		salts = append(salts, s)
	}
}

// idleFxModel is a loaded, paused model at 80x24 with the signal effects
// on, its bars flat, the global bursts kept out of the way and the clock
// on an idle frame boundary.
func idleFxModel(t *testing.T, c *clock) Model {
	t.Helper()
	m := fxModel(t, c)
	m, _ = step(t, m, stateMsg{state: pausedState()})
	// Let the bars fall flat, so the tick slows down, from the same instant.
	for range 20 {
		m = tick(t, m)
	}
	// Let the pause's swap to the emblem end (see vizswap.go).
	c.advance(swapDur)
	c.advance(idleFrameWait(c.t))
	m.fx.nextBurst = c.t.Add(time.Hour)
	return m.tickAt(t, c)
}

// tickAt ticks m at the clock's time, the global bursts kept away.
func (m Model) tickAt(t *testing.T, c *clock) Model {
	t.Helper()
	m = tick(t, m)
	m.fx.nextBurst = c.t.Add(time.Hour)
	return m
}

// idleBox is the stripped segment of row y inside the emblem's box.
func idleBox(rows []string, a idleArt, w, y int) string {
	x0, x1 := a.box(w)
	return ansi.Cut(rows[y], x0, x1)
}

// emblemChanges counts the cells of the emblem's box that rows change
// from the clean art, a torn row compared with the clean row torn: it
// fails on more than one torn row or a changed cell that is no block
// noise glyph.
func emblemChanges(t *testing.T, rows []string, a idleArt, w, h int) int {
	t.Helper()
	clean := stripAll(a.lines(w, h))
	changes, torn := 0, 0
	for i := range a.e.rows {
		y := a.y + i
		got := cells(idleBox(rows, a, w, y))
		x0, x1 := a.box(w)
		best, bestDiff := -1, []string(nil)
		for k, want := range []string{
			ansi.Cut(clean[y], x0, x1),
			shift(ansi.Cut(clean[y], x0, x1), 1, true),
			shift(ansi.Cut(clean[y], x0, x1), 1, false),
		} {
			var diff []string
			for x, cell := range cells(want) {
				if x < len(got) && got[x] != cell {
					diff = append(diff, got[x])
				}
			}
			if best < 0 || len(diff) < len(bestDiff) {
				best, bestDiff = k, diff
			}
		}
		if best > 0 {
			torn++
		}
		for _, cell := range bestDiff {
			if !slices.Contains(bootNoiseGlyphs, cell) {
				t.Fatalf("idle glitch drew %q over the emblem, not a block glyph:\n%s", cell, strings.Join(rows, "\n"))
			}
		}
		changes += len(bestDiff)
	}
	if torn > 1 {
		t.Fatalf("idle glitch tore %d rows", torn)
	}
	return changes + torn
}

func TestIdleGlitchIsContinuousAndSoft(t *testing.T) {
	c := newClock()
	m := idleFxModel(t, c)
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	quiet, glitched := 0, 0
	for i := range 80 {
		if got := m.tickInterval(); got != idleFrameTick {
			t.Fatalf("frame %d: idle tick with the effects on = %v, want %v", i, got, idleFrameTick)
		}
		rows := rainArea(t, m)
		n := emblemChanges(t, rows, art, w, h)
		if n > idleNoiseMax+1 {
			t.Fatalf("frame %d: idle glitch changed %d emblem cells, want at most %d and a tear", i, n, idleNoiseMax)
		}
		if n == 0 {
			quiet++
			if quiet >= 8 {
				t.Fatalf("frame %d: the emblem stayed clean %d frames in a row", i, quiet)
			}
		} else {
			quiet = 0
			glitched++
		}
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
	if glitched < 40 {
		t.Fatalf("only %d of 80 frames glitched", glitched)
	}
}

// outsideBox are the cells of rows outside the emblem's box.
func outsideBox(rows []string, a idleArt, w int) []string {
	x0, x1 := a.box(w)
	var out []string
	for y, r := range rows {
		cs := cells(r)
		for x := range w {
			if y >= a.y && y < a.y+len(a.e.rows) && x >= x0 && x < x1 {
				continue
			}
			if x < len(cs) {
				out = append(out, cs[x])
			}
		}
	}
	return out
}

func TestIdleAreaHoldsTheEmblemAlone(t *testing.T) {
	c := newClock()
	m := idleFxModel(t, c)
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	for i := range 60 {
		rows := rainArea(t, m)
		for _, g := range outsideBox(rows, art, w) {
			if g != " " {
				t.Fatalf("frame %d: %q drawn outside the emblem's box:\n%s", i, g, strings.Join(rows, "\n"))
			}
		}
		emblemChanges(t, rows, art, w, h) // only the soft glitch inside
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
}

func TestIdleGlitchStaysInTheRainArea(t *testing.T) {
	c := newClock()
	m := idleFxModel(t, c)
	w, h := m.vizSize()
	bottom := m.height - 2 - 1 // over the status, hint and panel frame
	x0 := listPanelWidthFor(m.width) + 2 + nowPlayingMargin
	for range 10 {
		still := m
		still.fx.on = false
		got, want := strings.Split(m.render(), "\n"), strings.Split(still.render(), "\n")
		for y := range got {
			// The status line says FX OFF on the still frame.
			if got[y] == want[y] || y == m.height-2 {
				continue
			}
			if y < bottom-h || y >= bottom {
				t.Fatalf("idle glitch changed row %d outside the rain area", y)
			}
			g, q := ansi.Strip(got[y]), ansi.Strip(want[y])
			if ansi.Cut(g, 0, x0) != ansi.Cut(q, 0, x0) || ansi.Cut(g, x0+w, m.width) != ansi.Cut(q, x0+w, m.width) {
				t.Fatalf("idle glitch changed row %d outside the rain columns", y)
			}
		}
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
}

func TestIdleStaticWithTheEffectsOff(t *testing.T) {
	c := newClock()
	m := loaded(t, playbacktest.New(), c)
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	first := rainArea(t, m)
	if n := emblemChanges(t, first, art, w, h); n != 0 {
		t.Fatalf("effects off: %d emblem cells glitched", n)
	}
	if !slices.Equal(first, stripAll(art.lines(w, h))) {
		t.Fatalf("effects off: the idle area is not the clean emblem:\n%s", strings.Join(first, "\n"))
	}
	for range 40 {
		c.advance(m.tickInterval())
		m = tick(t, m)
		if !slices.Equal(rainArea(t, m), first) {
			t.Fatal("effects off: the idle area changed")
		}
		if m.tickInterval() != idleTick {
			t.Fatalf("effects off, idle tick = %v, want %v", m.tickInterval(), idleTick)
		}
	}
}

func TestIdleFrameTickLandsOnFrames(t *testing.T) {
	at := time.Date(2077, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, off := range []time.Duration{0, time.Millisecond, idleFrameTick / 2, idleFrameTick - time.Nanosecond} {
		now := at.Add(off)
		d := idleFrameWait(now)
		if d <= 0 || d > idleFrameTick {
			t.Fatalf("wait from +%v = %v", off, d)
		}
		if idleFrame(now.Add(d)) != idleFrame(now)+1 || idleFrame(now.Add(d-time.Nanosecond)) != idleFrame(now) {
			t.Fatalf("wait from +%v = %v does not land on the next frame", off, d)
		}
	}
}

func TestPlayingRunsNoIdle(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	for range 30 {
		c.advance(m.tickInterval())
		m = tick(t, m)
		if m.idleActive() || m.idleShown() {
			t.Fatal("playing: the idle emblem is on")
		}
		for _, r := range rainArea(t, m) {
			if strings.ContainsAny(r, "▄▀█") || strings.Contains(r, "N U 1 1") {
				t.Fatal("playing: the rain area shows the emblem")
			}
		}
	}
}

func stripAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func TestIdleGolden80x24(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = step(t, m, stateMsg{state: pausedState()})
	for range glitchFrames + 1 { // the title's song-change glitch
		m = tick(t, m)
	}
	assertGolden(t, "idle_80x24.golden", ansi.Strip(m.View().Content))
}
