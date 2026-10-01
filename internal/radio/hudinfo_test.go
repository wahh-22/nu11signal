package radio

import (
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

// crumbOf is the breadcrumb at the right of the nav bar: the text
// between the rule and its closing "──".
func crumbOf(t *testing.T, m Model) string {
	t.Helper()
	nav := strings.TrimSuffix(strings.TrimRight(navOf(m), " "), "──")
	i := strings.LastIndex(nav, "─ ")
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(nav[i+len("─ "):])
}

func TestTheNavBarNamesWhereYouAre(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.SongAlbumResult = discovery()
	root := loaded(t, f, newClock())

	check := func(name string, m Model, want string) {
		t.Helper()
		if got := crumbOf(t, m); got != want {
			t.Errorf("%s: breadcrumb %q; want %q (nav %q)", name, got, want, navOf(m))
		}
		if strings.Contains(navOf(m), "RDO-77") {
			t.Errorf("%s: the nav bar still shows the serial code: %q", name, navOf(m))
		}
	}
	check("root", root, "PLAYLISTS")
	page := openStation(t, root, 0)
	check("library playlist", page, "PLAYLISTS // NIGHT DRIVE")
	page.setCursor(1) // the first track
	picker, _ := press(t, page, "a")
	check("picker", picker, "PLAYLISTS // ADD TO PLAYLIST")

	search, _ := press(t, root, "/")
	check("search", search, "SEARCH")
	found := searchFor(t, root, "daft")
	results, _ := press(t, found, "enter")
	check("results", results, "SEARCH // RESULTS")
	artist, _ := press(t, found, "down", "down", "down", "enter")
	check("artist", artist, "SEARCH // DAFT PUNK")
	check("song", openSong(t, f, 0), "SEARCH // DISCOVERY")
}

func TestTheBreadcrumbIsCutToFit(t *testing.T) {
	f := playbacktest.New()
	detail := nightDrive()
	detail.Playlist.Name = "Songs For The Longest Drive Through Night City Ever"
	f.LibraryPlaylistResult = detail
	m := openStation(t, loaded(t, f, newClock()), 0)
	nav := navOf(m)
	if w := ansi.StringWidth(nav); w > 80 {
		t.Fatalf("nav %d cells: %q", w, nav)
	}
	if !strings.Contains(nav, "PLAYLISTS // SONGS FOR") || !strings.Contains(nav, "…") || !strings.HasSuffix(strings.TrimRight(nav, " "), "──") {
		t.Fatalf("nav %q; want the breadcrumb cut with … inside its frame", nav)
	}
	// With no room left, the breadcrumb goes, the bar stays whole.
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 44, Height: 14})
	if nav := navOf(m); strings.Contains(nav, "PLAYLISTS //") || !strings.Contains(nav, "BACK") {
		t.Fatalf("narrow nav %q; want the tabs without the breadcrumb", nav)
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
	if got, want := statusOf(m), "░▒▓ UP NEXT // RESONANCE · HOME"; got != want {
		t.Fatalf("status %q; want %q", got, want)
	}
	m = onAir(t, playbacktest.New(), "i.2", playback.RepeatOff)
	if got, want := statusOf(m), "░▒▓ UP NEXT // TURBO KILLER · CARPENTER BRUT"; got != want {
		t.Fatalf("status %q; want %q", got, want)
	}
	// On the last song NEXT starts the list over while it repeats.
	m = onAir(t, playbacktest.New(), "i.3", playback.RepeatAll)
	if got, want := statusOf(m), "░▒▓ UP NEXT // NIGHTCALL · KAVINSKY"; got != want {
		t.Fatalf("last song: status %q; want %q", got, want)
	}
}

func TestUpNextSkipsTheSongsLeftOutOfTheQueue(t *testing.T) {
	f := playbacktest.New()
	f.PlaySongsReport = playback.QueueReport{Missing: []string{"i.2"}}
	m := onAir(t, f, "i.1", playback.RepeatOff)
	// The skipped notice holds the line first.
	m.status = ""
	if got, want := statusOf(m), "░▒▓ UP NEXT // TURBO KILLER · CARPENTER BRUT"; got != want {
		t.Fatalf("status %q; want %q", got, want)
	}
}

func TestTheStatusLineFallsBackToTheVolumeAndEffects(t *testing.T) {
	// A song the model did not queue: nothing known to follow it.
	m := playingModel(t, playbacktest.New())
	if got, want := statusOf(m), "░▒▓ FX OFF"; got != want {
		t.Fatalf("no volume mode: status %q; want %q", got, want)
	}
	m, _, _ = appPlaying(t)
	if got, want := statusOf(m), "░▒▓ APP VOLUME // FX OFF"; got != want {
		t.Fatalf("app volume: status %q; want %q", got, want)
	}
	m, _ = press(t, m, "x")
	m.status = ""
	if got, want := statusOf(m), "░▒▓ APP VOLUME // FX ON"; got != want {
		t.Fatalf("fx on: status %q; want %q", got, want)
	}
	s := appState(playback.StatusPlaying)
	s.VolumeMode = playback.VolumeSystem
	m, _ = step(t, m, stateMsg{state: s})
	m.status = ""
	if got, want := statusOf(m), "░▒▓ SYS VOLUME // FX ON"; got != want {
		t.Fatalf("system volume: status %q; want %q", got, want)
	}
	// The last song of a list that does not repeat: nothing follows.
	m = onAir(t, playbacktest.New(), "i.3", playback.RepeatOff)
	if got := statusOf(m); strings.Contains(got, "UP NEXT") {
		t.Fatalf("last song without repeat: status %q", got)
	}
}
