package radio

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// withVolume is m once the player reported level.
func withVolume(t *testing.T, m Model, level float64) Model {
	t.Helper()
	m, _ = step(t, m, volumeMsg{level: level})
	return m
}

// volumeRow is the NOW PLAYING line holding the volume readout.
func volumeRow(t *testing.T, m Model) string {
	t.Helper()
	for _, l := range strings.Split(plain(m), "\n") {
		if strings.Contains(l, "VOL ") {
			return l
		}
	}
	t.Fatalf("no volume readout in:\n%s", plain(m))
	return ""
}

// setVolumeCalls are the levels SetVolume was called with, in order.
func setVolumeCalls(f *playbacktest.Fake) []any {
	var out []any
	for _, c := range callsOf(f, "SetVolume") {
		out = append(out, c.Args[0])
	}
	return out
}

func TestStartupReadsTheVolume(t *testing.T) {
	f := playbacktest.New()
	f.VolumeResult = 0.4
	m := newModel(t, f, newClock())
	out := make(chan tea.Msg, 16)
	launch(m.Init(), out)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-out:
			if v, ok := msg.(volumeMsg); ok {
				m, _ = step(t, m, v)
				f.Close()
				if got := volumeRow(t, m); !strings.Contains(got, "40%") {
					t.Fatalf("readout %q; want 40%%", got)
				}
				return
			}
		case <-deadline:
			f.Close()
			t.Fatalf("Init did not read the volume; calls %v", f.Calls())
		}
	}
}

func TestVolumeReadoutShowsTheLevel(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	if got := volumeRow(t, m); !strings.Contains(got, "VOL --") {
		t.Fatalf("readout before the read %q; want VOL --", got)
	}
	m = withVolume(t, m, 0.6)
	got := volumeRow(t, m)
	if !strings.Contains(got, "60%") || !strings.Contains(got, "▮") {
		t.Fatalf("readout %q; want a meter at 60%%", got)
	}
	// A level out of range is shown clamped.
	m = withVolume(t, m, 1.7)
	if got := volumeRow(t, m); !strings.Contains(got, "100%") {
		t.Fatalf("readout %q; want 100%%", got)
	}
}

func TestVolumeKeysStepAndClamp(t *testing.T) {
	tests := []struct {
		name  string
		from  float64
		keys  []string
		sent  []any
		level float64
	}{
		{"plus", 0.6, []string{"+"}, []any{0.65}, 0.65},
		{"equals", 0.6, []string{"="}, []any{0.65}, 0.65},
		{"minus", 0.6, []string{"-"}, []any{0.55}, 0.55},
		{"shift+up", 0.6, []string{"shift+up"}, []any{0.65}, 0.65},
		{"shift+down", 0.6, []string{"shift+down"}, []any{0.55}, 0.55},
		{"k", 0.6, []string{"k"}, []any{0.65}, 0.65},
		{"j", 0.6, []string{"j"}, []any{0.55}, 0.55},
		{"clamped at full", 0.98, []string{"+"}, []any{1.0}, 1},
		{"clamped at silent", 0.02, []string{"-"}, []any{0.0}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			m := withVolume(t, loaded(t, f, newClock()), tt.from)
			m, cmd := press(t, m, tt.keys...)
			m = settle(t, m, cmd)
			if got := setVolumeCalls(f); !reflect.DeepEqual(got, tt.sent) {
				t.Fatalf("SetVolume calls %v; want %v", got, tt.sent)
			}
			if m.volume != tt.level || !m.volumeKnown {
				t.Fatalf("level %v known %v; want %v", m.volume, m.volumeKnown, tt.level)
			}
		})
	}
}

func TestVolumeKeysOnPagesAndThePlayer(t *testing.T) {
	opens := map[string]func(*testing.T, *playbacktest.Fake) Model{
		"list": func(t *testing.T, f *playbacktest.Fake) Model { return loaded(t, f, newClock()) },
		"page": func(t *testing.T, f *playbacktest.Fake) Model { return openStation(t, loaded(t, f, newClock()), 0) },
		"results": func(t *testing.T, f *playbacktest.Fake) Model {
			return openResults(t, f, &fakeRecents{})
		},
		"player": func(t *testing.T, f *playbacktest.Fake) Model {
			m, _ := press(t, loaded(t, f, newClock()), "right")
			return m
		},
		"tabs": func(t *testing.T, f *playbacktest.Fake) Model {
			m, _ := press(t, loaded(t, f, newClock()), "up")
			return m
		},
	}
	keys := []struct {
		key  string
		want float64
	}{{"+", 0.55}, {"k", 0.55}, {"j", 0.45}}
	for name, open := range opens {
		for _, k := range keys {
			t.Run(name+" "+k.key, func(t *testing.T) {
				f := playbacktest.New()
				m := withVolume(t, open(t, f), 0.5)
				focus, stack, cursor := m.focus, stackKinds(m), m.cursor()
				m, cmd := press(t, m, k.key)
				settle(t, m, cmd)
				if got := setVolumeCalls(f); !reflect.DeepEqual(got, []any{k.want}) {
					t.Fatalf("SetVolume calls %v; want [%v]", got, k.want)
				}
				// The volume keys never move the cursor or the focus.
				if m.focus != focus || !reflect.DeepEqual(stackKinds(m), stack) || m.cursor() != cursor {
					t.Fatalf("focus %v stack %v cursor %d; want %v %v %d kept",
						m.focus, stackKinds(m), m.cursor(), focus, stack, cursor)
				}
			})
		}
	}
}

func TestArrowsAloneMoveTheRows(t *testing.T) {
	f := playbacktest.New()
	m := withVolume(t, loaded(t, f, newClock()), 0.5)
	m, _ = press(t, m, "down")
	if m.stationCursor() != 1 {
		t.Fatalf("down: cursor %d; want 1", m.stationCursor())
	}
	m, _ = press(t, m, "j", "j", "k")
	if m.stationCursor() != 1 || m.focus != areaList {
		t.Fatalf("j/k: cursor %d focus %v; want row 1 on the list kept", m.stationCursor(), m.focus)
	}
	m, _ = press(t, m, "up")
	if m.stationCursor() != 0 {
		t.Fatalf("up: cursor %d; want 0", m.stationCursor())
	}

	p := openStation(t, withVolume(t, loaded(t, f, newClock()), 0.5), 0)
	p, _ = press(t, p, "down")
	at := p.cursor()
	if at == 0 {
		t.Fatal("down did not move the page cursor")
	}
	p, _ = press(t, p, "j", "k", "k")
	if p.cursor() != at || p.focus != areaList {
		t.Fatalf("page j/k: cursor %d focus %v; want %d on the list kept", p.cursor(), p.focus, at)
	}
}

func TestVolumeKeysTypeInTheSearchInput(t *testing.T) {
	f := playbacktest.New()
	m := withVolume(t, loaded(t, f, newClock()), 0.5)
	m, _ = press(t, m, "/")
	m = typeText(t, m, "a-ha+=jk")
	if m.input.Value() != "a-ha+=jk" || len(callsOf(f, "SetVolume")) != 0 {
		t.Fatalf("input %q, SetVolume calls %v; want the keys typed", m.input.Value(), setVolumeCalls(f))
	}
	// shift+↑ and shift+↓ work anywhere, the input included.
	m, cmd := press(t, m, "shift+down")
	settle(t, m, cmd)
	if got := setVolumeCalls(f); !reflect.DeepEqual(got, []any{0.45}) {
		t.Fatalf("SetVolume calls %v; want [0.45]", got)
	}
	if m.input.Value() != "a-ha+=jk" {
		t.Fatalf("input %q; shift+down edited it", m.input.Value())
	}
}

func TestRapidVolumePressesAreCoalesced(t *testing.T) {
	f := playbacktest.New()
	m := withVolume(t, loaded(t, f, newClock()), 0.5)
	m, first := press(t, m, "+")
	m, second := press(t, m, "+")
	m, third := press(t, m, "+")
	if first == nil || second != nil || third != nil {
		t.Fatalf("commands %v %v %v; want only the first press to call the player", first != nil, second != nil, third != nil)
	}
	if got := volumeRow(t, m); !strings.Contains(got, "65%") {
		t.Fatalf("readout %q; want the optimistic 65%%", got)
	}
	m, next := step(t, m, run(t, first))
	if next == nil {
		t.Fatal("the answer did not send the latest level")
	}
	m, last := step(t, m, run(t, next))
	if last != nil {
		t.Fatal("a settled level was sent again")
	}
	if got := setVolumeCalls(f); !reflect.DeepEqual(got, []any{0.55, 0.65}) {
		t.Fatalf("SetVolume calls %v; want the first and the latest level", got)
	}
	if m.volume != 0.65 || m.volumeBusy {
		t.Fatalf("level %v busy %v; want 0.65 settled", m.volume, m.volumeBusy)
	}
}

func TestRefusedVolumeShowsOnTheStatusLine(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"SetVolume": errors.New("output device has no settable volume")}
	m := withVolume(t, loaded(t, f, newClock()), 0.5)
	m, cmd := press(t, m, "+")
	m = settle(t, m, cmd)
	if view := plain(m); !strings.Contains(view, "VOLUME FAILED // OUTPUT DEVICE HAS NO SETTABLE VOLUME") {
		t.Fatalf("status line lacks the refusal:\n%s", view)
	}
	if got := volumeRow(t, m); !strings.Contains(got, "VOL --") {
		t.Fatalf("readout %q; want VOL --", got)
	}

	// The next press reads the level again before stepping from it.
	f.MethodErr = nil
	f.VolumeResult = 0.3
	m, cmd = press(t, m, "+")
	if calls := callsOf(f, "Volume"); len(calls) != 0 {
		t.Fatalf("Volume read before the command ran: %v", calls)
	}
	m, cmd = step(t, m, run(t, cmd)) // the read, then the step from it
	m = settle(t, m, cmd)
	if got := setVolumeCalls(f); len(got) < 2 || got[len(got)-1] != 0.35 {
		t.Fatalf("SetVolume calls %v; want 0.35 after reading 0.3", got)
	}
}

func TestUnreadableVolumeStaysQuietAtStartup(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	m, _ = step(t, m, volumeMsg{err: errors.New("no output device")})
	if m.status != "" {
		t.Fatalf("status %q; want the startup read to fail quietly", m.status)
	}
	if got := volumeRow(t, m); !strings.Contains(got, "VOL --") {
		t.Fatalf("readout %q; want VOL --", got)
	}
}

func TestVolumeButtons(t *testing.T) {
	f := playbacktest.New()
	m := withVolume(t, playingModel(t, f), 0.5)
	if got := textAt(m, zoneOf(t, m, zoneVolDown)); !strings.Contains(got, "-") {
		t.Fatalf("VOL- zone covers %q", got)
	}
	if got := textAt(m, zoneOf(t, m, zoneVolUp)); !strings.Contains(got, "+") {
		t.Fatalf("VOL+ zone covers %q", got)
	}
	m, cmd := click(t, m, zoneVolUp)
	m = settle(t, m, cmd)
	if m.focus != areaPlayer || m.control != ctlVolUp {
		t.Fatalf("focus %v control %v; want VOL+ focused", m.focus, m.control)
	}
	m, cmd = click(t, m, zoneVolDown)
	settle(t, m, cmd)
	if got := setVolumeCalls(f); !reflect.DeepEqual(got, []any{0.55, 0.5}) {
		t.Fatalf("SetVolume calls %v; want up then down", got)
	}
}

func TestArrowsReachTheVolumeRow(t *testing.T) {
	f := playbacktest.New()
	m := withVolume(t, playingModel(t, f), 0.5)
	steps := []struct {
		key   string
		focus focusArea
		ctl   playerControl
	}{
		{"right", areaPlayer, ctlPlay},
		{"down", areaPlayer, ctlVolDown},
		{"down", areaPlayer, ctlVolDown}, // the bottom row stays
		{"right", areaPlayer, ctlVolUp},
		{"right", areaPlayer, ctlVolUp}, // the last button stays
		{"up", areaPlayer, ctlNext},
		{"right", areaPlayer, ctlExpand},
		{"down", areaPlayer, ctlVolUp},
		{"left", areaPlayer, ctlVolDown},
		{"up", areaPlayer, ctlPrev},
		{"down", areaPlayer, ctlVolDown},
		{"left", areaList, ctlVolDown},
	}
	for i, s := range steps {
		m, _ = press(t, m, s.key)
		if m.focus != s.focus || (s.focus == areaPlayer && m.control != s.ctl) {
			t.Fatalf("step %d (%s): focus %v control %v; want %v %v", i, s.key, m.focus, m.control, s.focus, s.ctl)
		}
	}
	m, _ = press(t, m, "right", "down", "right")
	if got := textAt(m, zoneOf(t, m, zoneVolUp)); !strings.Contains(got, "▸") {
		t.Fatalf("focused VOL+ shows %q; want the ▸ marker", got)
	}
	m, cmd := press(t, m, "enter")
	settle(t, m, cmd)
	if got := setVolumeCalls(f); !reflect.DeepEqual(got, []any{0.55}) {
		t.Fatalf("SetVolume calls %v; want enter on VOL+ to step up", got)
	}
}

func TestPlayerKeysActOnTheTabs(t *testing.T) {
	tests := []struct {
		key    string
		method string
		args   []any
	}{
		{"space", "Pause", nil},
		{"n", "Next", nil},
		{"p", "Previous", nil},
		{".", "Seek", []any{70 * time.Second}},
		{"shift+left", "Seek", []any{50 * time.Second}},
		{"+", "SetVolume", []any{0.55}},
		{"-", "SetVolume", []any{0.45}},
		{"k", "SetVolume", []any{0.55}},
		{"j", "SetVolume", []any{0.45}},
	}
	for _, view := range []string{"root", "search"} {
		for _, tt := range tests {
			t.Run(view+" "+tt.key, func(t *testing.T) {
				f := playbacktest.New()
				m := withVolume(t, playingModel(t, f), 0.5)
				if view == "search" {
					m, _ = press(t, m, "/")
					m = typeText(t, m, "da")
				} else {
					m, _ = press(t, m, "up") // the + NEW PLAYLIST row
				}
				m, _ = press(t, m, "up")
				if m.focus != areaTabs {
					t.Fatalf("focus %v; want the tabs", m.focus)
				}
				m, cmd := press(t, m, tt.key)
				if m.focus != areaTabs {
					t.Fatalf("%s moved the focus to %v; want the tabs kept", tt.key, m.focus)
				}
				if view == "search" && m.input.Value() != "da" {
					t.Fatalf("%s was typed: %q", tt.key, m.input.Value())
				}
				settle(t, m, cmd)
				assertCall(t, f, tt.method, tt.args...)
			})
		}
	}
}

func TestVolumePressesBeforeTheStartupReadWaitForIt(t *testing.T) {
	// Init reads the volume; presses made before that read answers must
	// not start a second read, and are applied once it answers.
	f := playbacktest.New()
	m := loaded(t, f, newClock())
	m, first := press(t, m, "+")
	m, second := press(t, m, "+")
	if first != nil || second != nil {
		t.Fatalf("commands %v %v; want the presses to wait for the startup read", first != nil, second != nil)
	}
	m, cmd := step(t, m, volumeMsg{level: 0.5})
	m = settle(t, m, cmd)
	if calls := callsOf(f, "Volume"); len(calls) != 0 {
		t.Fatalf("Volume read again: %v", calls)
	}
	if got := setVolumeCalls(f); !reflect.DeepEqual(got, []any{0.6}) {
		t.Fatalf("SetVolume calls %v; want both presses applied to the startup level", got)
	}
}
