package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/wahh-22/nu11signal/internal/playback"
)

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
	e := newTestEnv(t)
	e.d.goos = "android"
	player := newFakePlayer()
	defer player.Close()
	bounded := false
	e.d.openWebPlayer = func(ctx context.Context) (playback.Player, error) { _, bounded = ctx.Deadline(); return player, nil }
	opts, err := parseFlags([]string{"--android-tv", "/private/app.sock"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	got, err := openPlayer(opts, nil, e.d)
	if err != nil || got != player {
		t.Fatalf("player = %T, %v; want app backend", got, err)
	}
	if !bounded {
		t.Fatal("app startup not bounded")
	}
	if e.local != nil {
		t.Fatal("Android TV opened local playback")
	}
}
