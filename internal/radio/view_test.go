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

	"soulking/internal/playback/playbacktest"
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
			if sz.w >= 40 && sz.h >= 10 && !strings.Contains(out, "SOUL KING") {
				t.Errorf("view lacks SOUL KING:\n%s", out)
			}
			if sz.w >= 60 && sz.h >= 16 && !strings.Contains(out, "NOW PLAYING") {
				t.Errorf("view lacks NOW PLAYING:\n%s", out)
			}
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
	got := ansi.Strip(m.View().Content)

	path := filepath.Join("testdata", "view_80x24.golden")
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
