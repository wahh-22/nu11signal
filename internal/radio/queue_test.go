package radio

import (
	"reflect"
	"testing"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

// A song picked from a list plays with the rest of that list queued after
// it: the queue is the list, started at the song.

func TestSearchEnterOnSongPlaysTheSearchSongsFromIt(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	r := &fakeRecents{}
	m := searchFor(t, loadedWithRecents(t, f, r), "daft")
	m, _ = press(t, m, "down", "down", "down", "down", "down") // the second song, Digital Love
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)

	if calls := callsOf(f, "PlaySongs"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{[]string{"s1", "s2"}, 1}) {
		t.Fatalf("PlaySongs calls = %v; want the search songs from s2", calls)
	}
	if want := []viewKind{viewStations, viewSearch}; !reflect.DeepEqual(stackKinds(m), want) {
		t.Fatalf("stack %v; want SEARCH kept on top", stackKinds(m))
	}
	if got := r.Added(); !reflect.DeepEqual(got, []string{"daft"}) {
		t.Fatalf("recents added = %q; want [daft]", got)
	}
}

func TestClickOnSearchSongPlaysTheSearchSongsFromIt(t *testing.T) {
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, cmd := click(t, m, rowZone(3)) // suggestions, artist, One More Time
	settle(t, m, cmd)
	if calls := callsOf(f, "PlaySongs"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{[]string{"s1", "s2"}, 0}) {
		t.Fatalf("PlaySongs calls = %v; want the search songs from s1", calls)
	}
}

func TestResultsSongsPlayTheirSectionFromThePickedSong(t *testing.T) {
	withSongs := func(res playback.SearchResults) playback.SearchResults {
		res.Songs = append(res.Songs, playback.Song{ID: "s3", Title: "Around the World", Artist: "Daft Punk"})
		return res
	}
	tests := []struct {
		name string
		// top replaces the top song, when set.
		top       *playback.Song
		pick      func([]playback.SearchItem) int
		wantIDs   []string
		wantStart int
	}{
		{"songs section", nil, firstOf(playback.ItemSong, 8), []string{"s1", "s2", "s3"}, 1},
		// A top song among SONGS plays that section from it.
		{"top song in the section", nil, firstOf(playback.ItemSong, 0), []string{"s1", "s2", "s3"}, 0},
		// A top song missing from SONGS plays first, the section after it.
		{"top song alone", &playback.Song{ID: "s9", Title: "Veridis Quo", Artist: "Daft Punk"}, firstOf(playback.ItemSong, 0), []string{"s9", "s1", "s2", "s3"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playbacktest.New()
			res := withSongs(fullCatalog())
			if tt.top != nil {
				res.Top[1].Song = *tt.top
			}
			f.SearchCatalogResult = res
			m := searchFor(t, loadedWithRecents(t, f, &fakeRecents{}), "daft")
			m, cmd := press(t, m, "down", "enter")
			m = settle(t, m, cmd)
			target := tt.pick(m.resultItems())
			for m.cursor() < target {
				m, _ = press(t, m, "down")
			}
			m, cmd = press(t, m, "enter")
			m = settle(t, m, cmd)

			if calls := callsOf(f, "PlaySongs"); len(calls) != 1 || !reflect.DeepEqual(calls[0].Args, []any{tt.wantIDs, tt.wantStart}) {
				t.Fatalf("PlaySongs calls = %v; want %v from %d", calls, tt.wantIDs, tt.wantStart)
			}
			if m.top().kind != viewResults {
				t.Fatalf("top %v; want RESULTS kept", m.top().kind)
			}
		})
	}
}

func TestAlbumKeyOpensTheSongViewOfASongRow(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		f := playbacktest.New()
		f.SearchCatalogResult = catalog()
		f.SongAlbumResult = discovery()
		m := searchFor(t, loaded(t, f, newClock()), "daft")
		m, _ = press(t, m, "down", "down", "down", "down", "down")
		m, cmd := press(t, m, keyAlbum)
		m = settle(t, m, cmd)
		if m.top().kind != viewAlbum || m.top().tracks.title() != "SONG" || m.cursor() != 2 {
			t.Fatalf("top %v %q cursor %d; want the SONG view on Digital Love", m.top().kind, m.top().tracks.title(), m.cursor())
		}
		if len(callsOf(f, "PlaySongs")) != 0 {
			t.Fatal("opening the song's album played it")
		}
		// Enter there plays the whole album from the song.
		m, cmd = press(t, m, "enter")
		settle(t, m, cmd)
		assertCall(t, f, "PlaySongs", []string{"s1", "s4", "s2", "s5"}, 2)
	})
	t.Run("results", func(t *testing.T) {
		f := playbacktest.New()
		f.SongAlbumResult = discovery()
		m := openResults(t, f, &fakeRecents{})
		target := firstOf(playback.ItemSong, 8)(m.resultItems())
		for m.cursor() < target {
			m, _ = press(t, m, "down")
		}
		m, cmd := press(t, m, keyAlbum)
		m = settle(t, m, cmd)
		if m.top().kind != viewAlbum || m.top().tracks.title() != "SONG" {
			t.Fatalf("top %v; want the SONG view", m.top().kind)
		}
		if calls := callsOf(f, "SongAlbum"); len(calls) != 1 || calls[0].Args[0] != "s2" {
			t.Fatalf("SongAlbum calls = %v; want one for s2", calls)
		}
	})
	t.Run("not a song", func(t *testing.T) {
		f := playbacktest.New()
		m := openResults(t, f, &fakeRecents{}) // on the top artist
		m, _ = press(t, m, keyAlbum)
		if m.top().kind != viewResults || len(callsOf(f, "SongAlbum")) != 0 {
			t.Fatalf("album key on an artist row opened %v", m.top().kind)
		}
	})
}
