package radio

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// nightDrive is the page of the first library playlist: library ids, no
// curator.
func nightDrive() playback.PlaylistDetail {
	song := func(id, title, artist string, secs int) playback.Song {
		return playback.Song{ID: id, Title: title, Artist: artist, Duration: time.Duration(secs) * time.Second}
	}
	return playback.PlaylistDetail{
		Playlist: playback.CatalogPlaylist{ID: "pl-1", Name: "Night Drive"},
		Tracks: []playback.Song{
			song("i.1", "Nightcall", "Kavinsky", 258),
			song("i.2", "Resonance", "Home", 212),
			song("i.3", "Turbo Killer", "Carpenter Brut", 229),
		},
	}
}

// openStation opens the library playlist at row of the PLAYLISTS root,
// letting its tracks load; the fake answers with nightDrive() unless it
// was given another page.
func openStation(t *testing.T, m Model, row int) Model {
	t.Helper()
	if f, ok := m.player.(*playbacktest.Fake); ok && len(f.LibraryPlaylistResult.Tracks) == 0 {
		f.LibraryPlaylistResult = nightDrive()
	}
	for len(m.stack) > 1 {
		m, _ = press(t, m, "esc")
	}
	m.setStationCursor(row)
	m, cmd := press(t, m, "enter")
	return settle(t, m, cmd)
}

// tune opens the library playlist at row and presses its ▶ PLAY row; cmd
// is the play request.
func tune(t *testing.T, m Model, row int) (Model, tea.Cmd) {
	t.Helper()
	m = openStation(t, m, row)
	m.setCursor(0)
	return press(t, m, "enter")
}

func TestRootIsNamedPlaylists(t *testing.T) {
	m := loaded(t, playbacktest.New(), newClock())
	view := plain(m)
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[1], "╱ PLAYLISTS ╱") && !strings.Contains(lines[1], "PLAYLISTS") {
		t.Fatalf("nav line lacks the PLAYLISTS tab: %q", lines[1])
	}
	if !strings.Contains(lines[2], "PLAYLISTS") {
		t.Fatalf("panel title lacks PLAYLISTS: %q", lines[2])
	}
	if strings.Contains(view, "STATION") {
		t.Fatalf("view still says STATION:\n%s", view)
	}
	// The FM dial stays: the playlists keep their frequencies.
	if !strings.Contains(view, "088.1") {
		t.Fatalf("view lost the FM dial:\n%s", view)
	}
	if got := hintsOf(m); !strings.Contains(got, "[ENTER] OPEN") {
		t.Fatalf("footer %q; want enter to open", got)
	}
	m, _ = press(t, m, "/")
	if got := hintsOf(m); !strings.Contains(got, "[TAB] PLAYLISTS") {
		t.Fatalf("search footer %q; want tab to name PLAYLISTS", got)
	}
}

func TestEnterOnAPlaylistOpensItsTracks(t *testing.T) {
	f := playbacktest.New()
	f.LibraryPlaylistResult = nightDrive()
	m := loaded(t, f, newClock())
	m, cmd := press(t, m, "enter")
	if !slices.Equal(stackKinds(m), []viewKind{viewStations, viewPlaylist}) {
		t.Fatalf("stack %v; want the playlist page over the root", stackKinds(m))
	}
	if view := plain(m); !strings.Contains(view, "DECRYPTING PLAYLIST FEED") || !strings.Contains(view, "NIGHT DRIVE") {
		t.Fatalf("loading page lacks the notice or the name:\n%s", view)
	}
	m = settle(t, m, cmd)
	if calls := callsOf(f, "LibraryPlaylist"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{"pl-1"}) {
		t.Fatalf("LibraryPlaylist calls = %v; want one for pl-1", calls)
	}
	for _, method := range []string{"PlayPlaylist", "PlayPlaylistFrom", "PlaySongs", "CatalogPlaylist"} {
		if n := len(callsOf(f, method)); n != 0 {
			t.Fatalf("opening the playlist called %s", method)
		}
	}
	body := trackRows(m)
	for _, want := range []string{"▶ PLAY", "1  NIGHTCALL · KAVINSKY", "3  TURBO KILLER", "3 SONGS"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page lacks %q:\n%s", want, body)
		}
	}
	if m.cursor() != 0 {
		t.Fatalf("cursor %d; want the PLAY row", m.cursor())
	}
	view := plain(m)
	if !strings.Contains(strings.Split(view, "\n")[2], "PLAYLIST") {
		t.Fatalf("panel title lacks PLAYLIST:\n%s", view)
	}
	_, zs := m.layout()
	if !hasZone(zs, zoneBack) {
		t.Fatal("the playlist page offers no BACK")
	}
	// The PLAYLISTS tab stays lit: the page belongs to that branch.
	for _, b := range m.navButtons() {
		if want := b.id == zoneTabStations; b.active != want {
			t.Fatalf("tab %s active %v; want %v", b.id, b.active, want)
		}
	}

	m, _ = press(t, m, "esc")
	if len(m.stack) != 1 || m.stationCursor() != 0 {
		t.Fatalf("esc: stack %v cursor %d; want the root on the playlist", stackKinds(m), m.stationCursor())
	}
}

func TestLibraryPlaylistEnterPlaysFromTheTrack(t *testing.T) {
	f := playbacktest.New()
	m := openStation(t, loaded(t, f, newClock()), 1)
	m, cmd := press(t, m, "down", "down", "enter") // the second track
	m = settle(t, m, cmd)
	// The page's songs are played by id, from the ones it loaded: the
	// playlist is not read again inside the playback budget.
	assertCall(t, f, "PlaySongs", []string{"i.1", "i.2", "i.3"}, 1)
	for _, method := range []string{"PlayPlaylist", "PlayPlaylistFrom"} {
		if n := len(callsOf(f, method)); n != 0 {
			t.Fatalf("a library track was played through %s", method)
		}
	}
	if n := len(callsOf(f, "LibraryPlaylist")); n != 1 {
		t.Fatalf("LibraryPlaylist called %d times; want only the page load", n)
	}
	if m.playingStation != "pl-2" {
		t.Fatalf("playingStation %q; want the playlist on air", m.playingStation)
	}
	m, _ = step(t, m, stateMsg{state: playback.State{Status: playback.StatusPlaying, SongID: "i.2", Title: "Resonance", Artist: "Home"}})
	if feed := plain(m); !strings.Contains(feed, "089.7 MHZ") {
		t.Fatalf("feed does not name the playlist's frequency:\n%s", feed)
	}
}

func TestLibraryPlaylistPlayRowPlaysFromTheStart(t *testing.T) {
	f := playbacktest.New()
	m := openStation(t, loaded(t, f, newClock()), 2)
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	assertCall(t, f, "PlaySongs", []string{"i.1", "i.2", "i.3"}, 0)
	if m.playingStation != "pl-3" {
		t.Fatalf("playingStation %q; want the playlist on air", m.playingStation)
	}
}

func TestClickOnALibraryTrackPlaysThePlaylistFromIt(t *testing.T) {
	f := playbacktest.New()
	m := openStation(t, loaded(t, f, newClock()), 0)
	m, cmd := click(t, m, rowZone(3))
	settle(t, m, cmd)
	assertCall(t, f, "PlaySongs", []string{"i.1", "i.2", "i.3"}, 2)
}

func TestLibraryPlaylistFailureOffersRetry(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"LibraryPlaylist": errors.New("playlist gone")}
	m := loaded(t, f, newClock())
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	if view := plain(m); !strings.Contains(view, "[R] RETRY // PLAYLIST GONE") || !strings.Contains(view, "PLAYLIST FEED FAILED") {
		t.Fatalf("failed page lacks the retry or the status:\n%s", view)
	}
	f.MethodErr = nil
	f.LibraryPlaylistResult = nightDrive()
	m, cmd = press(t, m, "r")
	m = settle(t, m, cmd)
	if n := len(callsOf(f, "LibraryPlaylist")); n != 2 {
		t.Fatalf("LibraryPlaylist called %d times; want a retry", n)
	}
	if body := trackRows(m); !strings.Contains(body, "NIGHTCALL") {
		t.Fatalf("retry did not load the tracks:\n%s", body)
	}
}

func TestLibraryPlaylistMarksThePlayingTrack(t *testing.T) {
	m := openStation(t, loaded(t, playbacktest.New(), newClock()), 0)
	for _, s := range []struct {
		name  string
		state playback.State
		want  string
	}{
		{"library id", playback.State{Status: playback.StatusPlaying, SongID: "i.3", Title: "Turbo Killer", Artist: "Carpenter Brut"}, "▶ 3  TURBO KILLER"},
		{"catalog id, same title and artist", playback.State{Status: playback.StatusPlaying, SongID: "123", Title: "Resonance", Artist: "Home"}, "▶ 2  RESONANCE"},
	} {
		m, _ = step(t, m, stateMsg{state: s.state})
		body := trackRows(m)
		if !strings.Contains(body, s.want) || strings.Count(body, "▶") != 2 { // the PLAY row and the track
			t.Fatalf("%s: want %q marked:\n%s", s.name, s.want, body)
		}
	}
}

func TestLeavingALibraryPlaylistPage(t *testing.T) {
	t.Run("tab opens the search", func(t *testing.T) {
		m := openStation(t, loaded(t, playbacktest.New(), newClock()), 0)
		m, _ = press(t, m, "tab")
		if !slices.Equal(stackKinds(m), []viewKind{viewStations, viewSearch}) || len(m.parked) != 0 || !m.input.Focused() {
			t.Fatalf("tab: stack %v parked %d; want a fresh SEARCH", stackKinds(m), len(m.parked))
		}
	})
	t.Run("tab restores a parked search branch", func(t *testing.T) {
		m := openResults(t, playbacktest.New(), &fakeRecents{})
		m, _ = press(t, m, "tab")
		m = openStation(t, m, 0)
		m, _ = press(t, m, "tab")
		if !slices.Equal(stackKinds(m), []viewKind{viewStations, viewSearch, viewResults}) {
			t.Fatalf("tab: stack %v; want the parked RESULTS back", stackKinds(m))
		}
	})
	t.Run("the PLAYLISTS tab goes back to the list", func(t *testing.T) {
		m := openStation(t, loaded(t, playbacktest.New(), newClock()), 1)
		m, _ = click(t, m, zoneTabStations)
		if len(m.stack) != 1 || len(m.parked) != 0 || m.stationCursor() != 1 {
			t.Fatalf("PLAYLISTS tab: stack %v parked %d cursor %d; want the root", stackKinds(m), len(m.parked), m.stationCursor())
		}
	})
	t.Run("up from the top focuses the lit PLAYLISTS tab", func(t *testing.T) {
		m := openStation(t, loaded(t, playbacktest.New(), newClock()), 0)
		m, _ = press(t, m, "up")
		if m.focus != areaTabs || m.tab != 0 {
			t.Fatalf("focus %v tab %d; want PLAYLISTS", m.focus, m.tab)
		}
		m, _ = press(t, m, "right", "right", "enter")
		if len(m.stack) != 1 {
			t.Fatalf("BACK: stack %v; want the root", stackKinds(m))
		}
	})
}

func TestPlaylistGolden80x24(t *testing.T) {
	for _, tt := range []struct {
		name string
		open func(t *testing.T, m Model) Model
	}{
		{"playlists", func(t *testing.T, m Model) Model {
			m, cmd := tune(t, m, 1)
			m = settle(t, m, cmd)
			m, _ = press(t, m, "esc")
			return m
		}},
		{"library_playlist", func(t *testing.T, m Model) Model {
			m = openStation(t, m, 0)
			m, _ = press(t, m, "down", "down")
			return m
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			m := loaded(t, f, newClock())
			m, _ = step(t, m, volumeMsg{level: 0.6})
			m = tt.open(t, m)
			m, _ = step(t, m, stateMsg{state: playback.State{Status: playback.StatusPlaying, SongID: "i.2", Title: "Resonance", Artist: "Home", Album: "Odyssey", Position: 83 * time.Second, Duration: 212 * time.Second}})
			for range 12 {
				m, _ = step(t, m, tickMsg{gen: m.tickGen})
			}
			assertGolden(t, tt.name+"_80x24.golden", ansi.Strip(m.View().Content))
		})
	}
}

// withUpload is nightDrive with its second track only in the library.
func withUpload() playback.PlaylistDetail {
	d := nightDrive()
	d.Tracks[1].LibraryOnly = true
	return d
}

func TestLibraryOnlyTrackIsMutedAndNotPlayed(t *testing.T) {
	f := playbacktest.New()
	f.LibraryPlaylistResult = withUpload()
	m := openStation(t, loaded(t, f, newClock()), 0)

	rows := linesOf(m.trackBody(m.listBodyWidth(), 30))
	styled := func(title string) string {
		for _, r := range rows {
			if strings.Contains(ansi.Strip(r), title) {
				return r
			}
		}
		t.Fatalf("no row for %s", title)
		return ""
	}
	if row := styled("RESONANCE"); !strings.Contains(row, stMuted.Render("RESONANCE")) {
		t.Errorf("library-only track is not muted: %q", row)
	}
	if row := styled("NIGHTCALL"); strings.Contains(row, stMuted.Render("NIGHTCALL")) {
		t.Errorf("catalog track is muted: %q", row)
	}

	before := len(f.Calls())
	m, cmd := press(t, m, "down", "down", "enter") // RESONANCE
	if cmd != nil {
		m = settle(t, m, cmd)
	}
	if calls := f.Calls()[before:]; len(calls) != 0 {
		t.Fatalf("enter on a library-only track called %v", calls)
	}
	if view := plain(m); !strings.Contains(view, "RESONANCE IS NOT IN THE APPLE MUSIC CATALOG") {
		t.Fatalf("no notice for the library-only track:\n%s", view)
	}

	m, cmd = press(t, m, "down", "enter") // TURBO KILLER still plays from itself
	settle(t, m, cmd)
	// RESONANCE is skipped: TURBO KILLER is the second song queued.
	assertCall(t, f, "PlaySongs", []string{"i.1", "i.3"}, 1)
	m.setCursor(0) // ▶ PLAY
	m, cmd = press(t, m, "enter")
	settle(t, m, cmd)
	assertCall(t, f, "PlaySongs", []string{"i.1", "i.3"}, 0)
}

// deadlinePlayer records the deadline of the Playlists call.
type deadlinePlayer struct {
	*playbacktest.Fake
	left time.Duration
}

func (p *deadlinePlayer) Playlists(ctx context.Context) ([]playback.Playlist, error) {
	if d, ok := ctx.Deadline(); ok {
		p.left = time.Until(d)
	}
	return p.Fake.Playlists(ctx)
}

func TestPlaylistsGetTheDetailTimeout(t *testing.T) {
	// The helper pages through the Apple Music API for the playlists,
	// within CatalogBudget.libraryRead; the call waits as long as a page.
	p := &deadlinePlayer{Fake: playbacktest.New()}
	m := New(p, Options{Now: newClock().now, Seed: 2077})
	run(t, m.loadPlaylistsCmd())
	if p.left <= defaultCallTimeout || p.left > detailCallTimeout {
		t.Fatalf("Playlists deadline %v away; want the detail timeout %v", p.left, detailCallTimeout)
	}
}
