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

func TestIdleGlitchScheduleStaysInRange(t *testing.T) {
	const stepDur = 10 * time.Millisecond
	glitches := 0
	for seed := uint64(1); seed <= 20; seed++ {
		start := time.Date(2077, 1, 1, 0, 0, 0, 0, time.UTC)
		var g idleGlitch
		var lastEnd time.Time
		seq := uint64(0)
		for now := start; now.Before(start.Add(5 * time.Minute)); now = now.Add(stepDur) {
			g = g.advance(now, seed, true)
			if g.seq == seq {
				continue
			}
			seq = g.seq
			glitches++
			if d := g.end.Sub(g.start); d < idleGlitchMin || d > idleGlitchMax {
				t.Errorf("seed %d idle glitch %d lasts %v, want %v..%v", seed, seq, d, idleGlitchMin, idleGlitchMax)
			}
			from := lastEnd
			if from.IsZero() {
				from = start
			}
			if gap := g.start.Sub(from); gap < idleGapMin || gap > idleGapMax+stepDur {
				t.Errorf("seed %d idle glitch %d comes %v after the last, want %v..%v", seed, seq, gap, idleGapMin, idleGapMax)
			}
			lastEnd = g.end
		}
	}
	if glitches < 20*5*60/int(idleGapMax/time.Second+1) {
		t.Fatalf("only %d idle glitches in 20 five minutes", glitches)
	}
	// Inactive, nothing is pending; becoming active again never fires at once.
	g := idleGlitch{}.advance(time.Unix(0, 0), 3, true)
	g = g.advance(time.Unix(100, 0), 3, false)
	if !g.next.IsZero() || g.on(time.Unix(100, 0)) {
		t.Fatal("inactive idle glitch still pending")
	}
	if g = g.advance(time.Unix(100, 0), 3, true); g.on(time.Unix(100, 0)) {
		t.Fatal("resuming fired the idle glitch at once")
	}
}

func TestIdleGlitchSaltsAreItsOwn(t *testing.T) {
	salts := []uint64{saltBurstGap, saltBurstLen, saltNoSignal, saltBurst, saltRain, saltRainBurst, saltIntro, saltBoot, saltShutdown}
	for _, s := range []uint64{saltIdleGap, saltIdleLen, saltIdle} {
		if slices.Contains(salts, s) {
			t.Errorf("idle salt %d shared with another effect", s)
		}
		salts = append(salts, s)
	}
}

// idleFxModel is a loaded, paused model at 80x24 with the signal effects
// on, its idle glitch armed by a tick.
func idleFxModel(t *testing.T, c *clock) Model {
	t.Helper()
	m := fxModel(t, c)
	m, _ = step(t, m, stateMsg{state: pausedState()})
	// Let the bars fall flat, so the tick slows down, from the same instant.
	for range 20 {
		m = tick(t, m)
	}
	return m
}

func TestIdleGlitchTearsOneRowAndDrawsBlockNoise(t *testing.T) {
	c := newClock()
	m := idleFxModel(t, c)
	if m.idle.next.IsZero() {
		t.Fatal("paused with the effects on: no idle glitch scheduled")
	}
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	clean := stripAll(art.lines(w, h))
	glitched := 0
	for range 10 {
		c.t = m.idle.next
		m = tick(t, m)
		if !m.idle.on(c.t) {
			t.Fatalf("no idle glitch at its scheduled time")
		}
		if d := m.idle.end.Sub(m.idle.start); d < idleGlitchMin || d > idleGlitchMax {
			t.Fatalf("idle glitch lasts %v", d)
		}
		for m.idle.on(c.t) {
			rows := rainArea(t, m)
			changed := 0
			for y := range rows {
				if rows[y] == clean[y] {
					continue
				}
				changed++
				if y < art.y || y >= art.y+len(art.e.rows) {
					t.Fatalf("idle glitch changed row %d outside the emblem", y)
				}
				for x, cell := range cells(rows[y]) {
					old := cells(clean[y])
					if x < len(old) && cell != old[x] && cell != " " && !slices.Contains(bootNoiseGlyphs, cell) && !strings.Contains(clean[y], cell) {
						t.Fatalf("idle glitch drew %q, not a block glyph", cell)
					}
				}
			}
			if changed == 0 || changed > 4 {
				t.Fatalf("idle glitch changed %d rows, want 1..4", changed)
			}
			glitched++
			c.advance(m.tickInterval())
			m = tick(t, m)
		}
		if !emblemAt(rainArea(t, m), art) || !slices.Equal(rainArea(t, m), clean) {
			t.Fatal("emblem not clean after the idle glitch")
		}
	}
	if glitched == 0 {
		t.Fatal("no glitched frame drawn")
	}
}

func TestIdleGlitchStaysInTheRainArea(t *testing.T) {
	c := newClock()
	m := idleFxModel(t, c)
	c.t = m.idle.next
	m = tick(t, m)
	quiet := m
	quiet.idle = idleGlitch{}
	got, want := strings.Split(m.render(), "\n"), strings.Split(quiet.render(), "\n")
	w, h := m.vizSize()
	bottom := m.height - 2 - 1 // over the status, hint and panel frame
	x0 := listPanelWidthFor(m.width) + 2 + nowPlayingMargin
	for y := range got {
		if got[y] == want[y] {
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
}

func TestIdleGlitchOffWithTheEffectsOff(t *testing.T) {
	c := newClock()
	m := loaded(t, playbacktest.New(), c)
	w, h := m.vizSize()
	art, _ := idleArtFor(w, h)
	clean := stripAll(art.lines(w, h))
	for range 40 {
		c.advance(m.tickInterval())
		m = tick(t, m)
		if !m.idle.next.IsZero() || m.idle.on(c.t) {
			t.Fatal("idle glitch scheduled with the effects off")
		}
		if !slices.Equal(rainArea(t, m), clean) {
			t.Fatal("emblem glitched with the effects off")
		}
		if m.tickInterval() != idleTick {
			t.Fatalf("effects off, idle tick = %v, want %v", m.tickInterval(), idleTick)
		}
	}
}

func TestIdleTickLandsOnTheIdleGlitch(t *testing.T) {
	c := newClock()
	m := idleFxModel(t, c)
	m.fx.nextBurst = c.t.Add(time.Hour) // keep the global bursts out of the way
	if m.tickFast {
		t.Fatal("paused: still on the fast tick")
	}
	for range 5 {
		want := min(idleTick, m.idle.next.Sub(c.t))
		if got := m.tickInterval(); got != max(want, minWake) {
			t.Fatalf("idle tick = %v, want %v (next idle glitch in %v)", got, want, m.idle.next.Sub(c.t))
		}
		for c.t.Add(m.tickInterval()).Before(m.idle.next) {
			c.advance(m.tickInterval())
			m = tick(t, m)
			m.fx.nextBurst = c.t.Add(time.Hour)
		}
		c.advance(m.tickInterval())
		m = tick(t, m)
		m.fx.nextBurst = c.t.Add(time.Hour)
		if !m.idle.on(c.t) {
			t.Fatalf("tick at %v missed the idle glitch at %v", c.t, m.idle.start)
		}
		for m.idle.on(c.t) {
			if got, want := m.tickInterval(), max(min(burstTick, m.idle.end.Sub(c.t)), minWake); got != want {
				t.Fatalf("idle glitch tick = %v, want %v", got, want)
			}
			c.advance(m.tickInterval())
			m = tick(t, m)
			m.fx.nextBurst = c.t.Add(time.Hour)
		}
		// It lands on the end, or minWake past it when the end came closer
		// than that (no spinning).
		if late := c.t.Sub(m.idle.end); late < 0 || late >= minWake {
			t.Fatalf("idle glitch ended at %v, the tick landed at %v", m.idle.end, c.t)
		}
	}
}

func TestPlayingSchedulesNoIdleGlitch(t *testing.T) {
	c := newClock()
	m := fxModel(t, c)
	for range 30 {
		c.advance(m.tickInterval())
		m = tick(t, m)
		if !m.idle.next.IsZero() {
			t.Fatal("playing: idle glitch scheduled")
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
