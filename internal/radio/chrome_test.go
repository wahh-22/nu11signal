package radio

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

func TestTheHeaderIsTheWordmarkAlone(t *testing.T) {
	m := playingModel(t, playbacktest.New())
	if top := ansi.Strip(strings.Split(m.render(), "\n")[0]); strings.Contains(top, "NIGHT CITY") || !strings.Contains(top, "NU11SIGNAL") {
		t.Fatalf("header %q; want the wordmark without NIGHT CITY RADIO", top)
	}
	if v := m.View(); strings.Contains(v.WindowTitle, "NIGHT CITY") {
		t.Fatalf("window title %q still names NIGHT CITY RADIO", v.WindowTitle)
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
