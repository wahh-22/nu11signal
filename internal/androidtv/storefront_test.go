package androidtv

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

type signedInPage struct{ reloads int }

func (p *signedInPage) Evaluate(_ context.Context, expr string) (json.RawMessage, error) {
	if strings.Contains(expr, "location.replace") {
		p.reloads++
	}
	if strings.Contains(expr, "installed") {
		return json.RawMessage(`{"installed":true}`), nil
	}
	return json.RawMessage(`{"value":{"ready":true,"authorized":true}}`), nil
}

func TestAlreadySignedInDoesNotRefreshStorefront(t *testing.T) {
	page := &signedInPage{}
	p := &Player{Player: webplayer.New(page), page: page, hideLogin: func(context.Context, webplayer.BridgeMessage) error {
		t.Error("already signed-in launch changed the sign-in view")
		return nil
	}}
	defer p.Close()
	for range 2 {
		if s, err := p.Authorize(context.Background()); err != nil || s != playback.AuthAuthorized {
			t.Fatalf("Authorize = %s, %v", s, err)
		}
	}
	if page.reloads != 0 {
		t.Fatalf("already signed-in launch reloaded %d times", page.reloads)
	}
}

// Phases: signed out, old signed-in document, navigation failure,
// new document awaiting MusicKit, and ready account document.
func TestLateAuthorizationWaitsForAccountDocument(t *testing.T) {
	goConn, app := net.Pipe()
	defer app.Close()
	r := &Runtime{loginClosed: make(chan struct{})}
	r.Bridge = webplayer.NewBridge(goConn, r.onMessage)
	defer r.Bridge.Close()
	var phase, reloads, hides, shows atomic.Int32
	go func() {
		decoder, encoder := json.NewDecoder(app), json.NewEncoder(app)
		for {
			var m webplayer.BridgeMessage
			if decoder.Decode(&m) != nil {
				return
			}
			if m.Type == "login" {
				if !m.Show {
					hides.Add(1)
				} else {
					shows.Add(1)
				}
				continue
			}
			var value any = map[string]any{"installed": true}
			ok, failure := true, ""
			switch {
			case strings.Contains(m.Expr, "location.replace"):
				reloads.Add(1)
				// Navigation can destroy the evaluation before its reply.
				ok, failure = false, "page navigated"
			case phase.Load() == 2:
				ok, failure = false, "page navigated"
			case strings.Contains(m.Expr, "__nu11StorefrontReload"):
				value = phase.Load() == 1
			case strings.Contains(m.Expr, "status") && !strings.Contains(m.Expr, "installed"):
				value = map[string]any{"value": map[string]any{"ready": phase.Load() != 3, "authorized": phase.Load() != 0}}
			}
			if encoder.Encode(map[string]any{"type": "result", "id": m.ID, "ok": ok, "value": value, "error": failure}) != nil {
				return
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := r.NewPlayer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	check := func(want playback.AuthStatus) {
		t.Helper()
		got, err := p.Authorize(ctx)
		if err != nil || got != want {
			t.Fatalf("phase %d: Authorize = %s, %v; want %s", phase.Load(), got, err, want)
		}
	}
	check(playback.AuthNotDetermined)
	phase.Store(1)
	check(playback.AuthNotDetermined)
	if reloads.Load() != 1 || hides.Load() != 1 {
		t.Fatalf("reloads=%d hides=%d", reloads.Load(), hides.Load())
	}
	// SignIn can finish immediately, but cannot reopen the hidden view
	// if its initial poll raced the composite handoff.
	if s, err := p.loginStatus(ctx); err != nil || s != playback.AuthAuthorized {
		t.Fatalf("login status during refresh = %s, %v", s, err)
	}
	if err := p.sendLogin(ctx, webplayer.BridgeMessage{Type: "login", Show: true}); err != nil {
		t.Fatal(err)
	}
	// An evaluation can still hit the old, fully authorized document.
	check(playback.AuthNotDetermined)
	if shows.Load() != 0 {
		t.Fatal("sign-in reopened during refresh")
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := p.Authorize(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation hidden: %v", err)
	}
	phase.Store(2)
	check(playback.AuthNotDetermined)
	phase.Store(3)
	// More than the device's ~40s of one-second sign-in polls: no timeout
	// may disable the primary while MusicKit initializes.
	for range 45 {
		check(playback.AuthNotDetermined)
	}
	phase.Store(4)
	check(playback.AuthAuthorized)
	check(playback.AuthAuthorized)
	if reloads.Load() != 1 {
		t.Fatalf("reloaded %d times", reloads.Load())
	}
	select {
	case err := <-p.Errors():
		t.Fatalf("transient UI error: %v", err)
	default:
	}
	app.Close()
	if _, err := p.Authorize(ctx); !errors.Is(err, webplayer.ErrBrowserGone) {
		t.Fatalf("disconnect hidden: %v", err)
	}
}
