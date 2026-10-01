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

// The NOW PLAYING rows on screen in the full layout at 80x24: the header
// and the nav bar, then the panel's frame, then its inside.
const (
	npScreenTop    = 3  // the panel's inside, under its frame
	npEmblemRight  = 78 // one cell left of the frame at 80 columns
	npTextBlockW   = 20 // the compact emblem with its text
	npEmblemAloneW = 6
)

// npModel is a loaded model at 80x24 showing state s (none for nothing
// loaded), expanded or not, its song-change glitch over.
func npModel(t *testing.T, s *playback.State, expanded bool) Model {
	t.Helper()
	m := loaded(t, playbacktest.New(), newClock())
	if s != nil {
		m, _ = step(t, m, stateMsg{state: *s})
	}
	for range glitchFrames + 1 {
		m = tick(t, m)
	}
	m.expanded = expanded
	return m
}

// npBlockRows are the plain rows the emblem draws: with its text, or
// alone, each as wide as the block.
func npBlockRows(text bool) []string {
	a := idleArt{e: emblemCompact, text: text}
	w := emblemCompact.width()
	if text {
		w = emblemCompact.blockWidth()
	}
	return stripAll(a.lines(w, len(emblemCompact.rows)))
}

// npRows are the stripped screen rows of m's artist, album and blank row
// under them.
func npRows(m Model) []string {
	lines := stripAll(strings.Split(m.render(), "\n"))
	return lines[npScreenTop+npArtistRow : npScreenTop+npArtistRow+3]
}

// hasEmblemGlyphs reports whether any of rows draws a cell of the
// emblem's blocks or its slants.
func hasEmblemGlyphs(rows []string) bool {
	for _, r := range rows {
		if strings.ContainsAny(r, "▄▀█◢◤") {
			return true
		}
	}
	return false
}

func withArtist(artist, album string) *playback.State {
	s := playing(83*time.Second, 225*time.Second)
	s.Artist, s.Album = artist, album
	return &s
}

func TestNPEmblemRightAlignedUnderTheHeart(t *testing.T) {
	m := npModel(t, ptr(playing(83*time.Second, 225*time.Second)), true)
	rows := npRows(m)
	want := npBlockRows(true)
	x0 := npEmblemRight - npTextBlockW
	for i, r := range rows {
		if got := ansi.Cut(r, x0, npEmblemRight); got != want[i] {
			t.Fatalf("row %d of the block = %q, want %q at column %d:\n%s", i, got, want[i], x0, strings.Join(rows, "\n"))
		}
		if ansi.Cut(r, npEmblemRight, npEmblemRight+1) != " " {
			t.Fatalf("row %d: the margin before the frame is not blank: %q", i, r)
		}
	}
	if !strings.HasPrefix(rows[0], "│ SAMURAI ") || !strings.HasPrefix(rows[1], "│ NEVER FADE AWAY ") {
		t.Fatalf("song info moved:\n%s", strings.Join(rows, "\n"))
	}
}

func TestNPEmblemFallsBackToTheEmblemAlone(t *testing.T) {
	artist := strings.Repeat("A", 60) // too long for the text, not for the emblem
	m := npModel(t, withArtist(artist, "ALBUM"), true)
	rows := npRows(m)
	want := npBlockRows(false)
	x0 := npEmblemRight - npEmblemAloneW
	for i, r := range rows {
		if got := ansi.Cut(r, x0, npEmblemRight); got != want[i] {
			t.Fatalf("row %d = %q, want the emblem alone %q:\n%s", i, got, want[i], strings.Join(rows, "\n"))
		}
	}
	if strings.Contains(strings.Join(rows, "\n"), "N U 1 1") {
		t.Fatal("the text drawn where it does not fit")
	}
	if !strings.Contains(rows[0], artist) {
		t.Fatalf("artist cut: %q", rows[0])
	}
}

func TestNPEmblemSkippedWhereItWouldOverlap(t *testing.T) {
	for _, tc := range []struct{ name, artist, album string }{
		{"long artist", strings.Repeat("B", 70), "ALBUM"},
		{"long album", "ARTIST", strings.Repeat("C", 70)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := npModel(t, withArtist(tc.artist, tc.album), true)
			rows := npRows(m)
			if hasEmblemGlyphs(rows) {
				t.Fatalf("emblem drawn over the song info:\n%s", strings.Join(rows, "\n"))
			}
			if !strings.Contains(rows[0], tc.artist) || !strings.Contains(rows[1], tc.album) {
				t.Fatalf("song info cut:\n%s", strings.Join(rows, "\n"))
			}
			if _, ok := m.npEmblem(); ok {
				t.Fatal("npEmblem places a block that does not fit")
			}
		})
	}
}

// Beside the list at 80 columns the panel is 27 cells inside its margins:
// the text never fits beside SAMURAI, the emblem alone does.
func TestNPEmblemInTheSidePanelWhenItFits(t *testing.T) {
	m := npModel(t, ptr(playing(83*time.Second, 225*time.Second)), false)
	rows := npRows(m)
	want := npBlockRows(false)
	x0 := npEmblemRight - npEmblemAloneW
	for i, r := range rows {
		if got := ansi.Cut(r, x0, npEmblemRight); got != want[i] {
			t.Fatalf("side panel row %d = %q, want the emblem alone %q:\n%s", i, got, want[i], strings.Join(rows, "\n"))
		}
	}
	if strings.Contains(strings.Join(rows, "\n"), "N U 1 1") {
		t.Fatal("the text drawn in the side panel")
	}
	// Nothing loaded, the hint is too long beside the list: no emblem.
	idle := npModel(t, nil, false)
	if hasEmblemGlyphs(npRows(idle)) {
		t.Fatalf("emblem over the hint:\n%s", strings.Join(npRows(idle), "\n"))
	}
}

func TestNPEmblemShownWhetherPlayingOrNot(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state *playback.State
	}{
		{"playing", ptr(playing(83*time.Second, 225*time.Second))},
		{"paused", ptr(pausedState())},
		{"nothing loaded", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := npModel(t, tc.state, true)
			rows := npRows(m)
			want := npBlockRows(true)
			for i, r := range rows {
				if got := ansi.Cut(r, npEmblemRight-npTextBlockW, npEmblemRight); got != want[i] {
					t.Fatalf("row %d = %q, want %q", i, got, want[i])
				}
			}
		})
	}
}

func TestNPEmblemNotInTheCompactLayoutNorUnderOverlays(t *testing.T) {
	m := npModel(t, ptr(playing(83*time.Second, 225*time.Second)), true)
	if _, ok := m.npEmblem(); !ok {
		t.Fatal("expanded 80x24: no emblem placed")
	}
	small := m
	small.width, small.height = 50, 20
	if _, ok := small.npEmblem(); ok {
		t.Fatal("compact layout: emblem placed")
	}
	for _, name := range []string{"help", "settings", "boot", "shutdown"} {
		o := m
		switch name {
		case "help":
			o.help = true
		case "settings":
			o.settings = true
		case "boot":
			o.boot = true
		case "shutdown":
			o.shutdown = true
		}
		if _, ok := o.npEmblem(); ok {
			t.Errorf("%s over the panel: emblem placed", name)
		}
	}
}

func TestNPEmblemAddsNoZones(t *testing.T) {
	m := npModel(t, ptr(playing(83*time.Second, 225*time.Second)), true)
	_, zs := m.layout()
	fav, ok := zs.find(zoneFavPlaying)
	if !ok || fav.y != npScreenTop+npTitleRow || fav.x+fav.w != npEmblemRight {
		t.Fatalf("heart zone %v, want on row %d ending at column %d", fav, npScreenTop+npTitleRow, npEmblemRight)
	}
	for _, z := range zs {
		if z.id == zonePanelPlayer {
			continue
		}
		top := npScreenTop + npArtistRow
		if z.y >= top && z.y < top+3 && z.x+z.w > npEmblemRight-npTextBlockW && z.x < npEmblemRight {
			t.Fatalf("zone %v on the emblem", z)
		}
	}
	seek, ok := zs.find(zoneSeek)
	if !ok || seek.y != npScreenTop+npProgressRow {
		t.Fatalf("seek zone %v moved", seek)
	}
}

// npFxModel is a loaded, expanded model at 80x24 with the effects on,
// showing state s, the bars settled.
func npFxModel(t *testing.T, c *clock, s playback.State) Model {
	t.Helper()
	m := fxModel(t, c)
	m, _ = step(t, m, stateMsg{state: s})
	m.expanded = true
	for range 20 {
		m = tick(t, m)
	}
	return m
}

func TestNPGlitchTearsOneRowAndDrawsBlockNoiseInItsCells(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state playback.State
	}{
		{"playing", playing(83*time.Second, 225*time.Second)},
		{"paused", pausedState()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newClock()
			m := npFxModel(t, c, tc.state)
			if m.npGlitch.next.IsZero() {
				t.Fatal("effects on: no emblem glitch scheduled")
			}
			x0 := npEmblemRight - npTextBlockW
			glitched := 0
			for range 6 {
				c.t = m.npGlitch.next
				m.fx.nextBurst = c.t.Add(time.Hour) // keep the bursts out of the way
				m = tick(t, m)
				if !m.npGlitch.on(c.t) {
					t.Fatal("no emblem glitch at its scheduled time")
				}
				if d := m.npGlitch.end.Sub(m.npGlitch.start); d < idleGlitchMin || d > idleGlitchMax {
					t.Fatalf("emblem glitch lasts %v", d)
				}
				for m.npGlitch.on(c.t) {
					quiet := m
					quiet.npGlitch = idleGlitch{}
					got := stripAll(strings.Split(m.render(), "\n"))
					want := stripAll(strings.Split(quiet.render(), "\n"))
					changed := 0
					for y := range got {
						if got[y] == want[y] {
							continue
						}
						changed++
						if y < npScreenTop+npArtistRow || y >= npScreenTop+npArtistRow+3 {
							t.Fatalf("emblem glitch changed row %d outside the block", y)
						}
						if ansi.Cut(got[y], 0, x0) != ansi.Cut(want[y], 0, x0) || ansi.Cut(got[y], npEmblemRight, 80) != ansi.Cut(want[y], npEmblemRight, 80) {
							t.Fatalf("emblem glitch changed row %d outside its columns:\n%q\n%q", y, got[y], want[y])
						}
						g, w := cells(ansi.Cut(got[y], x0, npEmblemRight)), cells(ansi.Cut(want[y], x0, npEmblemRight))
						for x, cell := range g {
							if x < len(w) && cell != w[x] && cell != " " && !slices.Contains(bootNoiseGlyphs, cell) && !strings.Contains(want[y], cell) {
								t.Fatalf("emblem glitch drew %q, not a block glyph", cell)
							}
						}
					}
					if changed == 0 || changed > 3 {
						t.Fatalf("emblem glitch changed %d rows, want 1..3", changed)
					}
					glitched++
					c.advance(m.tickInterval())
					m = tick(t, m)
					m.fx.nextBurst = c.t.Add(time.Hour)
				}
				if !slices.Equal(npRows(m)[0:3], npRows(func() Model { q := m; q.npGlitch = idleGlitch{}; return q }())) {
					t.Fatal("emblem not clean after its glitch")
				}
			}
			if glitched == 0 {
				t.Fatal("no glitched frame drawn")
			}
		})
	}
}

func TestNPGlitchScheduleIsItsOwn(t *testing.T) {
	salts := []uint64{saltBurstGap, saltBurstLen, saltNoSignal, saltBurst, saltRain, saltRainBurst, saltIntro, saltBoot, saltShutdown, saltIdleGap, saltIdleLen, saltIdle}
	for _, s := range []uint64{saltNPGlitch, saltNPLook} {
		if slices.Contains(salts, s) {
			t.Errorf("emblem salt %d shared with another effect", s)
		}
		salts = append(salts, s)
	}
	// Paused, both emblems glitch, never in step.
	c := newClock()
	m := npFxModel(t, c, pausedState())
	if m.npGlitch.next.IsZero() || m.idle.next.IsZero() {
		t.Fatal("paused with the effects on: a glitch not scheduled")
	}
	same := 0
	for range 20 {
		if m.npGlitch.next.Equal(m.idle.next) {
			same++
		}
		c.t = m.npGlitch.next
		m = tick(t, m)
	}
	if same > 1 {
		t.Fatalf("the emblem glitch runs with the idle one (%d of 20)", same)
	}
}

func TestNPGlitchOffWithTheEffectsOff(t *testing.T) {
	c := newClock()
	m := loaded(t, playbacktest.New(), c)
	m, _ = step(t, m, stateMsg{state: pausedState()})
	m.expanded = true
	clean := npRows(m)
	if !hasEmblemGlyphs(clean) {
		t.Fatal("effects off: no emblem")
	}
	for range 40 {
		c.advance(m.tickInterval())
		m = tick(t, m)
		if !m.npGlitch.next.IsZero() || m.npGlitch.on(c.t) {
			t.Fatal("emblem glitch scheduled with the effects off")
		}
		if !slices.Equal(npRows(m), clean) {
			t.Fatal("emblem glitched with the effects off")
		}
	}
	if m.tickInterval() != idleTick {
		t.Fatalf("effects off, paused tick = %v, want %v", m.tickInterval(), idleTick)
	}
}

func TestNPTickLandsOnTheEmblemGlitch(t *testing.T) {
	c := newClock()
	m := npFxModel(t, c, pausedState())
	calm := func() {
		m.fx.nextBurst = c.t.Add(time.Hour)
		m.idle.next = c.t.Add(time.Hour)
	}
	calm()
	if m.tickFast {
		t.Fatal("paused: still on the fast tick")
	}
	for range 4 {
		if got, want := m.tickInterval(), max(min(idleTick, m.npGlitch.next.Sub(c.t)), minWake); got != want {
			t.Fatalf("paused tick = %v, want %v", got, want)
		}
		for c.t.Add(m.tickInterval()).Before(m.npGlitch.next) {
			c.advance(m.tickInterval())
			m = tick(t, m)
			calm()
		}
		c.advance(m.tickInterval())
		m = tick(t, m)
		calm()
		if !m.npGlitch.on(c.t) {
			t.Fatalf("tick at %v missed the emblem glitch at %v", c.t, m.npGlitch.start)
		}
		for m.npGlitch.on(c.t) {
			if got, want := m.tickInterval(), max(min(burstTick, m.npGlitch.end.Sub(c.t)), minWake); got != want {
				t.Fatalf("emblem glitch tick = %v, want %v", got, want)
			}
			c.advance(m.tickInterval())
			m = tick(t, m)
			calm()
		}
		if late := c.t.Sub(m.npGlitch.end); late < 0 || late >= minWake {
			t.Fatalf("emblem glitch ended at %v, the tick landed at %v", m.npGlitch.end, c.t)
		}
	}
}

func TestNPGlitchStartsNoIntro(t *testing.T) {
	c := newClock()
	m := npFxModel(t, c, playing(83*time.Second, 225*time.Second))
	before := m
	before.npGlitch = idleGlitch{}
	c.t = m.npGlitch.next
	m.fx.nextBurst = c.t.Add(time.Hour)
	m = tick(t, m)
	before.fx = m.fx
	frameDiffers := false
	for range 20 {
		if !m.npGlitch.on(c.t) {
			break
		}
		prev := before
		prev.frame, prev.animFrame, prev.bars, prev.rain = m.frame, m.animFrame, m.bars, m.rain
		b, _ := prev.baseLayout()
		a, _ := m.baseLayout()
		if !slices.Equal(stripAll(a), stripAll(b)) {
			frameDiffers = true
		}
		if got := m.withIntro(prev, struct{}{}); got.intro.seq != m.intro.seq {
			t.Fatalf("the emblem glitch started an intro on cells %v", got.intro.cells)
		}
		c.advance(m.tickInterval())
		m = tick(t, m)
	}
	if !frameDiffers {
		t.Fatal("the glitch never showed in the compared frame")
	}
	// The field rows stop short of the emblem.
	_, zs := m.baseLayout()
	for _, region := range m.introRegions(zs) {
		for _, r := range region {
			if (r.y == npScreenTop+npArtistRow || r.y == npScreenTop+npAlbumRow) && r.x1 > npEmblemRight-npTextBlockW {
				t.Fatalf("intro region %v covers the emblem", r)
			}
		}
	}
}
