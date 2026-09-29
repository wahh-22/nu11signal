package radio

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestViewFitsEverySize(t *testing.T) {
	sizes := []struct{ w, h int }{{0, 0}, {1, 1}, {10, 3}, {40, 10}, {59, 15}, {80, 24}, {100, 30}, {160, 50}}
	for _, sz := range sizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			f := playbacktest.New()
			m := loaded(t, f, newClock())
			m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
			m, _ = step(t, m, tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
			m, _ = step(t, m, tickMsg{gen: m.tickGen})

			v := m.View()
			out := ansi.Strip(v.Content)
			lines := strings.Split(out, "\n")
			if sz.h > 0 && len(lines) > sz.h {
				t.Errorf("rendered %d lines, want at most %d", len(lines), sz.h)
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > sz.w && sz.w > 0 {
					t.Errorf("line %d is %d cells wide, want at most %d: %q", i, w, sz.w, l)
				}
			}
			if sz.w >= 40 && sz.h >= 10 && !strings.Contains(out, "NU11SIGNAL") {
				t.Errorf("view lacks NU11SIGNAL:\n%s", out)
			}
			if sz.w >= 60 && sz.h >= 16 && !strings.Contains(out, "NOW PLAYING") {
				t.Errorf("view lacks NOW PLAYING:\n%s", out)
			}
		})
	}
}

func TestSearchViewFitsEverySize(t *testing.T) {
	sizes := []struct{ w, h int }{{1, 1}, {10, 3}, {30, 8}, {40, 10}, {59, 15}, {80, 24}, {160, 50}}
	for _, sz := range sizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			f := playbacktest.New()
			f.SearchCatalogResult = catalog()
			// Wide characters take two cells each.
			f.SearchCatalogResult.Artists = append(f.SearchCatalogResult.Artists, playback.Artist{ID: "a2", Name: "宇多田ヒカル", Genres: []string{"J-Pop"}})
			f.SearchCatalogResult.Songs = append(f.SearchCatalogResult.Songs, playback.Song{ID: "s9", Title: "初恋", Artist: "宇多田ヒカル"})
			m := searchFor(t, loaded(t, f, newClock()), "daft")
			m, _ = press(t, m, "down", "down", "down", "down")
			m, _ = step(t, m, tea.WindowSizeMsg{Width: sz.w, Height: sz.h})

			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			if len(lines) > sz.h {
				t.Errorf("rendered %d lines, want at most %d", len(lines), sz.h)
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > sz.w {
					t.Errorf("line %d is %d cells wide, want at most %d: %q", i, w, sz.w, l)
				}
			}
		})
	}
}

func TestSearchRowsFillWidthWithWideCharacters(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = playback.SearchResults{
		Suggestions: []string{"宇多田"},
		Artists:     []playback.Artist{{ID: "a2", Name: "宇多田ヒカル", Genres: []string{"J-Pop"}}},
		Songs:       []playback.Song{{ID: "s9", Title: "初恋", Artist: "宇多田ヒカル"}},
	}
	m := searchFor(t, loaded(t, f, newClock()), "宇多")
	for _, w := range []int{9, 20, 41} {
		for cur := -1; cur < 3; cur++ {
			m.stack[len(m.stack)-1].cursor = cur
			for i, row := range m.searchBody(w, 8) {
				if got := ansi.StringWidth(row); got != w {
					t.Errorf("w=%d cursor=%d: row %d is %d cells: %q", w, cur, i, got, ansi.Strip(row))
				}
			}
		}
	}
}

func TestSearchViewGolden80x24(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, m Model) Model
	}{
		{"search_recent", func(t *testing.T, m Model) Model {
			m, _ = press(t, m, "/")
			return m
		}},
		{"search_results", func(t *testing.T, m Model) Model {
			m = searchFor(t, m, "daft")
			m, _ = press(t, m, "down", "down", "down")
			return m
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			f.SearchCatalogResult = catalog()
			r := &fakeRecents{terms: []string{"queen", "daft punk", "samurai"}}
			m := loadedWithRecents(t, f, r)
			m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
			for i := 0; i < 12; i++ {
				m, _ = step(t, m, tickMsg{gen: m.tickGen})
			}
			m = tt.setup(t, m)
			assertGolden(t, tt.name+"_80x24.golden", ansi.Strip(m.View().Content))
		})
	}
}

func TestViewGolden80x24(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, _ = step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	for i := 0; i < 12; i++ {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	assertGolden(t, "view_80x24.golden", ansi.Strip(m.View().Content))
}

// assertGolden compares got with testdata/name, rewriting it under -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("view differs from %s (run with -update after checking):\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func TestOnAirStationRowFillsWidth(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m.playingStation = "pl-1"
	for _, w := range []int{12, 30, 47} {
		for _, selected := range []bool{true, false} {
			row := m.stationRow(0, selected, w)
			if got := ansi.StringWidth(row); got != w {
				t.Errorf("on-air row (w=%d, selected=%v) is %d cells: %q", w, selected, got, ansi.Strip(row))
			}
			if !strings.Contains(row, "◉") {
				t.Errorf("on-air row (w=%d) lost its mark: %q", w, ansi.Strip(row))
			}
		}
	}
}
