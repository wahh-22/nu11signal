package radio

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
	updatecheck "github.com/wahh-22/nu11signal/internal/update"
)

// releaseChecker is an updatecheck.Checker answering rel, err and counting
// its calls.
type releaseChecker struct {
	rel   updatecheck.Release
	err   error
	calls atomic.Int32
}

func (c *releaseChecker) Latest(context.Context) (updatecheck.Release, error) {
	c.calls.Add(1)
	return c.rel, c.err
}

var v031 = updatecheck.Release{Version: "0.3.1", URL: "https://github.com/wahh-22/nu11signal/releases/tag/v0.3.1"}

// releaseModel is a loaded model of version 0.3.0 that ran the update
// check against c, with upgrade as the upgrade command.
func releaseModel(t *testing.T, c updatecheck.Checker, upgrade string) Model {
	t.Helper()
	f := playbacktest.New()
	f.PlaylistsResult = stations()
	m := New(f, Options{SkipBoot: true, Now: newClock().now, Seed: 2077, Updates: c, Version: "0.3.0", Upgrade: upgrade})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	m, _ = step(t, m, run(t, m.checkReleaseCmd()))
	return m
}

func TestTheIdleStatusLineAnnouncesANewerRelease(t *testing.T) {
	tests := []struct {
		upgrade, want string
	}{
		{updatecheck.BrewUpgrade, "◢◤◢◤ UPDATE v0.3.1 AVAILABLE // brew upgrade --cask nu11signal"},
		{"", "◢◤◢◤ UPDATE v0.3.1 AVAILABLE // " + v031.URL},
	}
	for _, tt := range tests {
		m := releaseModel(t, &releaseChecker{rel: v031}, tt.upgrade)
		if !strings.Contains(plain(m), tt.want) {
			t.Fatalf("view lacks %q:\n%s", tt.want, plain(m))
		}
		if got, want := m.statusLine(m.width), stHiBold.Render(fit(tt.want, m.width)); got != want {
			t.Fatalf("status line %q; want the buttons' style %q", got, want)
		}
	}
}

func TestTheReleaseNoticeIsCutToFit(t *testing.T) {
	m := releaseModel(t, &releaseChecker{rel: v031}, updatecheck.BrewUpgrade)
	for _, w := range []int{20, 40} {
		if got := ansi.StringWidth(m.statusLine(w)); got > w {
			t.Fatalf("status line at %d cells is %d wide", w, got)
		}
	}
}

func TestAStatusMessageTakesPrecedenceOverTheReleaseNotice(t *testing.T) {
	m := releaseModel(t, &releaseChecker{rel: v031}, updatecheck.BrewUpgrade)
	m.setStatus("queued")
	line := plain(m)
	if !strings.Contains(line, "▲ QUEUED") || strings.Contains(line, "UPDATE v0.3.1 AVAILABLE") {
		t.Fatalf("view while a message shows:\n%s", line)
	}
	m.status = ""
	if !strings.Contains(plain(m), "UPDATE v0.3.1 AVAILABLE") {
		t.Fatalf("the notice did not come back after the message:\n%s", plain(m))
	}
}

func TestNoReleaseNoticeWhenUpToDateOrOnError(t *testing.T) {
	checkers := map[string]*releaseChecker{
		"same version": {rel: updatecheck.Release{Version: "0.3.0", URL: "u"}},
		"older":        {rel: updatecheck.Release{Version: "0.2.9", URL: "u"}},
		"pre-release":  {rel: updatecheck.Release{Version: "0.3.1-rc.1", URL: "u"}},
		"error":        {rel: v031, err: errors.New("offline")},
	}
	for name, c := range checkers {
		m := releaseModel(t, c, updatecheck.BrewUpgrade)
		if view := plain(m); strings.Contains(view, "UPDATE") {
			t.Fatalf("%s: view mentions an update:\n%s", name, view)
		}
		if strings.Contains(m.status, "offline") {
			t.Fatalf("%s: the failure surfaced: %q", name, m.status)
		}
	}
}

func TestNoCheckWithoutACheckerOrAVersion(t *testing.T) {
	if cmd := New(playbacktest.New(), Options{Version: "0.3.0"}).checkReleaseCmd(); cmd != nil {
		t.Fatal("a model without a checker checks")
	}
	c := &releaseChecker{rel: v031}
	for _, v := range []string{"", "dev"} {
		if cmd := New(playbacktest.New(), Options{Updates: c, Version: v}).checkReleaseCmd(); cmd != nil {
			t.Fatalf("version %q checks", v)
		}
	}
}

func TestInitChecksForAReleaseOnce(t *testing.T) {
	f := playbacktest.New()
	defer f.Close()
	c := &releaseChecker{rel: v031}
	m := New(f, Options{SkipBoot: true, Now: newClock().now, Seed: 2077, Updates: c, Version: "0.3.0"})
	out := make(chan tea.Msg, 16)
	launch(m.Init(), out)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-out:
			if _, ok := msg.(releaseMsg); ok {
				if n := c.calls.Load(); n != 1 {
					t.Fatalf("checker called %d times; want 1", n)
				}
				return
			}
		case <-deadline:
			t.Fatal("Init did not check for a release")
		}
	}
}

func TestTheSettingsShowTheNewerRelease(t *testing.T) {
	m := releaseModel(t, &releaseChecker{rel: v031}, updatecheck.BrewUpgrade)
	t.Cleanup(func() { applyTheme(themes[0]) })
	m, _ = press(t, m, "s")
	view := plain(m)
	if !strings.Contains(view, "UPDATE v0.3.1 // brew upgrade --cask nu11signal") {
		t.Fatalf("settings lack the update line:\n%s", view)
	}
	// Drawn in the buttons' style, as the status line is.
	body, _ := m.settingsPanel(m.width, m.height)
	if !strings.Contains(strings.Join(body, "\n"), stHiBold.Render(fit("UPDATE v0.3.1 // brew upgrade --cask nu11signal", m.width-3))) {
		t.Fatalf("the settings update line is not in the buttons' style:\n%q", body)
	}
	// The theme rows still answer clicks where they are drawn.
	m, _ = press(t, m, "down", "enter")
	if m.theme != "BLUE" {
		t.Fatalf("theme %q after picking the second row; want BLUE", m.theme)
	}

	up := releaseModel(t, &releaseChecker{rel: updatecheck.Release{Version: "0.3.0"}}, updatecheck.BrewUpgrade)
	up, _ = press(t, up, "s")
	if strings.Contains(plain(up), "UPDATE") {
		t.Fatalf("settings mention an update when up to date:\n%s", plain(up))
	}
}
