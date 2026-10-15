package radio

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// loadedAt is loaded at a w x h terminal.
func loadedAt(t *testing.T, w, h int) Model {
	t.Helper()
	c := newClock()
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	m := New(f, Options{SkipBoot: true, Now: c.now, Seed: 2077})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	return m
}

// A terminal cell is about twice as tall as it is wide, so a screen that is
// physically portrait has fewer than twice as many columns as rows. That is
// a phone held upright: the panels stack instead of sitting side by side.
// Desktop terminals and landscape screens keep their layouts.
func TestLayoutKindFollowsTheScreenShape(t *testing.T) {
	for _, tt := range []struct {
		w, h int
		want layoutKind
	}{
		{161, 44, layoutFull}, // TV and desktop
		{80, 24, layoutFull},  // classic terminal
		{121, 28, layoutFull}, // phone, landscape
		{57, 60, layoutTall},  // phone, portrait
		{46, 80, layoutTall},  // narrow phone, portrait
		{100, 60, layoutTall}, // tablet, portrait
		{40, 20, layoutCompact},
		{30, 16, layoutCompact}, // too narrow and short to stack panels
		{10, 4, layoutTiny},
	} {
		m := Model{width: tt.w, height: tt.h}
		if got := m.kind(); got != tt.want {
			t.Errorf("%dx%d: kind %v; want %v", tt.w, tt.h, got, tt.want)
		}
	}
}

// Portrait stacks NOW PLAYING, with its spectrum, over the list, each as
// wide as the screen, and fills the height down to the status and hints.
func TestTallStacksNowPlayingOverTheList(t *testing.T) {
	const w, h = 56, 60
	m := loadedAt(t, w, h)
	lines := strings.Split(plain(m), "\n")
	if len(lines) != h {
		t.Fatalf("%d lines; want %d", len(lines), h)
	}
	player, list := -1, -1
	for i, l := range lines {
		if player < 0 && strings.Contains(l, "NOW PLAYING") {
			player = i
		}
		if list < 0 && strings.Contains(l, "PLAYLISTS ─") {
			list = i
		}
	}
	if player < 0 || list < 0 || player >= list {
		t.Fatalf("NOW PLAYING at row %d, list at row %d; want the player above the list:\n%s", player, list, plain(m))
	}
	for _, row := range []int{player, list} {
		if got := ansi.StringWidth(strings.TrimRight(lines[row], " ")); got != w {
			t.Errorf("panel title row %d is %d wide; want the full %d:\n%s", row, got, w, lines[row])
		}
	}
	if !strings.ContainsAny(strings.Join(lines[player:list], "\n"), "⣿⣶⣴⣠") {
		t.Errorf("no emblem or spectrum in NOW PLAYING:\n%s", strings.Join(lines[player:list], "\n"))
	}
	if !strings.Contains(lines[h-1], "[?]") {
		t.Errorf("no hints on the last line: %q", lines[h-1])
	}
	assertGolden(t, "tall_56x60.golden", plain(m))
}

// Clicking works in the stacked layout: list rows sit in the list panel,
// under the player, and a click opens the row as in the full layout.
func TestTallClickOpensAListRow(t *testing.T) {
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	m := New(f, Options{SkipBoot: true, Now: newClock().now, Seed: 2077})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 56, Height: 60})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	z := zoneOf(t, m, rowZone(1))
	player, list := zoneOf(t, m, zonePanelPlayer), zoneOf(t, m, zonePanelList)
	if player.y >= list.y || z.y <= list.y {
		t.Fatalf("player panel at y=%d, list panel at y=%d, row at y=%d; want the row inside the list, under the player", player.y, list.y, z.y)
	}
	if got := textAt(m, z); !strings.Contains(got, "FUSEWAY") {
		t.Fatalf("row 1 zone covers %q; want the FUSEWAY row", got)
	}
	m, cmd = click(t, m, rowZone(1))
	if m.stationCursor() != 1 {
		t.Fatalf("station cursor = %d; want the clicked row", m.stationCursor())
	}
	settle(t, m, cmd)
	assertCall(t, f, "LibraryPlaylist", "pl-2")
}
