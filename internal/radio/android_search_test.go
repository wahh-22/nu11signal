package radio

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

func TestTypingNotificationsResultCursor(t *testing.T) {
	var got []bool
	m := New(&playbacktest.Fake{}, Options{SkipBoot: true, OnTypingChanged: func(v bool) { got = append(got, v) }})
	m.auth = authOK
	m.input.SetValue("queen")
	m.search = searchState{term: "queen", results: catalog()}
	m.push(frame{kind: viewSearch, cursor: -1})
	m.input.Focus()
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = press(t, m, "down", "down", "up", "up")
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if !reflect.DeepEqual(got, []bool{true, false, true}) {
		t.Fatalf("typing transitions = %v", got)
	}
	if !m.input.Focused() {
		t.Fatal("desktop text input focus changed")
	}
}

type searchDeadlinePlayer struct {
	*playbacktest.Fake
	left time.Duration
}

func (p *searchDeadlinePlayer) SearchCatalog(ctx context.Context, term string, limit int) (playback.SearchResults, error) {
	deadline, _ := ctx.Deadline()
	p.left = time.Until(deadline)
	return playback.SearchResults{}, nil
}

func TestSearchAndroidTimingAndSupersession(t *testing.T) {
	p := &searchDeadlinePlayer{Fake: playbacktest.New()}
	m := New(p, Options{SkipBoot: true, SearchTimeout: 45 * time.Second, SearchDebounce: 750 * time.Millisecond})
	m.startSearch("queen")()
	if p.left < 44*time.Second || p.left > 45*time.Second {
		t.Fatalf("search deadline = %s", p.left)
	}
	m.input.SetValue("abba")
	start := time.Now()
	m.inputChanged()()
	if time.Since(start) < 700*time.Millisecond {
		t.Fatal("Android debounce was ignored")
	}
	m = New(p, Options{SkipBoot: true})
	m.startSearch("queen")()
	if p.left < 7*time.Second || p.left > 8*time.Second || m.searchDelay != 250*time.Millisecond {
		t.Fatalf("desktop timing changed: %s, %s", p.left, m.searchDelay)
	}
}

type bridgeSearchPlayer struct {
	*playbacktest.Fake
	bridge *webplayer.Bridge
}

func (p *bridgeSearchPlayer) SearchCatalog(ctx context.Context, term string, limit int) (playback.SearchResults, error) {
	_, err := p.bridge.Evaluate(ctx, "slow()")
	return playback.SearchResults{}, err
}

func TestSearchSupersessionCancelsBridge(t *testing.T) {
	goSide, app := net.Pipe()
	b := webplayer.NewBridge(goSide, nil)
	defer b.Close()
	defer app.Close()
	app.SetDeadline(time.Now().Add(3 * time.Second))
	m := New(&bridgeSearchPlayer{Fake: playbacktest.New(), bridge: b}, Options{SkipBoot: true})
	cmd := m.startSearch("queen")
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	dec := json.NewDecoder(app)
	var request webplayer.BridgeMessage
	if err := dec.Decode(&request); err != nil {
		t.Fatal(err)
	}
	m.input.SetValue("abba")
	m.inputChanged()
	msg := (<-done).(catalogMsg)
	if !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("superseded search = %v", msg.err)
	}
	var cancel webplayer.BridgeMessage
	if err := dec.Decode(&cancel); err != nil {
		t.Fatal(err)
	}
	if cancel.Type != "cancel" || cancel.ID != request.ID {
		t.Fatalf("cancel = %+v; request = %+v", cancel, request)
	}
	m = m.onCatalog(msg)
	if m.search.err != nil {
		t.Fatal("superseded error reached UI")
	}
}
