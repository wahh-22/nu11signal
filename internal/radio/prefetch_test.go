package radio

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// favoritesCalls are the id lists Favorites was called with, in order.
func favoritesCalls(f *playbacktest.Fake) [][]string {
	var out [][]string
	for _, c := range callsOf(f, "Favorites") {
		out = append(out, c.Args[0].([]string))
	}
	return out
}

func TestAPageReadsTheFavoritesOfAllItsSongsInOneCall(t *testing.T) {
	// want is the page's read: its songs not known from the pages before
	// it (SEARCH read s1 and s2, the artist page s3).
	tests := []struct {
		name string
		open func(*testing.T, *playbacktest.Fake) Model
		want []string
	}{
		{"song", func(t *testing.T, f *playbacktest.Fake) Model { return openSong(t, f, 1) }, []string{"s4", "s5"}},
		{"album", func(t *testing.T, f *playbacktest.Fake) Model { return openFromArtist(t, f, itemAlbum) }, []string{"s4", "s5"}},
		{"catalog playlist", func(t *testing.T, f *playbacktest.Fake) Model { return openFromArtist(t, f, itemPlaylist) }, []string{"s6", "s7"}},
		// The library-only song has no catalog id to rate.
		{"library playlist", func(t *testing.T, f *playbacktest.Fake) Model {
			f.LibraryPlaylistResult = withUpload()
			return openStation(t, loaded(t, f, newClock()), 0)
		}, []string{"i.1", "i.3"}},
		{"artist top songs", func(t *testing.T, f *playbacktest.Fake) Model { return openDaftPunk(t, f) }, []string{"s3"}},
		{"search", func(t *testing.T, f *playbacktest.Fake) Model {
			f.SearchCatalogResult = catalog()
			return searchFor(t, loaded(t, f, newClock()), "daft")
		}, []string{"s1", "s2"}},
		// The top song is among SONGS: read once.
		{"results", func(t *testing.T, f *playbacktest.Fake) Model { return openResults(t, f, &fakeRecents{}) }, []string{"s1", "s2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			f.Loved = map[string]bool{tt.want[len(tt.want)-1]: true}
			m := tt.open(t, f)
			calls := favoritesCalls(f)
			if len(calls) == 0 {
				t.Fatal("no Favorites call for the page")
			}
			if got := calls[len(calls)-1]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Favorites(%v); want %v", got, tt.want)
			}
			// Hearts show without waiting for a tick.
			if on, known := m.favoriteOf(tt.want[len(tt.want)-1]); !on || !known {
				t.Fatalf("loved song known %v on %v; want loved", known, on)
			}
			if len(tt.want) > 1 {
				if on, known := m.favoriteOf(tt.want[0]); on || !known {
					t.Fatalf("first song known %v on %v; want known, not loved", known, on)
				}
			}
			// The tick has nothing left to read on the page.
			m, _ = step(t, m, tickMsg{gen: m.tickGen})
			if n := len(callsOf(f, "Favorite")); n != 0 {
				t.Fatalf("the tick read %d favorites one by one", n)
			}
		})
	}
}

func TestKnownFavoritesAreNotReadAgainByTheNextPage(t *testing.T) {
	f := playbacktest.New()
	m := openSong(t, f, 1)
	m, _ = press(t, m, "esc")
	f.CatalogPlaylistResult = essentials()
	m, cmd := m.openPlaylist(essentials().Playlist)
	settle(t, m, cmd)
	calls := favoritesCalls(f)
	// s1 is on both pages: known from the album.
	if got := calls[len(calls)-1]; !reflect.DeepEqual(got, []string{"s3", "s6", "s7"}) {
		t.Fatalf("Favorites(%v); want only the songs not known yet", got)
	}
}

func TestAFailedPageReadLeavesTheTickToReadSongBySong(t *testing.T) {
	f := playbacktest.New()
	f.MethodErr = map[string]error{"Favorites": errors.New("offline")}
	f.Loved = map[string]bool{"s2": true}
	m := openSong(t, f, 1) // DIGITAL LOVE (s2), selected
	if _, known := m.favoriteOf("s2"); known {
		t.Fatal("a failed read made a state known")
	}
	if strings.Contains(m.status, "OFFLINE") {
		t.Fatalf("status %q; a failed prefetch should stay quiet", m.status)
	}
	// The fallback: the tick reads the selected song on its own.
	m, cmd := step(t, m, tickMsg{gen: m.tickGen})
	m = settle(t, m, cmd)
	if calls := callsOf(f, "Favorite"); len(calls) != 1 || calls[0].Args[0] != "s2" {
		t.Fatalf("Favorite calls = %v; want the selected s2", calls)
	}
	if on, _ := m.favoriteOf("s2"); !on {
		t.Fatal("the fallback read did not show s2 loved")
	}
}

func TestThePlayingSongOffThePageIsReadByTheTick(t *testing.T) {
	f := playbacktest.New()
	f.Loved = map[string]bool{"c1": true}
	m := openSong(t, f, 1)
	m, _ = step(t, m, stateMsg{state: playing(0, 0)}) // Hollow Wire, c1
	m, cmd := step(t, m, tickMsg{gen: m.tickGen})
	m = settle(t, m, cmd)
	if calls := callsOf(f, "Favorite"); len(calls) != 1 || calls[0].Args[0] != "c1" {
		t.Fatalf("Favorite calls = %v; want the playing c1", calls)
	}
	if on, _ := m.favoriteOf("c1"); !on {
		t.Fatal("the playing song's favorite is not known")
	}
}

func TestAStalePageReadsNoFavorites(t *testing.T) {
	// Answers to pages no longer waited for (closed, or loaded again)
	// must not read favorites for songs nobody sees.
	song := playback.Song{ID: "s9", Title: "Gone"}
	for _, tt := range []struct {
		name string
		msg  any
	}{
		{"artist", artistMsg{seq: 99, detail: playback.ArtistDetail{TopSongs: []playback.Song{song}}}},
		{"album", albumMsg{seq: 99, detail: playback.AlbumDetail{Tracks: []playback.Track{{Song: song}}}}},
		{"playlist", playlistMsg{seq: 99, detail: playback.PlaylistDetail{Tracks: []playback.Song{song}}}},
		{"results", resultsMsg{seq: 99, found: playback.SearchResults{Songs: []playback.Song{song}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, playbacktest.New(), newClock())
			m, cmd := step(t, m, tt.msg)
			if cmd != nil || m.favs[song.ID].reading {
				t.Fatalf("a stale %s page read favorites", tt.name)
			}
		})
	}
}
