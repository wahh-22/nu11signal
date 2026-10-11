package radio

import (
	"testing"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

func TestSearchSlashRefocusesResults(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, "/")
	m.input.SetValue("andres")
	m.search = searchState{term: "andres", results: catalog()}
	m.setCursor(0)
	seq := m.search.seq
	m, _ = press(t, m, "/")
	if m.input.Value() != "andres" || m.cursor() != -1 || !m.textInputSelected() || m.search.seq != seq {
		t.Fatalf("slash: value=%q cursor=%d selected=%v seq=%d (was %d)", m.input.Value(), m.cursor(), m.textInputSelected(), m.search.seq, seq)
	}
	// Desktop users can still type a slash while editing the input.
	m, _ = press(t, m, "/")
	if m.input.Value() != "andres/" {
		t.Fatalf("input slash = %q", m.input.Value())
	}
}

func TestRemoteMenuFocusesParkedSearch(t *testing.T) {
	for _, context := range []string{"fresh", "parked row", "parked page"} {
		t.Run(context, func(t *testing.T) {
			m := loaded(t, playbacktest.New(), newClock())
			if context != "fresh" {
				m, _ = press(t, m, "/")
				m.input.SetValue("andres")
				m.search = searchState{term: "andres", results: catalog()}
				m.setCursor(0)
				if context == "parked page" {
					m.input.Blur()
					m.push(frame{kind: viewResults})
				}
				m, _ = press(t, m, "tab")
			}
			term := m.input.Value()
			m, _ = press(t, m, "ctrl+_")
			if !m.textInputSelected() || m.input.Value() != term {
				t.Fatalf("Menu: top=%v selected=%v term=%q (was %q)", m.top().kind, m.textInputSelected(), m.input.Value(), term)
			}
		})
	}
}

func TestRemoteLongBackEscapesTyping(t *testing.T) {
	for _, context := range []string{"search input", "search row", "search player", "search tabs", "playlist name"} {
		t.Run(context, func(t *testing.T) {
			m := loaded(t, playbacktest.New(), newClock())
			m, _ = press(t, m, "/")
			m.input.SetValue("andres")
			if context == "search row" {
				m.search = searchState{term: "andres", results: catalog()}
				m.setCursor(0)
			}
			if context == "search player" {
				m.focusPlayer(ctlPlay)
			}
			if context == "search tabs" {
				m.focusTabs()
			}
			if context == "playlist name" {
				m.editor.mode = editName
				m.nameInput.SetValue("mix")
				m.nameInput.Focus()
			}
			want, _ := press(t, m, "esc")
			got, _ := press(t, m, "ctrl+q")
			if got.quitAsk || got.top().kind != want.top().kind || got.editor.mode != want.editor.mode || got.focus != want.focus || got.input.Value() != "andres" || got.nameInput.Value() != want.nameInput.Value() {
				t.Fatalf("long Back did not act as Escape: view=%v editor=%v quit=%v input=%q name=%q", got.top().kind, got.editor.mode, got.quitAsk, got.input.Value(), got.nameInput.Value())
			}
		})
	}
}

func TestRemoteLongBackAsksQuitOutsideTyping(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = press(t, m, "ctrl+q")
	if !m.quitAsk {
		t.Fatal("long Back did not ask to quit")
	}
}
