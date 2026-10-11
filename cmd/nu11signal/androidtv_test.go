package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/androidtv"
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/composite"
	"github.com/wahh-22/nu11signal/internal/playback/webplayer"
)

func TestAndroidTVTypingSendsKeyboard(t *testing.T) {
	goSide, app := net.Pipe()
	bridge := webplayer.NewBridge(goSide, nil)
	defer bridge.Close()
	defer app.Close()
	app.SetDeadline(time.Now().Add(3 * time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hook := androidTypingHook(ctx, bridge)
	dec := json.NewDecoder(app)
	for _, show := range []bool{true, false} {
		hook(show)
		var msg webplayer.BridgeMessage
		if err := dec.Decode(&msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type != "keyboard" || msg.Show != show {
			t.Fatalf("message = %+v", msg)
		}
	}
}

func TestAndroidTVFlag(t *testing.T) {
	if _, err := parseFlags([]string{"--android-tv", "/private/app.sock"}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestAndroidTVRejectsOtherPlatforms(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			e := newTestEnv(t)
			e.d.goos = goos
			if code := run([]string{"--android-tv", "/private/app.sock"}, e.d); code != 2 {
				t.Fatalf("exit = %d; stderr = %q", code, e.stderr.String())
			}
			if !strings.Contains(e.stderr.String(), "only supported on Android") {
				t.Fatalf("stderr = %q", e.stderr.String())
			}
			if e.uiRuns != 0 {
				t.Fatal("invalid mode started UI")
			}
		})
	}
}

func TestAndroidLoginPointsToApp(t *testing.T) {
	e := newTestEnv(t)
	e.d.goos = "android"
	if code := run([]string{"--apple-music-login"}, e.d); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(e.stderr.String(), "Android TV app") {
		t.Fatalf("stderr = %q", e.stderr.String())
	}
}

func TestAndroidWithoutAppIsLocalOnly(t *testing.T) {
	tempHome(t)
	e := newTestEnv(t)
	e.d.goos = "android"
	if code := run(nil, e.d); code != 0 {
		t.Fatalf("exit = %d; stderr = %q", code, e.stderr.String())
	}
	assertLocalOnly(t, e)
}

func TestAndroidTVBackendSelection(t *testing.T) {
	home := tempHome(t)
	e := newTestEnv(t)
	e.d.goos = "android"
	player := &androidtv.Player{Player: webplayer.New(androidUnauthorizedPage{})}
	defer player.Close()
	bounded := false
	e.d.openWebPlayer = func(ctx context.Context) (playback.Player, error) {
		deadline, ok := ctx.Deadline()
		bounded = ok
		remaining := time.Until(deadline)
		if remaining < webplayer.BridgeLoadTimeout-time.Second || remaining > webplayer.BridgeLoadTimeout {
			t.Errorf("app startup bound = %s; want 2m", remaining)
		}
		return player, nil
	}
	opts, err := parseFlags([]string{"--android-tv", "/private/app.sock"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	got, err := openPlayer(opts, nil, e.d)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	if _, ok := got.(*composite.Player); !ok {
		t.Fatalf("player = %T; want composite app backend", got)
	}
	if status, err := got.Authorize(context.Background()); err != nil || status != playback.AuthAuthorized {
		t.Fatalf("authorization = %v, %v", status, err)
	}
	select {
	case notice := <-got.Errors():
		if !strings.Contains(notice.Error(), "waiting for authorization") || !strings.Contains(notice.Error(), player.AuthorizationHint()) {
			t.Fatalf("notice = %v", notice)
		}
	case <-time.After(time.Second):
		t.Fatal("missing late authorization notice")
	}
	if !bounded {
		t.Fatal("app startup not bounded")
	}
	if e.local == nil || len(e.localDirs) != 1 || e.localDirs[0] != filepath.Join(home, "Music") {
		t.Fatalf("local dirs = %v", e.localDirs)
	}
}

func TestAndroidTVUIReceivesComposite(t *testing.T) {
	tempHome(t)
	e := newTestEnv(t)
	e.d.goos = "android"
	e.d.openWebPlayer = func(context.Context) (playback.Player, error) {
		return &androidtv.Player{Player: webplayer.New(androidUnauthorizedPage{})}, nil
	}
	if err := play(options{androidTV: "/private/app.sock"}, false, e.d); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.uiPlayer.(*composite.Player); !ok || e.uiRuns != 1 {
		t.Fatalf("UI player = %T, runs = %d", e.uiPlayer, e.uiRuns)
	}
}

type androidUnauthorizedPage struct{}

func (androidUnauthorizedPage) Evaluate(_ context.Context, expr string) (json.RawMessage, error) {
	if strings.Contains(expr, "status") && !strings.Contains(expr, "installed") {
		return json.RawMessage(`{"value":{"ready":true,"authorized":false}}`), nil
	}
	return json.RawMessage(`{"installed":true}`), nil
}
