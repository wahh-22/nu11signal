package radio

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// navOf is the nav bar line of the frame on screen.
func navOf(m Model) string { return strings.Split(plain(m), "\n")[1] }

// statusOf is the status line of the frame on screen, over the footer.
func statusOf(m Model) string {
	lines := strings.Split(plain(m), "\n")
	return strings.TrimRight(lines[len(lines)-2], " ")
}

// nodeOf is the flavor text at the right of the nav bar: the text
// between the rule and its closing "──".
func nodeOf(t *testing.T, m Model) string {
	t.Helper()
	nav := strings.TrimSuffix(strings.TrimRight(navOf(m), " "), "──")
	i := strings.LastIndex(nav, "─ ")
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(nav[i+len("─ "):])
}

var nodePattern = regexp.MustCompile(`^NODE [0-9A-F]{2} // NC-GRID$`)

func TestTheNavBarShowsANetNode(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	root := loaded(t, f, newClock())
	want := nodeOf(t, root)
	if !nodePattern.MatchString(want) {
		t.Fatalf("nav %q; want a NODE xx // NC-GRID readout, got %q", navOf(root), want)
	}
	// The readout is flavor, not a breadcrumb: the same wherever the list
	// panel is.
	check := func(name string, m Model) {
		t.Helper()
		nav := navOf(m)
		if got := nodeOf(t, m); got != want {
			t.Errorf("%s: node %q; want %q (nav %q)", name, got, want, nav)
		}
		if strings.Contains(nav, "PLAYLISTS //") || strings.Contains(nav, "SEARCH //") {
			t.Errorf("%s: the nav bar still shows a breadcrumb: %q", name, nav)
		}
	}
	check("page", openStation(t, root, 0))
	search, _ := press(t, root, "/")
	check("search", search)
	results, _ := press(t, searchFor(t, root, "daft"), "enter")
	check("results", results)
}

func TestTheNetNodeFollowsTheSeed(t *testing.T) {
	node := func(seed uint64) string {
		m := New(playbacktest.New(), Options{SkipBoot: true, Now: newClock().now, Seed: seed})
		m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
		return nodeOf(t, m)
	}
	if a, b := node(2077), node(2077); a != b {
		t.Fatalf("seed 2077 gave %q then %q", a, b)
	}
	seen := map[string]bool{}
	for seed := range uint64(32) {
		n := node(seed)
		if !nodePattern.MatchString(n) {
			t.Fatalf("seed %d: node %q", seed, n)
		}
		seen[n] = true
	}
	if len(seen) < 4 {
		t.Fatalf("32 seeds gave only %d nodes: %v", len(seen), seen)
	}
}

func TestTheNetNodeIsCutToFit(t *testing.T) {
	m := openStation(t, loaded(t, playbacktest.New(), newClock()), 0)
	full := nodeOf(t, m)
	var cut, gone bool
	for w := 44; w <= 80; w++ {
		m, _ = step(t, m, tea.WindowSizeMsg{Width: w, Height: 24})
		nav := navOf(m)
		if got := ansi.StringWidth(nav); got > w {
			t.Fatalf("at %d cells nav %d cells: %q", w, got, nav)
		}
		if !strings.Contains(nav, "BACK") {
			t.Fatalf("at %d cells nav %q lost the bar", w, nav)
		}
		switch got := nodeOf(t, m); {
		case got == full:
		case strings.HasSuffix(got, "…") && strings.HasPrefix(full, strings.TrimSuffix(got, "…")):
			cut = true
		case !strings.Contains(nav, "NODE"):
			gone = true
		default:
			t.Fatalf("at %d cells nav %q; want the node whole, cut with … or gone", w, nav)
		}
	}
	if !cut || !gone {
		t.Fatalf("cut %v gone %v; want both between 44 and 80 cells", cut, gone)
	}
}

// spectrumLabel is the NOW PLAYING panel's bottom label on screen.
func spectrumLabel(m Model) string {
	for _, l := range strings.Split(plain(m), "\n") {
		if i := strings.Index(l, "SPECTRUM "); i >= 0 {
			return strings.Fields(l[i+len("SPECTRUM "):])[0]
		}
	}
	return ""
}

func TestNowPlayingNamesTheSpectrumSource(t *testing.T) {
	m, f, _ := playingWithLevels(t)
	m = tick(t, m)
	if got := spectrumLabel(m); got != "SIM" {
		t.Fatalf("playing without readings: label %q; want SIM\n%s", got, plain(m))
	}
	f.PushLevels(playback.Spectrum{Bands: allBars(0.5)})
	m = tick(t, m)
	if got := spectrumLabel(m); got != "LIVE" {
		t.Fatalf("playing on readings: label %q; want LIVE\n%s", got, plain(m))
	}
	expanded, _ := press(t, m, "f")
	if got := spectrumLabel(expanded); got != "LIVE" {
		t.Fatalf("expanded: label %q; want LIVE\n%s", got, plain(expanded))
	}
	paused := playing(time.Second, time.Minute)
	paused.Status = playback.StatusPaused
	m, _ = step(t, m, stateMsg{state: paused})
	if got := spectrumLabel(m); got != "HOLD" {
		t.Fatalf("paused: label %q; want HOLD", got)
	}
	if strings.Contains(plain(m), "0x2077") {
		t.Fatalf("the panel still shows the serial code:\n%s", plain(m))
	}
}

// onAir plays the Night Drive library playlist from its ▶ PLAY row and
// tells the model the player is on song id, with repeat.
func onAir(t *testing.T, f *playbacktest.Fake, id string, repeat playback.RepeatMode) Model {
	t.Helper()
	m, cmd := tune(t, loaded(t, f, newClock()), 0)
	m = settle(t, m, cmd)
	s := playing(time.Second, time.Minute)
	s.SongID, s.Repeat = id, repeat
	m, _ = step(t, m, stateMsg{state: s})
	return m
}

func TestTheStatusLineNamesTheSongUpNext(t *testing.T) {
	m := onAir(t, playbacktest.New(), "i.1", playback.RepeatOff)
	if got, want := statusOf(m), "◢◤◢◤ UP NEXT // RESONANCE · HOME"; got != want {
		t.Fatalf("status %q; want %q", got, want)
	}
	m = onAir(t, playbacktest.New(), "i.2", playback.RepeatOff)
	if got, want := statusOf(m), "◢◤◢◤ UP NEXT // TURBO KILLER · CARPENTER BRUT"; got != want {
		t.Fatalf("status %q; want %q", got, want)
	}
	// On the last song NEXT starts the list over while it repeats.
	m = onAir(t, playbacktest.New(), "i.3", playback.RepeatAll)
	if got, want := statusOf(m), "◢◤◢◤ UP NEXT // NIGHTCALL · KAVINSKY"; got != want {
		t.Fatalf("last song: status %q; want %q", got, want)
	}
}

func TestUpNextSkipsTheSongsLeftOutOfTheQueue(t *testing.T) {
	f := playbacktest.New()
	f.PlaySongsReport = playback.QueueReport{Missing: []string{"i.2"}}
	m := onAir(t, f, "i.1", playback.RepeatOff)
	// The skipped notice holds the line first.
	m.status = ""
	if got, want := statusOf(m), "◢◤◢◤ UP NEXT // TURBO KILLER · CARPENTER BRUT"; got != want {
		t.Fatalf("status %q; want %q", got, want)
	}
}

func TestTheStatusLineFallsBackToTheVolumeAndEffects(t *testing.T) {
	// A song the model did not queue: nothing known to follow it.
	m := playingModel(t, playbacktest.New())
	if got, want := statusOf(m), "◢◤◢◤ FX OFF"; got != want {
		t.Fatalf("no volume mode: status %q; want %q", got, want)
	}
	m, _, _ = appPlaying(t)
	if got, want := statusOf(m), "◢◤◢◤ APP VOLUME // FX OFF"; got != want {
		t.Fatalf("app volume: status %q; want %q", got, want)
	}
	m, _ = press(t, m, "x")
	m.status = ""
	if got, want := statusOf(m), "◢◤◢◤ APP VOLUME // FX ON"; got != want {
		t.Fatalf("fx on: status %q; want %q", got, want)
	}
	s := appState(playback.StatusPlaying)
	s.VolumeMode = playback.VolumeSystem
	m, _ = step(t, m, stateMsg{state: s})
	m.status = ""
	if got, want := statusOf(m), "◢◤◢◤ SYS VOLUME // FX ON"; got != want {
		t.Fatalf("system volume: status %q; want %q", got, want)
	}
	// The last song of a list that does not repeat: nothing follows.
	m = onAir(t, playbacktest.New(), "i.3", playback.RepeatOff)
	if got := statusOf(m); strings.Contains(got, "UP NEXT") {
		t.Fatalf("last song without repeat: status %q", got)
	}
}
