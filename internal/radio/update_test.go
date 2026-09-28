package radio

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"soulking/internal/playback"
	"soulking/internal/playback/playbacktest"
)

// clock is a settable time source for deterministic tests.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock                   { return &clock{t: time.Date(2077, 11, 20, 23, 41, 7, 0, time.UTC)} }
func key(s string) tea.KeyPressMsg       { return keyMsg(s) }
func stations() []playback.Playlist {
	return []playback.Playlist{{ID: "pl-1", Name: "Night Drive"}, {ID: "pl-2", Name: "Samurai"}, {ID: "pl-3", Name: "Body Heat"}}
}
func songs() []playback.Song {
	return []playback.Song{{ID: "s1", Title: "Stronger"}, {ID: "s2", Title: "Harder"}, {ID: "s3", Title: "Faster"}}
}
func playing(pos, dur time.Duration) playback.State {
	return playback.State{Status: playback.StatusPlaying, Title: "Chippin' In", Artist: "Samurai", Album: "Never Fade Away", SongID: "c1", Position: pos, Duration: dur}
}

func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func newModel(t *testing.T, f *playbacktest.Fake, c *clock) Model {
	t.Helper()
	m := New(f, Options{Now: c.now, Seed: 2077})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

// loaded returns a model that is authorized and shows the fake's stations.
func loaded(t *testing.T, f *playbacktest.Fake, c *clock) Model {
	t.Helper()
	f.PlaylistsResult = stations()
	m := newModel(t, f, c)
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	return m
}

func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	nm, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want radio.Model", next)
	}
	return nm, cmd
}

func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = step(t, m, key(k))
	}
	return m, cmd
}

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = step(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// run executes cmd, failing instead of hanging if it blocks.
func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("command blocked")
		return nil
	}
}

// runAll executes cmd and, when it is a batch, every command in it.
func runAll(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	msg := run(t, cmd)
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		if c != nil {
			out = append(out, runAll(t, c)...)
		}
	}
	return out
}

func lastCall(t *testing.T, f *playbacktest.Fake) playbacktest.Call {
	t.Helper()
	calls := f.Calls()
	if len(calls) == 0 {
		t.Fatal("no player calls recorded")
	}
	return calls[len(calls)-1]
}

func assertCall(t *testing.T, f *playbacktest.Fake, method string, args ...any) {
	t.Helper()
	got := lastCall(t, f)
	want := playbacktest.Call{Method: method, Args: args}
	if got.Method != want.Method || !reflect.DeepEqual(got.Args, want.Args) {
		t.Fatalf("last call = %s%v, want %s%v", got.Method, got.Args, want.Method, want.Args)
	}
}

func TestStartupAuthorizesAndLoadsStations(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())

	calls := f.Calls()
	if len(calls) != 2 || calls[0].Method != "Authorize" || calls[1].Method != "Playlists" {
		t.Fatalf("calls = %v, want Authorize then Playlists", calls)
	}
	view := m.render()
	for _, want := range []string{"088.1", "NIGHT DRIVE", "089.7", "SAMURAI", "AUTH OK"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

func TestStartupUnauthorizedShowsErrorScreen(t *testing.T) {
	tests := []struct {
		name string
		fake func(*playbacktest.Fake)
	}{
		{"denied", func(f *playbacktest.Fake) { f.AuthStatus = playback.AuthDenied }},
		{"authorize fails", func(f *playbacktest.Fake) { f.MethodErr = map[string]error{"Authorize": errors.New("no helper")} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			tt.fake(f)
			m := newModel(t, f, newClock())
			m, cmd := step(t, m, run(t, m.authorizeCmd()))
			if cmd != nil {
				t.Error("unauthorized startup still issued a command")
			}
			if view := m.render(); !strings.Contains(view, "ACCESS DENIED") {
				t.Errorf("view lacks the error screen:\n%s", view)
			}
		})
	}
}

func TestEnterOnStationPlaysPlaylist(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, cmd := press(t, m, "j", "enter")
	run(t, cmd)
	assertCall(t, f, "PlayPlaylist", "pl-2")
}

func TestNavigationClampsToList(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "k", "up")
	if m.cursor() != 0 {
		t.Fatalf("cursor = %d after moving up at top, want 0", m.cursor())
	}
	m, _ = press(t, m, "down", "j", "j", "j", "j")
	if m.cursor() != 2 {
		t.Fatalf("cursor = %d after moving past the end, want 2", m.cursor())
	}
}

func TestSearchThenPlayResult(t *testing.T) {
	f := playbacktest.New()
	f.SearchResult = songs()
	m := loaded(t, f, newClock())

	m, _ = press(t, m, "/")
	m = typeText(t, m, "daft q")
	if !strings.Contains(m.render(), "CATALOG SCAN") {
		t.Fatal("search input not shown")
	}
	m, cmd := press(t, m, "enter")
	m, _ = step(t, m, run(t, cmd))
	assertCall(t, f, "Search", "daft q", 25)
	if f.Closed() {
		t.Fatal("typing q in the search input quit the player")
	}
	if view := m.render(); !strings.Contains(view, "HARDER") || strings.Contains(view, "SCANNING") {
		t.Fatalf("results not shown, or scan status left behind:\n%s", view)
	}

	m, cmd = press(t, m, "j", "enter")
	run(t, cmd)
	assertCall(t, f, "PlaySongs", []string{"s1", "s2", "s3"}, 1)
}

func TestSearchEscCancelsInput(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, _ = press(t, m, "/")
	m = typeText(t, m, "abc")
	m, cmd := press(t, m, "esc")
	if cmd != nil {
		run(t, cmd)
	}
	if strings.Contains(m.render(), "CATALOG SCAN") {
		t.Fatal("search input still shown after esc")
	}
	for _, c := range f.Calls() {
		if c.Method == "Search" {
			t.Fatal("esc ran a search")
		}
	}
}

func TestTabAndEscSwitchBetweenStationsAndResults(t *testing.T) {
	f := playbacktest.New()
	f.SearchResult = songs()
	m := loaded(t, f, newClock())

	m, _ = press(t, m, "tab")
	if m.showingResults() {
		t.Fatal("tab switched to results with no results")
	}
	m, _ = press(t, m, "/")
	m = typeText(t, m, "x")
	m, cmd := press(t, m, "enter")
	m, _ = step(t, m, run(t, cmd))
	if !m.showingResults() {
		t.Fatal("search results not shown")
	}
	m, _ = press(t, m, "tab")
	if m.showingResults() {
		t.Fatal("tab did not return to stations")
	}
	m, _ = press(t, m, "tab")
	if !m.showingResults() {
		t.Fatal("tab did not return to results")
	}
	m, _ = press(t, m, "esc")
	if m.showingResults() {
		t.Fatal("esc did not leave results")
	}
}

func TestSpaceTogglesByStatus(t *testing.T) {
	tests := []struct {
		status playback.Status
		want   string
	}{
		{playback.StatusPlaying, "Pause"},
		{playback.StatusSeeking, "Pause"},
		{playback.StatusPaused, "Resume"},
		{playback.StatusStopped, "Resume"},
		{playback.StatusInterrupted, "Resume"},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			f := playbacktest.New()
			m := loaded(t, f, newClock())
			s := playing(0, time.Minute)
			s.Status = tt.status
			m, _ = step(t, m, stateMsg{state: s})
			_, cmd := press(t, m, "space")
			run(t, cmd)
			assertCall(t, f, tt.want)
		})
	}
}

func TestNextAndPrevious(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, cmd := press(t, m, "n")
	run(t, cmd)
	assertCall(t, f, "Next")
	_, cmd = press(t, m, "p")
	run(t, cmd)
	assertCall(t, f, "Previous")
}

func TestSeekClampsToTrack(t *testing.T) {
	tests := []struct {
		name     string
		pos, dur time.Duration
		key      string
		want     time.Duration
	}{
		{"forward", time.Minute, 3 * time.Minute, "right", 70 * time.Second},
		{"back", time.Minute, 3 * time.Minute, "left", 50 * time.Second},
		{"back clamps at zero", 5 * time.Second, 3 * time.Minute, "left", 0},
		{"forward clamps at duration", 175 * time.Second, 3 * time.Minute, "right", 3 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			m := loaded(t, f, newClock())
			m, _ = step(t, m, stateMsg{state: playing(tt.pos, tt.dur)})
			_, cmd := press(t, m, tt.key)
			run(t, cmd)
			assertCall(t, f, "Seek", tt.want)
		})
	}
}

func TestSeekWithoutTrackDoesNothing(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	before := len(f.Calls())
	if _, cmd := press(t, m, "right"); cmd != nil {
		run(t, cmd)
	}
	if len(f.Calls()) != before {
		t.Fatalf("seek without a track called the player: %v", f.Calls()[before:])
	}
}

func TestSeekUsesElapsedPlaybackTime(t *testing.T) {
	f := playbacktest.New()
	c := newClock()
	m := loaded(t, f, c)
	m, _ = step(t, m, stateMsg{state: playing(time.Minute, 3*time.Minute)})
	c.advance(5 * time.Second)
	_, cmd := press(t, m, "right")
	run(t, cmd)
	assertCall(t, f, "Seek", 75*time.Second)
}

func TestStateMessagesUpdateNowPlayingAndRearm(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, cmd := step(t, m, stateMsg{state: playing(83*time.Second, 225*time.Second)})
	if cmd == nil {
		t.Fatal("state message did not re-arm the state listener")
	}
	if !m.tickFast {
		t.Fatal("playback start did not switch to the fast animation tick")
	}
	stale := m.tickGen - 1
	if _, cmd := step(t, m, tickMsg{gen: stale}); cmd != nil {
		t.Fatal("a superseded tick chain was re-armed")
	}
	for i := 0; i < glitchFrames+1; i++ {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	view := m.render()
	for _, want := range []string{"CHIPPIN' IN", "SAMURAI", "NEVER FADE AWAY", "01:23 / 03:45", "PLAYING"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}

	f.PushState(playback.State{Status: playback.StatusPaused, Title: "Chippin' In", SongID: "c1"})
	var msg tea.Msg
	for _, got := range runAll(t, cmd) {
		if sm, ok := got.(stateMsg); ok && sm.state.Status == playback.StatusPaused {
			msg = sm
		}
	}
	if msg == nil {
		t.Fatal("re-armed listener did not deliver the paused state")
	}
	m, _ = step(t, m, msg)
	if !strings.Contains(m.render(), "PAUSED") {
		t.Error("view does not show PAUSED")
	}
}

func TestClosedChannelsShowSignalLost(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	f.Close()

	m, cmd := step(t, m, run(t, m.waitStates()))
	if cmd != nil {
		t.Error("closed state channel was re-armed")
	}
	m, cmd = step(t, m, run(t, m.waitErrors()))
	if cmd != nil {
		t.Error("closed error channel was re-armed")
	}
	if view := m.render(); !strings.Contains(view, "SIGNAL LOST") {
		t.Fatalf("view lacks SIGNAL LOST:\n%s", view)
	}
}

func TestAsyncErrorsShowInStatusLineAndRearm(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	f.PushError(errors.New("queue exhausted"))
	m, cmd := step(t, m, run(t, m.waitErrors()))
	if cmd == nil {
		t.Error("error listener was not re-armed")
	}
	if view := m.render(); !strings.Contains(view, "QUEUE EXHAUSTED") {
		t.Fatalf("status line missing the error:\n%s", view)
	}
}

func TestFailedCallShowsTransientStatus(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"Next": errors.New("nothing queued")}
	c := newClock()
	m := loaded(t, f, c)
	m, cmd := press(t, m, "n")
	m, _ = step(t, m, run(t, cmd))
	if view := m.render(); !strings.Contains(view, "NOTHING QUEUED") {
		t.Fatalf("status line missing the error:\n%s", view)
	}
	c.advance(statusTTL + time.Second)
	m, _ = step(t, m, tickMsg{gen: m.tickGen})
	if view := m.render(); strings.Contains(view, "NOTHING QUEUED") {
		t.Fatal("status line did not expire")
	}
}

func TestQuitClosesPlayer(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			f := playbacktest.New()
			m := loaded(t, f, newClock())
			_, cmd := press(t, m, k)
			msg := run(t, cmd)
			if _, ok := msg.(tea.QuitMsg); !ok {
				t.Fatalf("quit command returned %T, want tea.QuitMsg", msg)
			}
			if !f.Closed() {
				t.Fatal("player was not closed")
			}
		})
	}
}

func TestTitleGlitchesOnTrackChangeThenSettles(t *testing.T) {
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, _ = step(t, m, stateMsg{state: playing(0, time.Minute)})
	if m.glitch == 0 {
		t.Fatal("track change did not start a glitch")
	}
	for i := 0; i < glitchFrames; i++ {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	if m.glitch != 0 {
		t.Fatalf("glitch still active after %d ticks", glitchFrames)
	}
	// Same song again: no new glitch.
	m, _ = step(t, m, stateMsg{state: playing(time.Second, time.Minute)})
	if m.glitch != 0 {
		t.Fatal("progress update on the same song restarted the glitch")
	}
}

func TestGlitchText(t *testing.T) {
	const s = "NIGHT CITY"
	if got := glitchText(s, 0, 1); got != s {
		t.Errorf("no glitch changed text: %q", got)
	}
	got := glitchText(s, 3, 1)
	if got == s {
		t.Error("active glitch left text unchanged")
	}
	if len([]rune(got)) != len([]rune(s)) {
		t.Errorf("glitch changed length: %q", got)
	}
	if again := glitchText(s, 3, 1); again != got {
		t.Errorf("glitch is not deterministic: %q vs %q", got, again)
	}
}
