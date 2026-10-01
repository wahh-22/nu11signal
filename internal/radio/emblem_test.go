package radio

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// bootModel is a model in its boot splash at w x h, the effects on or
// off, Apple Music access still linking (no authMsg yet), on the clock c.
func bootModel(t *testing.T, c *clock, w, h int, fx bool) Model {
	t.Helper()
	m := New(playbacktest.New(), Options{Now: c.now, Seed: 2077, Effects: fx})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	if !m.boot || m.auth != authPending {
		t.Fatalf("boot %v auth %v; want booting, still linking", m.boot, m.auth)
	}
	return m
}

// bootLine is the line under the emblem, spaced out.
const bootLine = "B O O T I N G   N U 1 1 S I G N A L . . ."

// endBoot runs the clock past the boot and ticks: the boot ends.
func endBoot(t *testing.T, m Model, c *clock) Model {
	t.Helper()
	c.advance(bootDur)
	m, _ = step(t, m, tickMsg{gen: m.tickGen})
	return m
}

// The masks paint exactly the drawn cells: a space in the row is a space
// in the mask and every other cell is r or s.
func TestEmblemMasksMatchTheirRows(t *testing.T) {
	for name, e := range map[string]emblem{"large": emblemLarge, "compact": emblemCompact} {
		if len(e.rows) != len(e.mask) {
			t.Fatalf("%s: %d rows, %d mask rows", name, len(e.rows), len(e.mask))
		}
		for i, row := range e.rows {
			r, m := []rune(row), []rune(e.mask[i])
			if len(r) != len(m) {
				t.Fatalf("%s row %d: %d cells, mask %d", name, i, len(r), len(m))
			}
			for x := range r {
				if (r[x] == ' ') != (m[x] == ' ') || (m[x] != ' ' && m[x] != 'r' && m[x] != 's') {
					t.Errorf("%s row %d cell %d: %q masked %q", name, i, x, r[x], m[x])
				}
			}
		}
	}
	if got := EmblemRows(); !slices.Equal(got, emblemCompact.rows) {
		t.Fatalf("EmblemRows() = %q; want the compact rows", got)
	}
}

// From the first frame the body is the large emblem with its text, and
// the BOOTING line under it; the panels are not drawn. The header keeps
// LINKING while access links.
func TestBootShowsTheEmblemAndTheBootLine(t *testing.T) {
	m := bootModel(t, newClock(), 80, 24, false)
	screen := plain(m)
	for _, want := range append(slices.Clone(emblemLarge.rows), "N U 1 1", "S I G N A L", "◢◤◢◤◢◤◢◤◢◤", bootLine, "LINKING") {
		if !strings.Contains(screen, strings.TrimRight(want, " ")) {
			t.Errorf("boot screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "NOW PLAYING") || strings.Contains(screen, "PLAYLISTS ─") || strings.Contains(screen, "L I N K I N G") {
		t.Fatalf("boot screen draws the panels or the old LINKING line:\n%s", screen)
	}
	lines := strings.Split(screen, "\n")
	if len(lines) != 24 || !strings.HasPrefix(lines[1], "◢◤◢◤ ") || !strings.HasPrefix(lines[22], "◢◤◢◤ ") {
		t.Fatalf("header, nav, status or footer moved:\n%s", screen)
	}
}

// The boot does not wait for access, nor end with it: granted at once,
// the splash stays until its time is up.
func TestBootShowsWhateverTheAuth(t *testing.T) {
	c := newClock()
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	m := New(f, Options{Now: c.now, Seed: 2077})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = step(t, m, run(t, m.authorizeCmd()))
	if m.auth != authOK {
		t.Fatalf("auth %v; want granted", m.auth)
	}
	if screen := plain(m); !strings.Contains(screen, bootLine) || !strings.Contains(screen, "AUTH OK") {
		t.Fatalf("granted access ended the boot early:\n%s", screen)
	}
	c.advance(bootDur - time.Millisecond)
	m, _ = step(t, m, tickMsg{gen: m.tickGen})
	if !strings.Contains(plain(m), bootLine) {
		t.Fatalf("the boot ended before its time:\n%s", plain(m))
	}
}

// Once its time is up the normal UI shows, access still linking: LINKING
// is left to the header, the splash is gone for good.
func TestBootEndsOnTimeWhileLinking(t *testing.T) {
	c := newClock()
	m := New(playbacktest.New(), Options{Now: c.now, Seed: 2077})
	m, cmd := step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd == nil {
		t.Fatal("the boot started without a tick to end it")
	}
	// The tick chain, calm, lands on the boot's end and ends it there.
	start := c.now()
	ticks := 0
	for ; m.boot && ticks < 10; ticks++ {
		c.advance(m.tickInterval())
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	if got := c.now().Sub(start); got != bootDur || ticks > 2 {
		t.Fatalf("the calm boot ended after %v in %d ticks; want %v in at most 2", got, ticks, bootDur)
	}
	screen := plain(m)
	if m.boot || strings.Contains(screen, bootLine) || splashBody(m) {
		t.Fatalf("the boot did not end:\n%s", screen)
	}
	if !strings.Contains(screen, "NOW PLAYING") || !strings.Contains(screen, "LINKING") {
		t.Fatalf("the normal UI with LINKING in the header did not show:\n%s", screen)
	}
	if m.auth != authPending {
		t.Fatalf("auth %v; want still linking", m.auth)
	}
}

// The block (emblem, gap and text) is centered on the width as a whole,
// and so is the BOOTING line, each on its own; together they are
// centered on the body's height.
func TestBootCentersTheBlockAndTheLine(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			lines := strings.Split(plain(bootModel(t, newClock(), sz.w, sz.h, false)), "\n")
			blockW := emblemLarge.blockWidth()
			left := (sz.w - blockW) / 2
			bodyH := sz.h - 4
			top := 2 + (bodyH-len(emblemLarge.rows)-2)/2
			for i, row := range emblemLarge.rows {
				row = strings.TrimRight(row, " ")
				got := []rune(lines[top+i])
				if len(got) < left+len([]rune(row)) || strings.TrimSpace(string(got[:left])) != "" || string(got[left:left+len([]rune(row))]) != row {
					t.Fatalf("emblem row %d at line %d reads %q; want it %d cells in", i, top+i, lines[top+i], left)
				}
			}
			if got, want := lines[top+1], strings.Repeat(" ", left)+emblemLarge.rows[1]+"   N U 1 1"; got != want {
				t.Fatalf("text row reads %q; want %q", got, want)
			}
			line := lines[top+len(emblemLarge.rows)+1]
			if lines[top+len(emblemLarge.rows)] != "" {
				t.Fatalf("no blank row under the emblem: %q", lines[top+len(emblemLarge.rows)])
			}
			if want := strings.Repeat(" ", (sz.w-ansi.StringWidth(bootLine))/2) + bootLine; line != want {
				t.Fatalf("BOOTING line reads %q; want %q", line, want)
			}
		})
	}
}

// Where the large block does not fit the compact one takes its place,
// and the line goes plain where its spaced form does not fit.
func TestBootFallsBackToTheCompactEmblem(t *testing.T) {
	screen := plain(bootModel(t, newClock(), 40, 10, false))
	for _, row := range emblemCompact.rows {
		if !strings.Contains(screen, strings.TrimRight(row, " ")) {
			t.Fatalf("compact boot screen lacks %q:\n%s", row, screen)
		}
	}
	if strings.Contains(screen, strings.TrimSpace(emblemLarge.rows[1])) {
		t.Fatalf("compact boot screen draws the large emblem:\n%s", screen)
	}
	if !strings.Contains(screen, "S I G N A L") || !strings.Contains(screen, "BOOTING NU11SIGNAL...") {
		t.Fatalf("compact boot screen lacks its text:\n%s", screen)
	}
}

// Where neither emblem fits only the BOOTING line is left.
func TestBootFallsBackToTheTextAlone(t *testing.T) {
	lines := strings.Split(plain(bootModel(t, newClock(), 40, 7, false)), "\n")
	body := strings.Join(lines[2:len(lines)-2], "\n")
	if strings.ContainsAny(body, "▄▀█") || strings.Contains(body, "S I G N A L") {
		t.Fatalf("text-only boot body draws the emblem:\n%s", body)
	}
	if !strings.Contains(body, "BOOTING NU11SIGNAL...") {
		t.Fatalf("text-only boot body lacks the BOOTING line:\n%s", body)
	}
}

// With the effects on the splash glitches from the first frame and on
// every frame of the boot, a burst's tears and noise over the body only,
// at burstTick; the header and the footer stay clean.
func TestBootGlitches(t *testing.T) {
	c := newClock()
	m := bootModel(t, c, 80, 24, true)
	// Past the intro, so only the glitch draws over the splash.
	c.advance(introDur)
	m, _ = step(t, m, tickMsg{gen: m.tickGen})
	if d := m.tickInterval(); d != burstTick {
		t.Fatalf("boot ticks every %v; want %v", d, burstTick)
	}
	frames := map[string]bool{}
	for range 4 {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
		base, _ := m.baseLayout()
		lines, _ := m.layout()
		if reflect.DeepEqual(lines, base) {
			t.Fatalf("frame %d of the boot drew no glitch", m.frame)
		}
		for _, y := range []int{0, 1, 22, 23} {
			if lines[y] != base[y] {
				t.Fatalf("frame %d glitched row %d outside the body", m.frame, y)
			}
		}
		if !strings.ContainsAny(ansi.Strip(strings.Join(lines, "\n")), "░▒▓▚▞") {
			t.Fatalf("frame %d of the boot has no noise cells", m.frame)
		}
		frames[strings.Join(lines, "\n")] = true
	}
	if len(frames) < 2 {
		t.Fatal("the boot glitch does not move")
	}
	// Replayed from the same seed and frame, it draws the same.
	again := bootModel(t, newClock(), 80, 24, true)
	again.frame, again.intro = m.frame, m.intro
	again.now = c.now
	if a, b := first(again.layout()), first(m.layout()); !reflect.DeepEqual(a, b) {
		t.Fatal("the boot glitch is not deterministic")
	}
}

// With the effects off (x, --calm) the splash shows still: no glitch, no
// intro.
func TestBootIsStillWhenCalm(t *testing.T) {
	c := newClock()
	m := bootModel(t, c, 80, 24, false)
	for range 3 {
		if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
			t.Fatalf("the calm boot drew effects on frame %d", m.frame)
		}
		c.advance(burstTick)
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
}

// Any key skips the boot and is not otherwise acted on; q and ctrl+c
// open the quit modal as well. A click skips it too.
func TestAKeyOrAClickSkipsTheBoot(t *testing.T) {
	for _, k := range []string{"down", "space", keyEnter, "/", "x", "?"} {
		t.Run(k, func(t *testing.T) {
			f := playbacktest.New()
			m := New(f, Options{Now: newClock().now, Seed: 2077})
			m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
			before := len(f.Calls())
			next, cmd := press(t, m, k)
			if next.boot || cmd != nil || len(f.Calls()) != before {
				t.Fatalf("%q: boot %v command %v calls %v; want the boot skipped alone", k, next.boot, cmd != nil, f.Calls()[before:])
			}
			if next.help || next.fx.on != m.fx.on || next.focus != m.focus || next.cursor() != m.cursor() || !slices.Equal(stackKinds(next), stackKinds(m)) {
				t.Fatalf("%q acted beyond skipping the boot", k)
			}
			if !strings.Contains(plain(next), "NOW PLAYING") {
				t.Fatalf("%q did not show the normal UI:\n%s", k, plain(next))
			}
		})
	}
	for _, k := range []string{keyQuit, keyCtrlC} {
		m := bootModel(t, newClock(), 80, 24, false)
		m, cmd := press(t, m, k)
		if m.boot {
			t.Fatalf("%q did not skip the boot", k)
		}
		assertAsking(t, m, cmd)
	}
	m := bootModel(t, newClock(), 80, 24, false)
	m, cmd := step(t, m, tea.MouseClickMsg{X: 40, Y: 10, Button: tea.MouseLeft})
	if m.boot || cmd != nil {
		t.Fatalf("a click: boot %v command %v; want the boot skipped alone", m.boot, cmd != nil)
	}
}

// Refused access wins at once: the auth error screen replaces the boot.
func TestAuthFailureEndsTheBootAtOnce(t *testing.T) {
	f := playbacktest.New()
	f.AuthStatus = playback.AuthDenied
	m := New(f, Options{Now: newClock().now, Seed: 2077, Effects: true})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = step(t, m, run(t, m.authorizeCmd()))
	screen := plain(m)
	if m.boot || !strings.Contains(screen, "AUTH // ERROR") || strings.Contains(screen, bootLine) {
		t.Fatalf("refused access during the boot: boot %v\n%s", m.boot, screen)
	}
	if lines, _ := m.layout(); !reflect.DeepEqual(lines, first(m.baseLayout())) {
		t.Fatal("the auth error screen glitches")
	}
}

// Once the boot is over the normal view stays, access granted or not.
func TestAuthorizedViewDropsTheEmblem(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	screen := plain(m)
	if strings.Contains(screen, bootLine) || splashBody(m) {
		t.Fatalf("authorized view still shows the boot screen:\n%s", screen)
	}
	if !strings.Contains(screen, "NOW PLAYING") {
		t.Fatalf("authorized view lacks the panels:\n%s", screen)
	}
}

// splashBody reports whether m's list side shows the emblem's text, the
// splash's body; the idle emblem in NOW PLAYING's spectrum area (see
// idle.go) is not the splash.
func splashBody(m Model) bool {
	for _, l := range strings.Split(plain(m), "\n") {
		if strings.Contains(ansi.Cut(l, 0, listPanelWidthFor(m.width)), "S I G N A L") {
			return true
		}
	}
	return false
}

// The splash body is not clickable: every zone is in the header.
func TestBootZonesAreTheHeaderOnly(t *testing.T) {
	_, zs := bootModel(t, newClock(), 80, 24, false).baseLayout()
	if len(zs) == 0 {
		t.Fatal("no zones: the nav tabs should stay clickable")
	}
	for _, z := range zs {
		if z.y > 1 {
			t.Errorf("zone %q at row %d in the splash body", z.id, z.y)
		}
	}
}

// The ring takes the label color and the slash the accent color, in
// every theme.
func TestEmblemTakesTheThemeColors(t *testing.T) {
	for _, tt := range []struct{ theme, label, accent string }{
		{"NIGHT CITY", "255;95;87", "252;238;10"},
		{"BLUE", "52;122;255", "124;92;255"},
	} {
		t.Run(tt.theme, func(t *testing.T) {
			m := bootModel(t, newClock(), 80, 24, false)
			useTheme(t, tt.theme)
			screen := m.render()
			ring, slash := stLabel.Render("▄████▄"), stAccent.Render("▄▀")
			if !strings.Contains(ring, tt.label) || !strings.Contains(slash, tt.accent) {
				t.Fatalf("styles %q %q lack the theme colors", ring, slash)
			}
			if !strings.Contains(screen, ring+slash) {
				t.Fatalf("the first emblem row is not ring then slash colored:\n%s", screen)
			}
			if !strings.Contains(screen, stLabel.Render("▄█▀")+"   "+stAccent.Render("▄")) {
				t.Fatalf("the second emblem row is not ring then slash colored:\n%s", screen)
			}
		})
	}
}

// The splash scrambles in on the first frame, its text only (the
// emblem's block glyphs are not text cells), and the normal UI scrambles
// in when the boot ends.
func TestBootScramblesIn(t *testing.T) {
	c := newClock()
	m := bootModel(t, c, 80, 24, true)
	if !m.introAnimating() {
		t.Fatal("the splash did not intro")
	}
	base, _ := m.baseLayout()
	lines := strings.Split(ansi.Strip(strings.Join(base, "\n")), "\n")
	y := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, bootLine) })
	if len(m.intro.cells[y]) == 0 {
		t.Fatalf("the BOOTING line has no intro cells: %v", m.intro.cells)
	}
	for row, xs := range m.intro.cells {
		cells := []rune(lines[row])
		for _, x := range xs {
			if x < len(cells) && cells[x] >= 0x2580 && cells[x] <= 0x259F {
				t.Fatalf("emblem cell %q at %d,%d scrambles", cells[x], x, row)
			}
		}
		if row < 2 || row >= 22 {
			t.Fatalf("row %d outside the body scrambles", row)
		}
	}
	// The boot ends: the content replacing the splash scrambles in.
	seq := m.intro.seq
	m = endBoot(t, m, c)
	if m.intro.seq == seq || !m.introAnimating() {
		t.Fatal("the content replacing the splash did not intro")
	}
}

func TestBootGolden80x24(t *testing.T) {
	assertGolden(t, "boot_80x24.golden", plain(bootModel(t, newClock(), 80, 24, false)))
}

func TestBootBlueGolden80x24(t *testing.T) {
	m := bootModel(t, newClock(), 80, 24, false)
	useTheme(t, "BLUE")
	assertGolden(t, "boot_blue_80x24.golden", m.View().Content)
}

func TestSplashSurvivesNoRoom(t *testing.T) {
	// A body with no rows left (a terminal at the full layout's edge)
	// draws nothing instead of panicking.
	for _, h := range []int{-3, 0, 1} {
		if got := splash(80, h, bootText); len(got) > max(h, 1) {
			t.Fatalf("splash(80, %d) drew %d lines", h, len(got))
		}
	}
}

// The boot glitch draws no letters or digits: its noise is shades and
// blocks only, as many cells as a periodic burst corrupts.
func TestBootNoiseIsBlocksOnly(t *testing.T) {
	for _, g := range bootNoiseGlyphs {
		for _, r := range g {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				t.Fatalf("boot noise glyph %q is text", g)
			}
		}
	}
	if bootNoiseMin != 4 || bootNoiseSpan != 7 {
		t.Fatalf("boot noise %d..%d cells; want a burst's 4..10", bootNoiseMin, bootNoiseMin+bootNoiseSpan-1)
	}
}
