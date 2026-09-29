package radio

import (
	"strings"
	"unicode"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Catalog text (titles, names, notes) and player state come from the
// network. A control character in them, such as ESC, would reach the
// terminal as a control sequence, so every answer is cleaned when it
// arrives, before anything renders it.

// cleanLine makes text safe for one terminal line: control characters (C0,
// DEL and C1) are removed, except control whitespace such as a tab or a
// newline, which becomes a space.
func cleanLine(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case !unicode.IsControl(r):
			return r
		case unicode.IsSpace(r):
			return ' '
		}
		return -1
	}, s)
}

// cleanText is cleanLine for text that wraps over several lines: newlines
// stay.
func cleanText(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = cleanLine(l)
	}
	return strings.Join(lines, "\n")
}

// cleanEach cleans every element of xs, keeping a nil list nil.
func cleanEach[T any](xs []T, clean func(T) T) []T {
	if xs == nil {
		return nil
	}
	out := make([]T, len(xs))
	for i, x := range xs {
		out[i] = clean(x)
	}
	return out
}

func cleanSong(s playback.Song) playback.Song {
	s.Title, s.Artist, s.Album = cleanLine(s.Title), cleanLine(s.Artist), cleanLine(s.Album)
	return s
}

func cleanArtist(a playback.Artist) playback.Artist {
	a.Name = cleanLine(a.Name)
	a.Genres = cleanEach(a.Genres, cleanLine)
	return a
}

func cleanAlbum(a playback.Album) playback.Album {
	a.Title, a.Artist = cleanLine(a.Title), cleanLine(a.Artist)
	return a
}

func cleanCatalogPlaylist(p playback.CatalogPlaylist) playback.CatalogPlaylist {
	p.Name, p.Curator = cleanLine(p.Name), cleanLine(p.Curator)
	return p
}

func cleanResults(r playback.SearchResults) playback.SearchResults {
	return playback.SearchResults{
		Suggestions: cleanEach(r.Suggestions, cleanLine),
		Top:         cleanEach(r.Top, cleanSearchItem),
		Artists:     cleanEach(r.Artists, cleanArtist),
		Albums:      cleanEach(r.Albums, cleanAlbum),
		Songs:       cleanEach(r.Songs, cleanSong),
		Playlists:   cleanEach(r.Playlists, cleanCatalogPlaylist),
	}
}

// cleanSearchItem cleans the text of a top result, and its kind, which
// the results page shows as a tag.
func cleanSearchItem(it playback.SearchItem) playback.SearchItem {
	return playback.SearchItem{
		Kind:     playback.SearchItemKind(cleanLine(string(it.Kind))),
		Artist:   cleanArtist(it.Artist),
		Album:    cleanAlbum(it.Album),
		Song:     cleanSong(it.Song),
		Playlist: cleanCatalogPlaylist(it.Playlist),
	}
}

func cleanArtistDetail(d playback.ArtistDetail) playback.ArtistDetail {
	return playback.ArtistDetail{
		Artist:          cleanArtist(d.Artist),
		TopSongs:        cleanEach(d.TopSongs, cleanSong),
		EssentialAlbums: cleanEach(d.EssentialAlbums, cleanAlbum),
		Albums:          cleanEach(d.Albums, cleanAlbum),
		Singles:         cleanEach(d.Singles, cleanAlbum),
		Compilations:    cleanEach(d.Compilations, cleanAlbum),
		Playlists:       cleanEach(d.Playlists, cleanCatalogPlaylist),
		About: playback.ArtistAbout{
			Notes:  cleanText(d.About.Notes),
			Genre:  cleanLine(d.About.Genre),
			Origin: cleanLine(d.About.Origin),
			Formed: cleanLine(d.About.Formed),
		},
	}
}

func cleanAlbumDetail(d playback.AlbumDetail) playback.AlbumDetail {
	return playback.AlbumDetail{
		Album: cleanAlbum(d.Album),
		Tracks: cleanEach(d.Tracks, func(t playback.Track) playback.Track {
			t.Song = cleanSong(t.Song)
			return t
		}),
		Genre:       cleanLine(d.Genre),
		ReleaseDate: cleanLine(d.ReleaseDate),
		RecordLabel: cleanLine(d.RecordLabel),
		Copyright:   cleanLine(d.Copyright),
		Notes:       cleanText(d.Notes),
	}
}

func cleanPlaylistDetail(d playback.PlaylistDetail) playback.PlaylistDetail {
	return playback.PlaylistDetail{
		Playlist: cleanCatalogPlaylist(d.Playlist),
		Tracks:   cleanEach(d.Tracks, cleanSong),
		Notes:    cleanText(d.Notes),
	}
}

func cleanState(s playback.State) playback.State {
	s.Title, s.Artist, s.Album = cleanLine(s.Title), cleanLine(s.Artist), cleanLine(s.Album)
	return s
}
