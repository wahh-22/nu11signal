package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/config"
	"github.com/wahh-22/nu11signal/internal/helper"
	"github.com/wahh-22/nu11signal/internal/history"
	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/demo"
	"github.com/wahh-22/nu11signal/internal/update"
)

// fakePlayer is a helper-backed player stand-in that records Close calls.
// Calling any other Player method panics (nil embedded interface).
type fakePlayer struct {
	playback.Player
	closed int
}

func (p *fakePlayer) Close() error {
	p.closed++
	return nil
}

// testEnv wires run to fakes; each test overrides only what it exercises.
type testEnv struct {
	stdout, stderr bytes.Buffer
	d              deps
	uiPlayer       playback.Player
	uiRecents      history.Recents
	uiConfig       config.Source
	uiCalm         bool
	uiUpdates      update.Checker
	uiRuns         int
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{}
	e.d = deps{
		stdout: &e.stdout,
		stderr: &e.stderr,
		locateHelper: func() (string, error) {
			t.Error("locateHelper called unexpectedly")
			return "", errors.New("unexpected locate")
		},
		startHelper: func(context.Context, string) (playback.Player, error) {
			t.Error("startHelper called unexpectedly")
			return nil, errors.New("unexpected start")
		},
		runUI: func(p playback.Player, r history.Recents, cfg config.Source, calm bool, updates update.Checker) error {
			e.uiRuns++
			e.uiPlayer, e.uiRecents, e.uiConfig, e.uiCalm, e.uiUpdates = p, r, cfg, calm, updates
			return nil
		},
	}
	return e
}

func TestRunVersionPrintsVersionAndExitsZero(t *testing.T) {
	e := newTestEnv(t)
	if code := run([]string{"--version"}, e.d); code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	if got, want := e.stdout.String(), version+"\n"; got != want {
		t.Fatalf("stdout = %q; want %q", got, want)
	}
	if e.uiRuns != 0 {
		t.Fatal("--version started the UI")
	}
}

func TestRunHelpExitsZeroWithUsage(t *testing.T) {
	e := newTestEnv(t)
	if code := run([]string{"-h"}, e.d); code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	if !strings.Contains(e.stderr.String(), "-demo") {
		t.Fatalf("stderr = %q; want usage listing -demo", e.stderr.String())
	}
}

func TestRunUnknownFlagExitsTwo(t *testing.T) {
	e := newTestEnv(t)
	if code := run([]string{"--nope"}, e.d); code != 2 {
		t.Fatalf("exit code = %d; want 2", code)
	}
	if !strings.Contains(e.stderr.String(), "flag provided but not defined: -nope") {
		t.Fatalf("stderr = %q; want the flag error", e.stderr.String())
	}
	if e.uiRuns != 0 {
		t.Fatal("a flag error started the UI")
	}
}

func TestRunHelperNotFoundExitsOne(t *testing.T) {
	e := newTestEnv(t)
	e.d.locateHelper = func() (string, error) {
		return "", fmt.Errorf("%w: set NU11SIGNAL_HELPER or build the helper", helper.ErrHelperNotFound)
	}
	if code := run(nil, e.d); code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	want := "nu11signal: nu11signal-helper not found: set NU11SIGNAL_HELPER or build the helper\n"
	if got := e.stderr.String(); got != want {
		t.Fatalf("stderr = %q; want %q", got, want)
	}
	if e.uiRuns != 0 {
		t.Fatal("the UI started without a helper")
	}
}

func TestRunHelperStartFailureExitsOne(t *testing.T) {
	e := newTestEnv(t)
	e.d.locateHelper = func() (string, error) { return "/opt/helper", nil }
	var gotPath string
	e.d.startHelper = func(_ context.Context, path string) (playback.Player, error) {
		gotPath = path
		return nil, errors.New("helper exited")
	}
	if code := run(nil, e.d); code != 1 {
		t.Fatalf("exit code = %d; want 1", code)
	}
	if gotPath != "/opt/helper" {
		t.Fatalf("startHelper path = %q; want the located one", gotPath)
	}
	if got, want := e.stderr.String(), "nu11signal: start helper: helper exited\n"; got != want {
		t.Fatalf("stderr = %q; want %q", got, want)
	}
}

func TestRunPlaysThroughHelperAndClosesIt(t *testing.T) {
	// The recent-searches file lives in the user's config directory; point
	// it at a temporary home so the test never depends on the host's.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	e := newTestEnv(t)
	player := &fakePlayer{}
	e.d.locateHelper = func() (string, error) { return "/opt/helper", nil }
	e.d.startHelper = func(context.Context, string) (playback.Player, error) { return player, nil }
	if code := run(nil, e.d); code != 0 {
		t.Fatalf("exit code = %d; want 0 (stderr %q)", code, e.stderr.String())
	}
	if e.uiPlayer != playback.Player(player) {
		t.Fatalf("UI got player %T; want the helper player", e.uiPlayer)
	}
	if player.closed != 1 {
		t.Fatalf("player closed %d times; want 1", player.closed)
	}
	if _, ok := e.uiRecents.(*history.File); !ok {
		t.Fatalf("UI got recents %T; want the recent-searches file", e.uiRecents)
	}
	if _, ok := e.uiConfig.(*config.File); !ok {
		t.Fatalf("UI got settings %T; want the settings file", e.uiConfig)
	}
}

func TestRunDemoUsesSimulatedPlayerWithoutHelper(t *testing.T) {
	e := newTestEnv(t) // locateHelper and startHelper fail the test if called
	if code := run([]string{"--demo"}, e.d); code != 0 {
		t.Fatalf("exit code = %d; want 0 (stderr %q)", code, e.stderr.String())
	}
	if _, ok := e.uiPlayer.(*demo.Player); !ok {
		t.Fatalf("UI got player %T; want *demo.Player", e.uiPlayer)
	}
	// The demo keeps recent searches in memory, off the user's config.
	if e.uiRecents != nil {
		t.Fatalf("UI got recents %T; want none (in-memory fallback)", e.uiRecents)
	}
	// It reads the settings, though: they only choose how it looks.
	if _, ok := e.uiConfig.(*config.File); !ok {
		t.Fatalf("UI got settings %T; want the settings file", e.uiConfig)
	}
}

func TestRunUIExitPaths(t *testing.T) {
	tests := []struct {
		name       string
		uiErr      error
		wantCode   int
		wantStderr string
	}{
		{"interrupt is a clean exit", tea.ErrInterrupted, 0, ""},
		{"UI failure exits one", errors.New("no tty"), 1, "nu11signal: no tty\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t)
			e.d.runUI = func(playback.Player, history.Recents, config.Source, bool, update.Checker) error { return tt.uiErr }
			if code := run([]string{"--demo"}, e.d); code != tt.wantCode {
				t.Fatalf("exit code = %d; want %d", code, tt.wantCode)
			}
			if got := e.stderr.String(); got != tt.wantStderr {
				t.Fatalf("stderr = %q; want %q", got, tt.wantStderr)
			}
		})
	}
}

func TestRunCalmStartsTheEffectsOff(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want bool
	}{
		{"effects on by default", []string{"--demo"}, "", false},
		{"calm flag", []string{"--demo", "--calm"}, "", true},
		{"calm environment", []string{"--demo"}, "1", true},
		{"environment other than 1", []string{"--demo"}, "0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NU11SIGNAL_CALM", tt.env)
			e := newTestEnv(t)
			if code := run(tt.args, e.d); code != 0 {
				t.Fatalf("exit code = %d; want 0 (stderr %q)", code, e.stderr.String())
			}
			if e.uiCalm != tt.want {
				t.Fatalf("UI calm = %v; want %v", e.uiCalm, tt.want)
			}
		})
	}
}

// updateEnv is a helper-backed run with version v, a temporary home and
// NU11SIGNAL_NO_UPDATE_CHECK set to noCheck; settings, when not empty,
// is written as the settings file first.
func updateEnv(t *testing.T, v, noCheck, settings string) *testEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv(noUpdateCheckEnv, noCheck)
	old := version
	version = v
	t.Cleanup(func() { version = old })
	if settings != "" {
		path, err := config.DefaultPath()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e := newTestEnv(t)
	e.d.locateHelper = func() (string, error) { return "/opt/helper", nil }
	e.d.startHelper = func(context.Context, string) (playback.Player, error) { return &fakePlayer{}, nil }
	return e
}

func TestRunPassesACachedGitHubChecker(t *testing.T) {
	e := updateEnv(t, "0.3.0", "", "")
	if code := run(nil, e.d); code != 0 {
		t.Fatalf("exit code = %d (stderr %q)", code, e.stderr.String())
	}
	c, ok := e.uiUpdates.(*update.Cached)
	if !ok {
		t.Fatalf("UI got updates %T; want *update.Cached", e.uiUpdates)
	}
	cfg, _ := config.DefaultPath()
	if want := filepath.Join(filepath.Dir(cfg), "update.json"); c.Path != want {
		t.Fatalf("cache path = %q; want %q", c.Path, want)
	}
	g, ok := c.Inner.(*update.GitHub)
	if !ok || g.UserAgent != "nu11signal/0.3.0" {
		t.Fatalf("inner checker = %#v; want GitHub for nu11signal/0.3.0", c.Inner)
	}
}

func TestRunUpdateCheckOptOuts(t *testing.T) {
	tests := []struct {
		name, version, env, settings string
		args                         []string
	}{
		{"dev build", "dev", "", "", nil},
		{"unparsable version", "nightly", "", "", nil},
		{"environment", "0.3.0", "1", "", nil},
		{"settings file", "0.3.0", "", `{"update_check": false}`, nil},
		{"demo", "0.3.0", "", "", []string{"--demo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := updateEnv(t, tt.version, tt.env, tt.settings)
			if code := run(tt.args, e.d); code != 0 {
				t.Fatalf("exit code = %d (stderr %q)", code, e.stderr.String())
			}
			if e.uiUpdates != nil {
				t.Fatalf("UI got updates %T; want none", e.uiUpdates)
			}
		})
	}
	// An environment value other than 1 leaves the check on.
	e := updateEnv(t, "0.3.0", "0", `{"update_check": true}`)
	if run(nil, e.d); e.uiUpdates == nil {
		t.Fatal("NU11SIGNAL_NO_UPDATE_CHECK=0 with update_check true turned the check off")
	}
}
