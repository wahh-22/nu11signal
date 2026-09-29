package radio

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wahh-22/nu11signal/internal/playback"
	"github.com/wahh-22/nu11signal/internal/playback/playbacktest"
)

func TestCleanLineRemovesControlCharacters(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain text", "Daft Punk — 宇多田ヒカル", "Daft Punk — 宇多田ヒカル"},
		{"escape sequence", "red\x1b[31malert", "red[31malert"},
		{"osc title", "x\x1b]0;pwned\x07y", "x]0;pwnedy"},
		{"c1 controls and del", "a\u009bb\u0085c\x7fd", "ab cd"},
		{"control whitespace", "one\ttwo\nthree\r", "one two three "},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanLine(tt.in); got != tt.want {
				t.Errorf("cleanLine(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanTextKeepsNewlines(t *testing.T) {
	if got, want := cleanText("first\x1b[2J\nsecond\tline\x00"), "first[2J\nsecond line"; got != want {
		t.Errorf("cleanText = %q; want %q", got, want)
	}
}

func TestCleanDetailsCleanEveryTextFieldAndKeepNils(t *testing.T) {
	esc := "\x1b[5m"
	d := cleanAlbumDetail(playback.AlbumDetail{
		Album:  playback.Album{ID: "al1", Title: "A" + esc, Artist: "B" + esc},
		Tracks: []playback.Track{{Song: playback.Song{ID: "s1", Title: "C" + esc, Artist: "D" + esc, Album: "E" + esc}, Number: 1}},
		Genre:  "F" + esc, ReleaseDate: "G" + esc, RecordLabel: "H" + esc, Copyright: "I" + esc, Notes: "J\n" + esc,
	})
	for _, s := range []string{d.Album.Title, d.Album.Artist, d.Tracks[0].Title, d.Tracks[0].Artist, d.Tracks[0].Album, d.Genre, d.ReleaseDate, d.RecordLabel, d.Copyright, d.Notes} {
		if strings.ContainsRune(s, '\x1b') {
			t.Errorf("field %q kept an escape", s)
		}
	}
	if d.Notes != "J\n[5m" || d.Tracks[0].ID != "s1" || d.Tracks[0].Number != 1 {
		t.Errorf("cleaned detail = %+v", d)
	}
	if got := cleanAlbumDetail(playback.AlbumDetail{}); !reflect.DeepEqual(got, playback.AlbumDetail{}) {
		t.Errorf("empty detail = %+v; want nil lists kept", got)
	}
	p := cleanPlaylistDetail(playback.PlaylistDetail{
		Playlist: playback.CatalogPlaylist{Name: "N" + esc, Curator: "C" + esc},
		Tracks:   []playback.Song{{Title: "T" + esc}},
		Notes:    "X" + esc,
	})
	if p.Playlist.Name != "N[5m" || p.Playlist.Curator != "C[5m" || p.Tracks[0].Title != "T[5m" || p.Notes != "X[5m" {
		t.Errorf("cleaned playlist = %+v", p)
	}
	a := cleanArtistDetail(playback.ArtistDetail{
		Artist:    playback.Artist{Name: "N" + esc, Genres: []string{"G" + esc}},
		TopSongs:  []playback.Song{{Title: "T" + esc}},
		Albums:    []playback.Album{{Title: "A" + esc}},
		Playlists: []playback.CatalogPlaylist{{Name: "P" + esc}},
		About:     playback.ArtistAbout{Notes: "1\n2" + esc, Genre: "G" + esc, Origin: "O" + esc, Formed: "F" + esc},
	})
	if a.Artist.Name != "N[5m" || a.Artist.Genres[0] != "G[5m" || a.TopSongs[0].Title != "T[5m" || a.Albums[0].Title != "A[5m" ||
		a.Playlists[0].Name != "P[5m" || a.About != (playback.ArtistAbout{Notes: "1\n2[5m", Genre: "G[5m", Origin: "O[5m", Formed: "F[5m"}) {
		t.Errorf("cleaned artist = %+v", a)
	}
}

// TestCatalogTextNeverReachesTheTerminalRaw feeds control sequences through
// every catalog answer and the player state; none may survive to the view.
func TestCatalogTextNeverReachesTheTerminalRaw(t *testing.T) {
	const evil = "\x1b]0;pwned\x07"
	f := playbacktest.New()
	f.SearchCatalogResult = catalog()
	f.SearchCatalogResult.Songs[0].Title += evil
	f.SongAlbumResult = discovery()
	f.SongAlbumResult.Album.Title += evil
	f.SongAlbumResult.Notes += evil
	m := searchFor(t, loaded(t, f, newClock()), "daft")
	m, _ = step(t, m, stateMsg{state: playback.State{Status: playback.StatusPlaying, Title: "Now" + evil, Artist: "A" + evil}})
	if strings.Contains(m.render(), "\x07") {
		t.Fatal("search results or player state reached the terminal with a control character")
	}
	m, _ = press(t, m, "down", "down", "down", "down")
	m, cmd := press(t, m, "enter")
	m = settle(t, m, cmd)
	if view := m.render(); strings.Contains(view, "\x07") || !strings.Contains(plain(m), "DISCOVERY]0;PWNED") {
		t.Fatalf("album page not cleaned:\n%q", plain(m))
	}
	m.setStatus("boom" + evil)
	if strings.Contains(m.render(), "\x07") {
		t.Fatal("status line reached the terminal with a control character")
	}
}
