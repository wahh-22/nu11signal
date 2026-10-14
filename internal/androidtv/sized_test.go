package androidtv

import (
	"testing"

	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

func sizedYet(r *Runtime) bool {
	select {
	case <-r.Sized():
		return true
	default:
		return false
	}
}

// The TUI waits for the app's real terminal size before it starts (see
// Sized): only a valid size counts, and later sizes are harmless.
func TestSizedClosesOnTheFirstValidSize(t *testing.T) {
	r := &Runtime{sized: make(chan struct{})}
	if sizedYet(r) {
		t.Fatal("sized before any size arrived")
	}
	for _, bad := range []webplayer.BridgeMessage{
		{Type: "resize", Cols: 0, Rows: 30},
		{Type: "resize", Cols: 100, Rows: 0},
		{Type: "resize", Cols: 70000, Rows: 30},
		{Type: "levels", Bands: []int{1}},
	} {
		r.onMessage(bad)
		if sizedYet(r) {
			t.Fatalf("%+v counted as the terminal size", bad)
		}
	}
	r.onMessage(webplayer.BridgeMessage{Type: "resize", Cols: 161, Rows: 44})
	if !sizedYet(r) {
		t.Fatal("the first valid size did not mark the terminal sized")
	}
	r.onMessage(webplayer.BridgeMessage{Type: "resize", Cols: 120, Rows: 40})
	if !sizedYet(r) {
		t.Fatal("a later size reopened the wait")
	}
}

// A Runtime built without the channel (tests, or code that never waits for
// the size) still handles resizes: the close is skipped, not a panic.
func TestSizedIsOptional(t *testing.T) {
	r := &Runtime{}
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("resize without a sized channel panicked: %v", p)
		}
	}()
	r.onMessage(webplayer.BridgeMessage{Type: "resize", Cols: 161, Rows: 44})
	if r.Sized() != nil {
		t.Fatal("Sized invented a channel")
	}
}
