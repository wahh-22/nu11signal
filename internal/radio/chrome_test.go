package radio

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

func TestTheHeaderIsTheWordmarkAlone(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	if top := ansi.Strip(strings.Split(m.render(), "\n")[0]); strings.Contains(top, "REDSHIFT") || !strings.Contains(top, "NU11SIGNAL") {
		t.Fatalf("header %q; want the wordmark without REDSHIFT RADIO", top)
	}
	if v := m.View(); strings.Contains(v.WindowTitle, "REDSHIFT") {
		t.Fatalf("window title %q still names REDSHIFT RADIO", v.WindowTitle)
	}
}

// The footer's key hints sit in the background, all in the muted color
// (the dim one would be unreadable), never the accent or the label color
// the content uses.
func TestTheKeyHintsSitInTheBackground(t *testing.T) {
	parts, _ := renderHints([]hint{{"Q", "QUIT"}})
	want := stMuted.Render("[Q]") + " " + stMuted.Render("QUIT")
	if parts[0].text != want {
		t.Fatalf("hint %q; want %q", parts[0].text, want)
	}
}

func TestTheWordmarkIsCenteredAndTheNavLeadsWithSlants(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	top, nav := lines[0], lines[1]
	if strings.Contains(top, "◢◤") {
		t.Fatalf("header %q still draws ◢◤ before the wordmark", top)
	}
	at := strings.Index(top, "NU11SIGNAL")
	if want := (m.width - len("NU11SIGNAL")) / 2; at < 0 || len([]rune(top[:at])) != want {
		t.Fatalf("NU11SIGNAL at column %d in %q; want centered at %d", len([]rune(top[:max(at, 0)])), top, want)
	}
	if !strings.HasPrefix(nav, "◢◤◢◤ ") {
		t.Fatalf("nav %q; want it to lead with ◢◤◢◤", nav)
	}
}
