package main

import (
	"errors"
	"strings"
	"testing"
)

func TestAndroidTVRunUsesAppRuntime(t *testing.T) {
	e := newTestEnv(t)
	e.d.goos = "android"
	calls := 0
	e.d.playAndroidTV = func(socket string, calm bool) error {
		calls++
		if socket != "/private/app.sock" || !calm {
			t.Fatalf("mode = %q, calm=%v", socket, calm)
		}
		return nil
	}
	if code := run([]string{"--android-tv", "/private/app.sock", "--calm"}, e.d); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if calls != 1 || e.local != nil || e.uiRuns != 0 {
		t.Fatalf("calls=%d, local=%v, desktop UI=%d", calls, e.local, e.uiRuns)
	}
}

func TestAndroidTVRunReportsStartupError(t *testing.T) {
	e := newTestEnv(t)
	e.d.goos = "android"
	e.d.playAndroidTV = func(string, bool) error { return errors.New("app did not connect") }
	if code := run([]string{"--android-tv", "/private/app.sock"}, e.d); code != 1 || !strings.Contains(e.stderr.String(), "app did not connect") {
		t.Fatalf("exit=%d, stderr=%q", code, e.stderr.String())
	}
}

func TestAndroidTVRejectsConflictingFlags(t *testing.T) {
	for _, flag := range []string{"--demo", "--local", "--apple-music-login", "--version"} {
		t.Run(flag, func(t *testing.T) {
			e := newTestEnv(t)
			e.d.goos = "android"
			if code := run([]string{"--android-tv", "/private/app.sock", flag}, e.d); code != 2 || !strings.Contains(e.stderr.String(), "cannot be combined") {
				t.Fatalf("exit=%d, stderr=%q", code, e.stderr.String())
			}
		})
	}
}

func TestAndroidTVRejectsEmptySocket(t *testing.T) {
	e := newTestEnv(t)
	e.d.goos = "android"
	if code := run([]string{"--android-tv", ""}, e.d); code != 2 || !strings.Contains(e.stderr.String(), "nonempty socket") {
		t.Fatalf("exit=%d, stderr=%q", code, e.stderr.String())
	}
}
