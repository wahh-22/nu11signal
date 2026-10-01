package radio

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// linkingModel is a model still linking Apple Music access (no authMsg
// yet) at w x h.
func linkingModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := newModel(t, playbacktest.New(), newClock())
	m, _ = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	if m.auth != authPending {
		t.Fatalf("auth %v; want still linking", m.auth)
	}
	return m
}

// linkingLine is the line under the emblem, spaced out.
const linkingLine = "L I N K I N G   A P P L E   M U S I C . . ."

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

// While linking, the body is the large emblem with its text, and the
// LINKING line under it; the panels are not drawn.
func TestLinkingShowsTheEmblemAndTheLinkingLine(t *testing.T) {
	m := linkingModel(t, 80, 24)
	screen := plain(m)
	for _, want := range append(slices.Clone(emblemLarge.rows), "N U 1 1", "S I G N A L", "◢◤◢◤◢◤◢◤◢◤", linkingLine, "LINKING") {
		if !strings.Contains(screen, strings.TrimRight(want, " ")) {
			t.Errorf("linking screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "NOW PLAYING") || strings.Contains(screen, "PLAYLISTS ─") {
		t.Fatalf("linking screen still draws the panels:\n%s", screen)
	}
	lines := strings.Split(screen, "\n")
	if len(lines) != 24 || !strings.HasPrefix(lines[1], "◢◤◢◤ ") || !strings.HasPrefix(lines[22], "◢◤◢◤ ") {
		t.Fatalf("header, nav, status or footer moved:\n%s", screen)
	}
}

// The block (emblem, gap and text) is centered on the width as a whole,
// and so is the LINKING line, each on its own; together they are
// centered on the body's height.
func TestLinkingCentersTheBlockAndTheLine(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			lines := strings.Split(plain(linkingModel(t, sz.w, sz.h)), "\n")
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
			if want := strings.Repeat(" ", (sz.w-ansi.StringWidth(linkingLine))/2) + linkingLine; line != want {
				t.Fatalf("LINKING line reads %q; want %q", line, want)
			}
		})
	}
}

// Where the large block does not fit the compact one takes its place.
func TestLinkingFallsBackToTheCompactEmblem(t *testing.T) {
	screen := plain(linkingModel(t, 40, 10))
	for _, row := range emblemCompact.rows {
		if !strings.Contains(screen, strings.TrimRight(row, " ")) {
			t.Fatalf("compact linking screen lacks %q:\n%s", row, screen)
		}
	}
	if strings.Contains(screen, strings.TrimSpace(emblemLarge.rows[1])) {
		t.Fatalf("compact linking screen draws the large emblem:\n%s", screen)
	}
	if !strings.Contains(screen, "S I G N A L") || !strings.Contains(screen, "LINKING APPLE MUSIC...") {
		t.Fatalf("compact linking screen lacks its text:\n%s", screen)
	}
}

// Where neither emblem fits only the LINKING line is left.
func TestLinkingFallsBackToTheTextAlone(t *testing.T) {
	lines := strings.Split(plain(linkingModel(t, 40, 7)), "\n")
	body := strings.Join(lines[2:len(lines)-2], "\n")
	if strings.ContainsAny(body, "▄▀█") || strings.Contains(body, "S I G N A L") {
		t.Fatalf("text-only linking body draws the emblem:\n%s", body)
	}
	if !strings.Contains(body, "LINKING APPLE MUSIC...") {
		t.Fatalf("text-only linking body lacks the LINKING line:\n%s", body)
	}
}

// Once access is granted the normal view returns.
func TestAuthorizedViewDropsTheEmblem(t *testing.T) {
	screen := plain(loaded(t, playbacktest.New(), newClock()))
	if strings.Contains(screen, linkingLine) || strings.Contains(screen, "S I G N A L") {
		t.Fatalf("authorized view still shows the linking screen:\n%s", screen)
	}
	if !strings.Contains(screen, "NOW PLAYING") {
		t.Fatalf("authorized view lacks the panels:\n%s", screen)
	}
}

// The splash body is not clickable: every zone is in the header.
func TestLinkingZonesAreTheHeaderOnly(t *testing.T) {
	_, zs := linkingModel(t, 80, 24).baseLayout()
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
			m := linkingModel(t, 80, 24)
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

// The splash scrambles in at startup, its text only (the emblem's block
// glyphs are not text cells), like any other content.
func TestLinkingScramblesIn(t *testing.T) {
	c := newClock()
	m := New(playbacktest.New(), Options{Now: c.now, Seed: 2077, Effects: true})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if !m.introAnimating() {
		t.Fatal("the splash did not intro")
	}
	base, _ := m.baseLayout()
	lines := strings.Split(ansi.Strip(strings.Join(base, "\n")), "\n")
	y := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, linkingLine) })
	if len(m.intro.cells[y]) == 0 {
		t.Fatalf("the LINKING line has no intro cells: %v", m.intro.cells)
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
	// Access arrives: the content replacing the splash scrambles in.
	seq := m.intro.seq
	m, _ = step(t, m, run(t, m.authorizeCmd()))
	if m.intro.seq == seq {
		t.Fatal("the content replacing the splash did not intro")
	}
}

func TestLinkingGolden80x24(t *testing.T) {
	assertGolden(t, "linking_80x24.golden", plain(linkingModel(t, 80, 24)))
}

func TestLinkingBlueGolden80x24(t *testing.T) {
	m := linkingModel(t, 80, 24)
	useTheme(t, "BLUE")
	assertGolden(t, "linking_blue_80x24.golden", m.View().Content)
}

func TestSplashSurvivesNoRoom(t *testing.T) {
	// A body with no rows left (a terminal at the full layout's edge)
	// draws nothing instead of panicking.
	for _, h := range []int{-3, 0, 1} {
		if got := splash(80, h); len(got) > max(h, 1) {
			t.Fatalf("splash(80, %d) drew %d lines", h, len(got))
		}
	}
}
