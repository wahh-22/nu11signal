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
// in the mask and every other cell is r or s; every row of a size is as
// wide as the others.
func TestEmblemMasksMatchTheirRows(t *testing.T) {
	for name, e := range map[string]emblem{
		"large": emblemLarge, "compact": emblemCompact,
		"large head": emblemLargeHead, "compact head": emblemCompactHead,
	} {
		if len(e.rows) != len(e.mask) {
			t.Fatalf("%s: %d rows, %d mask rows", name, len(e.rows), len(e.mask))
		}
		for i, row := range e.rows {
			r, m := []rune(row), []rune(e.mask[i])
			if len(r) != len(m) || ansi.StringWidth(row) != e.width() {
				t.Fatalf("%s row %d: %d cells, mask %d, %d wide of %d", name, i, len(r), len(m), ansi.StringWidth(row), e.width())
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
	if rows, mask := EmblemArt(); !slices.Equal(rows, emblemLarge.rows) || !slices.Equal(mask, emblemLarge.mask) {
		t.Fatalf("EmblemArt() = %q %q; want the large rows and mask", rows, mask)
	}
}

// raisedDots counts the raised Braille dots of rows.
func raisedDots(rows []string) int {
	n := 0
	for _, row := range rows {
		for _, r := range row {
			if brailleRune(r) {
				for b := r - 0x2800; b != 0; b &= b - 1 {
					n++
				}
			}
		}
	}
	return n
}

// The emblem is the logo: the masked head with its headphones and LED
// eyes, then NU11SIGNAL over five slanted bars. The large one is 52 x 8
// cells, all Braille, the wordmark drawn in dots; the compact one 24 x 6,
// its head Braille and NU11SIGNAL and the bars as text beside it. The
// heads alone are the first 16 and 12 cells of their rows, a blank
// column away from the wordmark. NU11 (and the LEDs under the top row)
// take the secondary color, the rest the primary one.
func TestEmblemIsTheLogo(t *testing.T) {
	for _, tt := range []struct {
		name              string
		e, head           emblem
		w, h, headW, dots int
		nameRow           int
		nameRun, nameMask string
		barsRun, barsMask string
	}{
		{"large", emblemLarge, emblemLargeHead, 52, 8, 16, 648, 3,
			"⣿⣆⢿⢸⡇⢸⡇⠚⣿ ⠐⢻⡇ ⢾⣉⡉", "sssssssss sss rrr", "", ""},
		{"compact", emblemCompact, emblemCompactHead, 24, 6, 12, 204, 2,
			"NU11SIGNAL", "ssssrrrrrr", "⡾⡾⡾⡾⡾", "rrrrr"},
	} {
		if tt.e.width() != tt.w || len(tt.e.rows) != tt.h {
			t.Fatalf("%s emblem %d x %d; want %d x %d", tt.name, tt.e.width(), len(tt.e.rows), tt.w, tt.h)
		}
		if got := raisedDots(tt.e.rows); got != tt.dots {
			t.Errorf("%s emblem raises %d Braille dots; want %d", tt.name, got, tt.dots)
		}
		if tt.head.width() != tt.headW || len(tt.head.rows) != tt.h {
			t.Fatalf("%s head %d x %d; want %d x %d", tt.name, tt.head.width(), len(tt.head.rows), tt.headW, tt.h)
		}
		for i, row := range tt.e.rows {
			cells, kinds := []rune(row), []rune(tt.e.mask[i])
			if string(cells[:tt.headW]) != tt.head.rows[i] || string(kinds[:tt.headW]) != tt.head.mask[i] {
				t.Errorf("%s head row %d = %q; want the row's first %d cells", tt.name, i, tt.head.rows[i], tt.headW)
			}
			if cells[tt.headW] != ' ' {
				t.Errorf("%s row %d: no blank column after the head", tt.name, i)
			}
			for x, r := range cells {
				if r != ' ' && !brailleRune(r) && (tt.name == "large" || x < tt.headW) {
					t.Errorf("%s row %d draws %q in its art, not a Braille cell", tt.name, i, r)
				}
			}
		}
		for _, run := range []struct{ cells, mask string }{{tt.nameRun, tt.nameMask}, {tt.barsRun, tt.barsMask}} {
			if run.cells == "" {
				continue
			}
			found := false
			for i, row := range tt.e.rows {
				if at := strings.Index(row, run.cells); at >= 0 {
					x := len([]rune(row[:at]))
					got := string([]rune(tt.e.mask[i])[x : x+len([]rune(run.cells))])
					if got != run.mask {
						t.Errorf("%s %q masked %q; want %q", tt.name, run.cells, got, run.mask)
					}
					if run.cells == tt.nameRun && i != tt.nameRow {
						t.Errorf("%s wordmark on row %d; want %d", tt.name, i, tt.nameRow)
					}
					found = true
				}
			}
			if !found {
				t.Errorf("%s emblem lacks %q", tt.name, run.cells)
			}
		}
	}
}

// brailleRune reports whether r is a Braille pattern (U+2800..U+28FF).
func brailleRune(r rune) bool { return r >= 0x2800 && r <= 0x28FF }

// Braille cells are not text: the intros and the text glitches leave
// the emblem alone, as they left its block glyphs.
func TestBrailleIsNotText(t *testing.T) {
	for _, r := range []rune{'⠀', '⢀', '⣿', '⠋', 0x28FF} {
		if textRune(r) {
			t.Errorf("textRune(%q) = true; want Braille kept out of the text", r)
		}
	}
	for _, r := range []rune{'A', '7', '/', 'ｱ'} {
		if !textRune(r) {
			t.Errorf("textRune(%q) = false; want text", r)
		}
	}
}

// From the first frame the body is the large emblem, and the BOOTING
// line under it; the panels are not drawn. The header keeps
// LINKING while access links.
func TestBootShowsTheEmblemAndTheBootLine(t *testing.T) {
	m := bootModel(t, newClock(), 80, 24, false)
	screen := plain(m)
	for _, want := range append(slices.Clone(emblemLarge.rows), bootLine, "LINKING") {
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

// The emblem is centered on the width, and so is the BOOTING line, each
// on its own; together they are centered on the body's height.
func TestBootCentersTheBlockAndTheLine(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			lines := strings.Split(plain(bootModel(t, newClock(), sz.w, sz.h, false)), "\n")
			left := (sz.w - emblemLarge.width()) / 2
			bodyH := sz.h - 4
			top := 2 + (bodyH-len(emblemLarge.rows)-2)/2
			for i, row := range emblemLarge.rows {
				if got, want := lines[top+i], strings.TrimRight(strings.Repeat(" ", left)+row, " "); got != want {
					t.Fatalf("emblem row %d at line %d reads %q; want %q", i, top+i, got, want)
				}
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

// Where the large emblem does not fit the compact one takes its place,
// and the line goes plain where its spaced form does not fit.
func TestBootFallsBackToTheCompactEmblem(t *testing.T) {
	// 60x13 leaves a 9-row body, short of the large emblem's 10 rows;
	// 51x20 is narrower than its 52 cells.
	for _, sz := range []struct{ w, h int }{{60, 13}, {51, 20}} {
		screen := plain(bootModel(t, newClock(), sz.w, sz.h, false))
		for _, row := range emblemCompact.rows {
			if !strings.Contains(screen, strings.TrimRight(row, " ")) {
				t.Fatalf("%dx%d: compact boot screen lacks %q:\n%s", sz.w, sz.h, row, screen)
			}
		}
		if strings.Contains(screen, strings.TrimSpace(emblemLarge.rows[3])) {
			t.Fatalf("%dx%d: compact boot screen draws the large emblem:\n%s", sz.w, sz.h, screen)
		}
		if !strings.Contains(screen, bootLine) && !strings.Contains(screen, bootText) {
			t.Fatalf("%dx%d: compact boot screen lacks the BOOTING line:\n%s", sz.w, sz.h, screen)
		}
	}
}

// Where neither emblem fits only the BOOTING line is left.
func TestBootFallsBackToTheTextAlone(t *testing.T) {
	// 40x11 leaves a 7-row body, short of the compact emblem's 8 rows.
	lines := strings.Split(plain(bootModel(t, newClock(), 40, 11, false)), "\n")
	body := strings.Join(lines[2:len(lines)-2], "\n")
	if strings.ContainsFunc(body, brailleRune) || strings.Contains(body, "  NU11SIGNAL") {
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
		if n := emblemNoiseOver(lines, base); n == 0 {
			t.Fatalf("frame %d of the boot has no Braille noise cells", m.frame)
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

// splashBody reports whether m's list side shows the emblem's head, the
// splash's body; the idle emblem in NOW PLAYING's spectrum area (see
// idle.go) is not the splash.
func splashBody(m Model) bool {
	for _, l := range strings.Split(plain(m), "\n") {
		side := ansi.Cut(l, 0, listPanelWidthFor(m.width))
		for _, e := range []emblem{emblemLargeHead, emblemCompactHead} {
			if strings.Contains(side, e.rows[len(e.rows)/2]) {
				return true
			}
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

// The emblem takes the logo's colors in every theme: its primary color
// (logo) for the head's frame, the headphones, SIGNAL and the bars, its
// secondary one (logoAlt) for NU11 and the lower LED rows.
func TestEmblemTakesTheThemeColors(t *testing.T) {
	for _, tt := range []struct{ theme, logo, logoAlt string }{
		{"NIGHT CITY", "255;95;87", "94;246;255"},
		{"BLUE", "52;122;255", "92;225;255"},
		{"MATRIX", "0;200;50", "0;255;65"},
		{"ROSE", "240;149;200", "255;177;221"},
		{"NEON ROSE", "244;56;136", "255;79;154"},
	} {
		t.Run(tt.theme, func(t *testing.T) {
			m := bootModel(t, newClock(), 80, 24, false)
			useTheme(t, tt.theme)
			screen := m.render()
			frame, nu11 := stLogo.Render("⣿⣿⣿⠠⢀⢀"), stLogoAlt.Render("⣿⣆⢿⢸⡇⢸⡇⠚⣿")
			if !strings.Contains(frame, tt.logo) || !strings.Contains(nu11, tt.logoAlt) {
				t.Fatalf("styles %q %q lack the theme's logo colors", frame, nu11)
			}
			if want := frame + "    " + stLogo.Render("⡀⡀⠄⣿⣿⣿") + " " + nu11; !strings.Contains(screen, want) {
				t.Fatalf("the fourth emblem row is not frame then NU11 colored:\n%s", screen)
			}
			if !strings.Contains(screen, stLogoAlt.Render("⠨⢐⢐⠨")) {
				t.Fatalf("the lower LEDs are not logoAlt colored:\n%s", screen)
			}
		})
	}
}

// The splash scrambles in on the first frame, its text only (the
// emblem's Braille cells, its wordmark and bars included, are not text
// cells), and the normal UI scrambles in when the boot ends.
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
			if x < len(cells) && brailleRune(cells[x]) {
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

// The emblem's glitch noise (boot, shutdown, idle, swap) draws no
// letters or digits: it is dense Braille cells only, like the art it
// corrupts, as many cells a frame as a periodic burst corrupts.
func TestEmblemNoiseIsBrailleOnly(t *testing.T) {
	if len(emblemNoiseGlyphs) < 4 {
		t.Fatalf("emblem noise %q; want a few glyphs", emblemNoiseGlyphs)
	}
	for _, g := range emblemNoiseGlyphs {
		rs := []rune(g)
		if len(rs) != 1 || !brailleRune(rs[0]) || unicode.IsLetter(rs[0]) || unicode.IsDigit(rs[0]) || ansi.StringWidth(g) != 1 {
			t.Fatalf("emblem noise glyph %q is not one Braille cell", g)
		}
	}
	// A noise cell never draws the glyph it lands on, so it always shows.
	for _, under := range emblemNoiseGlyphs {
		for h := range uint64(64) {
			if g := emblemNoise(h, []rune(under)[0]); g == under {
				t.Fatalf("emblemNoise(%d, %q) drew the cell it covers", h, under)
			}
		}
	}
	if bootNoiseMin != 4 || bootNoiseSpan != 7 {
		t.Fatalf("boot noise %d..%d cells; want a burst's 4..10", bootNoiseMin, bootNoiseMin+bootNoiseSpan-1)
	}
}

// emblemNoiseOver counts the cells of lines that show an emblem noise
// glyph base does not show there.
func emblemNoiseOver(lines, base []string) int {
	n := 0
	for y := range lines {
		got, was := cells(ansi.Strip(lines[y])), cells(ansi.Strip(base[y]))
		for x, c := range got {
			if slices.Contains(emblemNoiseGlyphs, c) && (x >= len(was) || was[x] != c) {
				n++
			}
		}
	}
	return n
}
