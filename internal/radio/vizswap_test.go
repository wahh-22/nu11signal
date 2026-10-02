package radio

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// emblemCells counts the drawn cells of art (its non-blank cells, the
// text included) that rows show where the clean art has them.
func emblemCells(rows []string, art idleArt, w, h int) int {
	n := 0
	for y, line := range stripAll(art.lines(w, h)) {
		got := cells(rows[y])
		for x, cell := range cells(line) {
			if cell != " " && x < len(got) && got[x] == cell {
				n++
			}
		}
	}
	return n
}

// rainCells counts the rain's drawn cells, rain being its stripped rows,
// and how many of them rows show.
func rainCells(rows, rain []string) (shown, drawn int) {
	for y, line := range rain {
		got := cells(rows[y])
		for x, cell := range cells(line) {
			if cell == " " {
				continue
			}
			drawn++
			if x < len(got) && got[x] == cell {
				shown++
			}
		}
	}
	return shown, drawn
}

// noiseCells counts the Braille noise glyphs in rows that the clean
// art (its stripped rows) does not show there: the emblem is drawn in
// Braille too, so its own cells do not count.
func noiseCells(rows, clean []string) int {
	n := 0
	for y, r := range rows {
		was := cells(clean[y])
		for x, cell := range cells(r) {
			if slices.Contains(emblemNoiseGlyphs, cell) && (x >= len(was) || was[x] != cell) {
				n++
			}
		}
	}
	return n
}

// swapSlack is how far the emblem's count may move against the swap
// between two frames: the idle glitch's noise cells and a torn row.
func swapSlack(art idleArt) int { return idleNoiseMax + art.e.width() + 2 }

func TestSwapDissolvesTheEmblemIntoTheRain(t *testing.T) {
	c := newClock()
	m := idleFxModel(t, c)
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	full := emblemCells(stripAll(art.lines(w, h)), art, w, h)
	intro := m.intro.seq
	m, _ = step(t, m, stateMsg{state: playing(90*time.Second, 225*time.Second)})
	m.fx.nextBurst = c.t.Add(time.Hour)
	start := c.t
	prev, frames, glitched := full, 0, false
	// The rain's cells shown and drawn, pooled over the swap's first and
	// second halves: the rain starts sparse, so a frame alone says little.
	var early, late [2]int
	for c.t.Sub(start) < swapDur {
		if !m.swapAnimating() {
			t.Fatalf("+%v: the swap stopped before %v", c.t.Sub(start), swapDur)
		}
		if got := m.tickInterval(); got > burstTick {
			t.Fatalf("+%v: tick during the swap = %v, want at most %v", c.t.Sub(start), got, burstTick)
		}
		rows := rainArea(t, m)
		e := emblemCells(rows, art, w, h)
		if e > prev+swapSlack(art) {
			t.Fatalf("+%v: the emblem grew back from %d to %d cells:\n%s", c.t.Sub(start), prev, e, strings.Join(rows, "\n"))
		}
		prev = e
		shown, drawn := rainCells(rows, stripAll(m.rain.Render(w, h)))
		half := &early
		if m.swapLevel() >= 0.5 {
			half = &late
		}
		half[0] += shown
		half[1] += drawn
		// Around the middle the area mixes both. The window stops short of
		// 0.7: with seed 2077 the 80x24 idle art (the head alone) keeps
		// only a cell or two past 0.6 and none by 0.75.
		if l := m.swapLevel(); l > 0.3 && l < 0.6 {
			if e == 0 || e == full {
				t.Fatalf("level %.2f: %d of %d emblem cells, want a mix", l, e, full)
			}
			if noiseCells(rows, stripAll(art.lines(w, h))) > idleNoiseMax+2 {
				glitched = true
			}
		}
		frames++
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
	if frames < 6 {
		t.Fatalf("the swap drew %d frames, want a dissolve", frames)
	}
	if !glitched {
		t.Fatal("no glitch glyphs at the moving edge")
	}
	if early[1] < 10 || late[1] < 10 || early[0]*late[1] >= late[0]*early[1] {
		t.Fatalf("rain cells shown/drawn early %v, late %v: want the share rising", early, late)
	}
	if m.swapAnimating() || m.swapLevel() != 1 {
		t.Fatalf("after %v: swap still running at %.2f", swapDur, m.swapLevel())
	}
	if got, want := rainArea(t, m), stripAll(m.rain.Render(w, h)); !slices.Equal(got, want) {
		t.Fatalf("after the swap the area is not the rain:\n%s", strings.Join(got, "\n"))
	}
	if got := m.tickInterval(); got != fastTick {
		t.Fatalf("after the swap the tick = %v, want %v", got, fastTick)
	}
	if m.intro.seq != intro {
		t.Fatal("the swap started a content intro")
	}
}

func TestSwapSettlesTheRainIntoTheEmblem(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	m, _ = step(t, m, stateMsg{state: playing(90*time.Second, 225*time.Second)})
	for range int(swapDur/burstTick) + 20 {
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
	if m.swapLevel() != 1 {
		t.Fatalf("playing: swap level %.2f, want 1", m.swapLevel())
	}
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	full := emblemCells(stripAll(art.lines(w, h)), art, w, h)
	intro := m.intro.seq
	m, _ = step(t, m, stateMsg{state: pausedState()})
	start := c.t
	prev, glitched := 0, false
	for c.t.Sub(start) < swapDur {
		if got := m.tickInterval(); got > burstTick {
			t.Fatalf("+%v: tick during the swap = %v, want at most %v", c.t.Sub(start), got, burstTick)
		}
		rows := rainArea(t, m)
		e := emblemCells(rows, art, w, h)
		if e < prev-swapSlack(art) {
			t.Fatalf("+%v: the emblem lost cells, %d to %d", c.t.Sub(start), prev, e)
		}
		prev = e
		// Around the middle the area mixes both. The window stops short of
		// 0.7: with seed 2077 the 80x24 idle art (the head alone) keeps
		// only a cell or two past 0.6 and none by 0.75.
		if l := m.swapLevel(); l > 0.3 && l < 0.6 {
			if e == 0 || e == full {
				t.Fatalf("level %.2f: %d of %d emblem cells, want a mix", l, e, full)
			}
			if noiseCells(rows, stripAll(art.lines(w, h))) > idleNoiseMax+2 {
				glitched = true
			}
		}
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
	if !glitched {
		t.Fatal("no glitch glyphs at the moving edge")
	}
	if m.swapAnimating() || m.swapLevel() != 0 {
		t.Fatalf("after %v: swap still running at %.2f", swapDur, m.swapLevel())
	}
	if got, want := rainArea(t, m), stripAll(m.idleRows(w, h)); !slices.Equal(got, want) {
		t.Fatalf("after the swap the area is not the emblem:\n%s", strings.Join(got, "\n"))
	}
	for range 40 {
		if m.bars.flat() {
			break
		}
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
	if got := m.tickInterval(); got != max(idleFrameWait(c.t), minWake) {
		t.Fatalf("after the swap the tick = %v, want the idle frame's %v", got, idleFrameWait(c.t))
	}
	if m.intro.seq != intro {
		t.Fatal("the swap started a content intro")
	}
}

func TestSwapReversesFromWhereItIs(t *testing.T) {
	c := newClock()
	m := idleFxModel(t, c)
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	full := emblemCells(stripAll(art.lines(w, h)), art, w, h)
	m, _ = step(t, m, stateMsg{state: playing(90*time.Second, 225*time.Second)})
	start := c.t
	for c.t.Sub(start) < swapDur/2 {
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
	half := c.t.Sub(start)
	level := m.swapLevel()
	before := emblemCells(rainArea(t, m), art, w, h)
	m, _ = step(t, m, stateMsg{state: pausedState()})
	if got := m.swapLevel(); got != level {
		t.Fatalf("the flip moved the level from %.3f to %.3f", level, got)
	}
	if after := emblemCells(rainArea(t, m), art, w, h); after < before-swapSlack(art) || after > before+swapSlack(art) {
		t.Fatalf("the flip jumped the emblem from %d to %d cells", before, after)
	}
	if !m.swapAnimating() {
		t.Fatal("the flip ended the swap")
	}
	flip := c.t
	for c.t.Sub(flip) < half {
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
	if m.swapAnimating() || m.swapLevel() != 0 {
		t.Fatalf("%v after the flip the swap runs at %.2f, want back to the emblem", half, m.swapLevel())
	}
	if e := emblemCells(rainArea(t, m), art, w, h); e < full-idleNoiseMax-art.e.width() {
		t.Fatalf("back to the emblem with %d of %d cells", e, full)
	}
}

func TestSwapCutsWithTheEffectsOff(t *testing.T) {
	c := newClock()
	m := loaded(t, playbacktest.New(), c)
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	m, _ = step(t, m, stateMsg{state: playing(90*time.Second, 225*time.Second)})
	if m.swapAnimating() || m.swapLevel() != 1 {
		t.Fatalf("effects off: playing swaps at %.2f", m.swapLevel())
	}
	if e := emblemCells(rainArea(t, m), art, w, h); e != 0 {
		t.Fatalf("effects off: %d emblem cells left on play", e)
	}
	c.advance(m.tickInterval())
	m = tick(t, m)
	m, _ = step(t, m, stateMsg{state: pausedState()})
	if m.swapAnimating() || m.swapLevel() != 0 {
		t.Fatalf("effects off: pausing swaps at %.2f", m.swapLevel())
	}
	if got := rainArea(t, m); !slices.Equal(got, stripAll(art.lines(w, h))) {
		t.Fatalf("effects off: pausing did not cut to the emblem:\n%s", strings.Join(got, "\n"))
	}
}

func TestSwapRunsInTheExpandedPlayer(t *testing.T) {
	c := newClock()
	m := idleFxModel(t, c)
	m.expanded = true
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	full := emblemCells(stripAll(art.lines(w, h)), art, w, h)
	m, _ = step(t, m, stateMsg{state: playing(90*time.Second, 225*time.Second)})
	start := c.t
	for m.swapLevel() < 0.5 && c.t.Sub(start) < swapDur {
		c.advance(m.tickInterval())
		m = m.tickAt(t, c)
	}
	if e := emblemCells(rainArea(t, m), art, w, h); e == 0 || e == full {
		t.Fatalf("expanded, level %.2f: %d of %d emblem cells, want a mix", m.swapLevel(), e, full)
	}
}

func TestSwapNoneWithoutTheRainArea(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	m, _ = step(t, m, stateMsg{state: pausedState()})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 60, Height: 16})
	if w, h := m.vizSize(); w > 0 && h > 0 {
		t.Fatalf("60x16 has a rain area %dx%d", w, h)
	}
	m, _ = step(t, m, stateMsg{state: playing(90*time.Second, 225*time.Second)})
	if m.swapAnimating() || m.swapLevel() != 1 {
		t.Fatalf("no rain area: playing swaps at %.2f", m.swapLevel())
	}
}

func TestSwapThresholdsReachEveryCell(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{1, 2}, {40, 6}, {74, 12}} {
		for y := range sz.h {
			for x := range sz.w {
				if th := swapThreshold(2077, x, y, sz.w, sz.h); th < 0 || th >= 1 {
					t.Fatalf("threshold at %d,%d of %dx%d = %v, want in [0,1)", x, y, sz.w, sz.h, th)
				}
			}
		}
	}
	// The emblem leaves from the edges: the corners go before the middle.
	w, h := 74, 12
	for seed := range uint64(20) {
		if swapThreshold(seed, 0, 0, w, h) >= swapThreshold(seed, w/2, h/2, w, h) {
			t.Fatalf("seed %d: the corner does not go before the middle", seed)
		}
	}
}

func TestSwapCutsOnThePlayersFirstState(t *testing.T) {
	c := newClock()
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	m := New(f, Options{SkipBoot: true, Now: c.now, Seed: 2077, Effects: true})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.swapAnimating() || m.swapLevel() != 0 {
		t.Fatalf("before any state: swap at %.2f, want the emblem", m.swapLevel())
	}
	// Music already on when the player first reports: no dissolve.
	m, _ = step(t, m, stateMsg{state: playing(90*time.Second, 225*time.Second)})
	if m.swapAnimating() || m.swapLevel() != 1 {
		t.Fatalf("first state playing: swap at %.2f, want the rain", m.swapLevel())
	}
}
