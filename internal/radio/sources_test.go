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

// sourced is a Fake with sources: local ids support none of the catalog
// capabilities and, with localOnly, nothing does (no Apple Music). Its
// LibraryChanged is changed.
type sourced struct {
	*playbacktest.Fake
	localOnly bool
	changed   chan struct{}
}

func (s *sourced) Supports(c playback.Capability, id string) bool {
	if s.localOnly {
		return false
	}
	return id == "" || playback.SourceOf(id) != playback.SourceLocal
}

func (s *sourced) LibraryChanged() <-chan struct{} { return s.changed }

func localStations() []playback.Playlist {
	return []playback.Playlist{
		{ID: "local:pl:1", Name: "Vinyl Rips", Source: playback.SourceLocal},
		{ID: "local:pl:2", Name: "Demos", Source: playback.SourceLocal},
	}
}

func localTape() playback.PlaylistDetail {
	return playback.PlaylistDetail{
		Playlist: playback.CatalogPlaylist{ID: "local:pl:1", Name: "Vinyl Rips"},
		Tracks: []playback.Song{
			{ID: "local:a", Title: "Side A", Artist: "Tape", Duration: 200 * time.Second},
			{ID: "local:b", Title: "Side B", Artist: "Tape", Duration: 180 * time.Second},
		},
	}
}

// loadedSourced is loaded for a sourced player listing pls.
func loadedSourced(t *testing.T, localOnly bool, pls []playback.Playlist) (Model, *sourced) {
	t.Helper()
	p := &sourced{Fake: playbacktest.New(), localOnly: localOnly, changed: make(chan struct{}, 1)}
	p.PlaylistsResult = pls
	p.LibraryPlaylistResult = localTape()
	m := New(p, Options{SkipBoot: true, Now: newClock().now, Seed: 2077})
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := step(t, m, run(t, m.authorizeCmd()))
	m, _ = step(t, m, run(t, cmd))
	return m, p
}

func mixedStations() []playback.Playlist { return append(stations(), localStations()...) }

func calls(f *playbacktest.Fake, method string) int {
	n := 0
	for _, c := range f.Calls() {
		if c.Method == method {
			n++
		}
	}
	return n
}

func TestPlaylistsShowALocalSectionAfterApple(t *testing.T) {
	m, _ := loadedSourced(t, false, mixedStations())
	lines := strings.Split(plain(m), "\n")
	at := func(s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		t.Fatalf("view lacks %q:\n%s", s, strings.Join(lines, "\n"))
		return -1
	}
	head := at(spaced("LOCAL"))
	if !(at("BODY HEAT") < head && head < at("VINYL RIPS") && at("VINYL RIPS") < at("DEMOS")) {
		t.Errorf("the LOCAL section is not between the Apple and the local playlists:\n%s", strings.Join(lines, "\n"))
	}
	// The header is not a row: ↓ from BODY HEAT selects VINYL RIPS.
	m.setStationCursor(2)
	m, _ = press(t, m, "down")
	if got := m.stations[m.stationCursor()].Name; got != "Vinyl Rips" {
		t.Errorf("↓ from the last Apple playlist selected %q", got)
	}
}

func TestLocalSectionKeepsTheCursorOnScreen(t *testing.T) {
	pls := stations()
	for i := 0; i < 20; i++ {
		pls = append(pls, playback.Playlist{ID: "local:pl:x" + string(rune('a'+i)), Name: "Tape " + string(rune('A'+i)), Source: playback.SourceLocal})
	}
	m, _ := loadedSourced(t, false, pls)
	for i := range pls {
		m.setStationCursor(i)
		rows, _ := m.stationRows(40, 8)
		found := false
		for _, r := range rows {
			if strings.Contains(ansi.Strip(r), "▌▶") {
				found = true
			}
		}
		if !found {
			t.Fatalf("cursor %d is off screen: %q", i, rows)
		}
	}
}

func TestLocalSongsHaveNoLoveOrAdd(t *testing.T) {
	m, p := loadedSourced(t, false, mixedStations())
	m = openStation(t, m, 3)
	m.setCursor(1) // the first song, under ▶ PLAY
	if s, ok := m.selectedSong(); !ok || s.ID != "local:a" {
		t.Fatalf("selected %+v, %v", s, ok)
	}
	view := plain(m)
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "SIDE A") && (strings.Contains(l, favoriteOffMark) || strings.Contains(l, favoriteOnMark) || strings.Contains(l, " + ")) {
			t.Errorf("a local song row shows the love and add controls: %q", l)
		}
	}
	m, _ = press(t, m, "l")
	if strings.Contains(m.status, "APPLE MUSIC") || !strings.Contains(m.status, "LOCAL FILE") {
		t.Errorf("love on a local song: status %q", m.status)
	}
	m, _ = press(t, m, "a")
	if m.editor.mode != editClosed {
		t.Error("add on a local song opened the picker")
	}
	if strings.Contains(m.status, "APPLE MUSIC") || !strings.Contains(m.status, "LOCAL FILE") {
		t.Errorf("add on a local song: status %q", m.status)
	}
	for i := 0; i < 5; i++ {
		m, _ = step(t, m, tickMsg{gen: m.tickGen})
	}
	if n := calls(p.Fake, "SetFavorite") + calls(p.Fake, "Favorite") + calls(p.Fake, "Favorites"); n != 0 {
		t.Errorf("%d favorite calls for local songs", n)
	}
}

func TestNoHeartWhileALocalSongPlays(t *testing.T) {
	m, _ := loadedSourced(t, false, mixedStations())
	s := playing(10*time.Second, 200*time.Second)
	s.SongID = "local:a"
	m, _ = step(t, m, stateMsg{state: s})
	if m.drawn(zoneFavPlaying) {
		t.Error("NOW PLAYING draws the favorite of a local song")
	}
	if feed := ansi.Strip(m.feedLine()); !strings.Contains(feed, "LOCAL FEED") {
		t.Errorf("feed of a local song = %q, want LOCAL FEED", feed)
	}
	s.SongID = "c1"
	m, _ = step(t, m, stateMsg{state: s})
	if !m.drawn(zoneFavPlaying) {
		t.Error("NOW PLAYING lost the favorite of an Apple song")
	}
}

func TestLocalOnlyHidesSearchAndPlaylistEditing(t *testing.T) {
	m, _ := loadedSourced(t, true, localStations())
	view := plain(m)
	lines := strings.Split(view, "\n")
	if strings.Contains(lines[1], "SEARCH") {
		t.Errorf("nav line shows SEARCH without a catalog: %q", lines[1])
	}
	if strings.Contains(view, "NEW PLAYLIST") {
		t.Errorf("+ NEW PLAYLIST shows without playlist editing:\n%s", view)
	}
	if strings.Contains(hintsOf(m), "SCAN") {
		t.Errorf("footer offers the search: %q", hintsOf(m))
	}
	for _, k := range []string{"/", "tab"} {
		next, _ := press(t, m, k)
		if next.top().kind != viewStations {
			t.Errorf("%s opened view %v without a catalog", k, next.top().kind)
		}
	}
	if m.stationCursor() != 0 {
		t.Errorf("cursor %d, want the first playlist", m.stationCursor())
	}
}

func TestLibraryChangeReloadsThePlaylists(t *testing.T) {
	m, p := loadedSourced(t, true, nil)
	p.PlaylistsResult = localStations()
	p.changed <- struct{}{}
	m, cmd := step(t, m, run(t, m.waitLibrary()))
	// Closed, the re-armed wait answers at once (and ends the watch).
	close(p.changed)
	for _, msg := range runAll(t, cmd) {
		m, _ = step(t, m, msg)
	}
	found := false
	for _, s := range m.stations {
		if s.ID == "local:pl:1" {
			found = true
		}
	}
	if !found {
		t.Errorf("stations after a library change = %+v", m.stations)
	}
}

func TestSourcesGolden80x24(t *testing.T) {
	for _, tt := range []struct {
		name      string
		localOnly bool
		pls       []playback.Playlist
	}{
		{"local_section", false, mixedStations()},
		{"local_only", true, localStations()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := loadedSourced(t, tt.localOnly, tt.pls)
			s := playing(83*time.Second, 225*time.Second)
			s.SongID = "local:a"
			m, _ = step(t, m, stateMsg{state: s})
			for i := 0; i < 12; i++ {
				m, _ = step(t, m, tickMsg{gen: m.tickGen})
			}
			assertGolden(t, tt.name+"_80x24.golden", ansi.Strip(m.View().Content))
		})
	}
}
