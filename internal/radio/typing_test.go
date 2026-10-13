package radio

import (
	tea "charm.land/bubbletea/v2"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
	"reflect"
	"testing"
)

func TestTypingNotificationsSearchDeduplicated(t *testing.T) {
	var got []bool
	m := New(&playbacktest.Fake{}, Options{SkipBoot: true, OnTypingChanged: func(v bool) { got = append(got, v) }})
	m.auth = authOK
	m, _ = press(t, m, "/")
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = press(t, m, "up", "down", "a", "esc")
	if !reflect.DeepEqual(got, []bool{true, false, true, false}) {
		t.Fatalf("typing = %v", got)
	}
}

func TestTypingNotificationsNameAndModal(t *testing.T) {
	var got []bool
	m := New(&playbacktest.Fake{}, Options{SkipBoot: true, OnTypingChanged: func(v bool) { got = append(got, v) }})
	m.auth = authOK
	m.editor.mode = editName
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = press(t, m, "ctrl+c", "esc")
	m, _ = press(t, m, "esc")
	if !reflect.DeepEqual(got, []bool{true, false, true, false}) {
		t.Fatalf("typing = %v", got)
	}
}
