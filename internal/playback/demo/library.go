package demo

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// The demo library is fictional: invented artists and tracks.

func song(id, title, artist, album string, secs int) playback.Song {
	return playback.Song{ID: id, Title: title, Artist: artist, Album: album, Duration: time.Duration(secs) * time.Second}
}

var catalog = []playback.Song{
	song("d01", "Neon Arteries", "Chrome Saints", "Last Call Sessions", 214),
	song("d02", "Kabuki Rain", "Lux Vendetta", "Dockside Nights", 187),
	song("d03", "Overclocked Heart", "Chrome Saints", "Last Call Sessions", 241),
	song("d04", "Badlands Mirage", "Dust Protocol", "Route 77", 198),
	song("d05", "Glass District", "Lux Vendetta", "Dockside Nights", 226),
	song("d06", "Plaza Blues", "The Netrunners", "Spire Tower", 263),
	song("d07", "Harbor Undertow", "Dust Protocol", "Route 77", 175),
	song("d08", "Ghost in the Chrome", "The Netrunners", "Spire Tower", 232),
	song("d09", "Neon Samurai", "Midnight Surgeon", "Implants", 205),
	song("d10", "Dreamfeed Lullaby", "Midnight Surgeon", "Implants", 248),
	song("d11", "Northside Static", "Kuroi Hana", "Signal Bleed", 193),
	song("d12", "Afterparty Uptown", "Kuroi Hana", "Signal Bleed", 219),
}

// artists are the demo catalog's artists, in the order search returns them.
var artists = []playback.Artist{
	{ID: "demo-artist-chrome-saints", Name: "Chrome Saints", Genres: []string{"Synthwave"}},
	{ID: "demo-artist-lux-vendetta", Name: "Lux Vendetta", Genres: []string{"Darkwave"}},
	{ID: "demo-artist-dust-protocol", Name: "Dust Protocol", Genres: []string{"Rock"}},
	{ID: "demo-artist-the-netrunners", Name: "The Netrunners", Genres: []string{"Industrial"}},
	{ID: "demo-artist-midnight-surgeon", Name: "Midnight Surgeon", Genres: []string{"Electronic"}},
	{ID: "demo-artist-kuroi-hana", Name: "Kuroi Hana", Genres: []string{"J-Pop", "Electronic"}},
}

type station struct {
	playback.Playlist
	ids []string
}

func (s station) songs() []playback.Song {
	out := make([]playback.Song, 0, len(s.ids))
	for _, id := range s.ids {
		if song, ok := songByID(id); ok {
			out = append(out, song)
		}
	}
	return out
}

var stations = []station{
	{playback.Playlist{ID: "demo-1", Name: "Heat Sink Radio"}, []string{"d01", "d03", "d09"}},
	{playback.Playlist{ID: "demo-2", Name: "Night Drive"}, []string{"d02", "d05", "d11", "d12"}},
	{playback.Playlist{ID: "demo-3", Name: "Badlands Rock"}, []string{"d04", "d07"}},
	{playback.Playlist{ID: "demo-4", Name: "Coastline Dreams"}, []string{"d06", "d08", "d10"}},
	{playback.Playlist{ID: "demo-5", Name: "Low Orbit"}, []string{"d10", "d08", "d03", "d11"}},
	{playback.Playlist{ID: "demo-6", Name: "Static FM"}, []string{"d12", "d01", "d05", "d07", "d09"}},
}

func songByID(id string) (playback.Song, bool) {
	for _, s := range catalog {
		if s.ID == id {
			return s, true
		}
	}
	return playback.Song{}, false
}

// albumReleases dates the demo albums ("2006-01-02"); every album is by
// one artist.
var albumReleases = map[string]string{
	"Last Call Sessions": "2076-03-14",
	"Dockside Nights":    "2075-10-31",
	"Route 77":           "2071-06-27",
	"Spire Tower":        "2077-01-09",
	"Implants":           "2074-08-02",
	"Signal Bleed":       "2077-05-20",
}

// albumNotes are the editorial notes of the albums that have some.
var albumNotes = map[string]string{
	"Last Call Sessions": "Recorded in one night at the Afterlife after the last call, with the doors locked and the " +
		"bartender on backing vocals. Neon Arteries was the first take; Overclocked Heart the last, at dawn, " +
		"when the city's power grid dipped and the tape machines kept rolling anyway.",
}

// demoLabel is the record label of every demo release.
const demoLabel = "Nu11Signal Records"

// artistExtras is what an artist page adds to the catalog: its about text,
// singles and playlists. Top songs and albums come from the catalog.
type artistExtras struct {
	notes, origin, formed string
	// singles are edits of catalog songs: "Overclocked Heart (Edit)" is
	// the song "Overclocked Heart".
	singles   []string
	playlists []string
}

var extras = map[string]artistExtras{
	"demo-artist-chrome-saints": {
		notes:     "Chrome Saints turned the last-call jukeboxes of Watson into a genre: slow-burning synth anthems for people who never go home.",
		origin:    "Watson, Night City",
		formed:    "2069",
		singles:   []string{"Overclocked Heart (Edit)"},
		playlists: []string{"Chrome Saints Essentials"},
	},
	"demo-artist-lux-vendetta": {
		notes:  "Darkwave duo recorded entirely on the docks of Kabuki, between shift changes.",
		origin: "Kabuki, Night City",
	},
	"demo-artist-dust-protocol": {
		notes:   "Badlands rock played loud enough to reach Night City from the highway.",
		formed:  "2066",
		singles: []string{"Harbor Undertow (Live)"},
	},
	"demo-artist-the-netrunners": {
		notes:     "Industrial crew rumoured to master their records inside the Net itself.",
		playlists: []string{"Netrunners: Deep Dive"},
	},
	"demo-artist-midnight-surgeon": {
		notes: "Electronic lullabies for the freshly chromed.",
	},
	"demo-artist-kuroi-hana": {
		notes:  "J-Pop idols turned static-soaked producers; Signal Bleed is their third reinvention.",
		origin: "Japantown, Night City",
	},
}

func albumID(title string) string   { return "demo-album-" + slug(title) }
func singleID(title string) string  { return "demo-single-" + slug(title) }
func playlistID(name string) string { return "demo-playlist-" + slug(name) }
func artistByID(id string) (playback.Artist, bool) {
	for _, a := range artists {
		if a.ID == id {
			return a, true
		}
	}
	return playback.Artist{}, false
}

// yearOf is the year of a "2006-01-02" date; 0 when there is none.
func yearOf(date string) int {
	year, _ := strconv.Atoi(strings.SplitN(date, "-", 2)[0])
	return year
}

// singleSong is the catalog song a single by artist edits: its title up to
// the parenthesised edit name.
func singleSong(single, artist string) (playback.Song, bool) {
	base, _, _ := strings.Cut(single, " (")
	for _, s := range catalog {
		if s.Title == base && s.Artist == artist {
			return s, true
		}
	}
	return playback.Song{}, false
}

// artistPage builds the demo page of the artist with id.
func artistPage(id string) (playback.ArtistDetail, bool) {
	a, ok := artistByID(id)
	if !ok {
		return playback.ArtistDetail{}, false
	}
	return buildArtistPage(a, catalog, extras[id]), true
}

// buildArtistPage lays out the page of a from the songs by it in songs and
// its extras. Sections with nothing in them stay nil.
func buildArtistPage(a playback.Artist, songs []playback.Song, x artistExtras) playback.ArtistDetail {
	d := playback.ArtistDetail{Artist: a}
	for _, s := range songs {
		if s.Artist != a.Name {
			continue
		}
		d.TopSongs = append(d.TopSongs, s)
		if n := len(d.Albums); n > 0 && d.Albums[n-1].Title == s.Album {
			d.Albums[n-1].TrackCount++
			continue
		}
		d.Albums = append(d.Albums, playback.Album{
			ID: albumID(s.Album), Title: s.Album, Artist: a.Name, Year: yearOf(albumReleases[s.Album]), TrackCount: 1,
		})
	}
	if len(d.Albums) > 0 {
		// The first album stands in for Apple Music's "Essential Albums".
		d.EssentialAlbums = []playback.Album{d.Albums[0]}
	}
	for _, title := range x.singles {
		if single, ok := singleAlbum(title, a); ok {
			d.Singles = append(d.Singles, single.Album)
		}
	}
	for _, pl := range x.playlists {
		d.Playlists = append(d.Playlists, playback.CatalogPlaylist{ID: playlistID(pl), Name: pl, Curator: "Nu11Signal"})
	}
	d.About = playback.ArtistAbout{Notes: x.notes, Origin: x.origin, Formed: x.formed}
	if len(a.Genres) > 0 {
		d.About.Genre = a.Genres[0]
	}
	return d
}

// albumDetail completes an album page with its release facts; the artist's
// first genre is the album's.
func albumDetail(al playback.Album, tracks []playback.Track, release string, a playback.Artist) playback.AlbumDetail {
	d := playback.AlbumDetail{Album: al, Tracks: tracks, ReleaseDate: release, RecordLabel: demoLabel, Notes: albumNotes[al.Title]}
	if len(a.Genres) > 0 {
		d.Genre = a.Genres[0]
	}
	if al.Year > 0 {
		d.Copyright = fmt.Sprintf("℗ %d %s", al.Year, a.Name)
	}
	return d
}

// catalogAlbum is the page of the catalog album with id: its songs in
// catalog order.
func catalogAlbum(id string) (playback.AlbumDetail, bool) {
	var tracks []playback.Track
	var first playback.Song
	for _, s := range catalog {
		if albumID(s.Album) == id {
			if len(tracks) == 0 {
				first = s
			}
			tracks = append(tracks, playback.Track{Song: s, Number: len(tracks) + 1, Disc: 1})
		}
	}
	if len(tracks) == 0 {
		return playback.AlbumDetail{}, false
	}
	release := albumReleases[first.Album]
	al := playback.Album{ID: id, Title: first.Album, Artist: first.Artist, Year: yearOf(release), TrackCount: len(tracks)}
	var artist playback.Artist
	for _, a := range artists {
		if a.Name == first.Artist {
			artist = a
		}
	}
	if artist.Name == "" {
		artist.Name = first.Artist
	}
	return albumDetail(al, tracks, release, artist), true
}

// singleAlbum is the page of single title by a: the one song it edits,
// released with the album that song is from (a single without its song is
// left out).
func singleAlbum(title string, a playback.Artist) (playback.AlbumDetail, bool) {
	s, ok := singleSong(title, a.Name)
	if !ok {
		return playback.AlbumDetail{}, false
	}
	release := albumReleases[s.Album]
	al := playback.Album{ID: singleID(title), Title: title, Artist: a.Name, Year: yearOf(release), TrackCount: 1}
	return albumDetail(al, []playback.Track{{Song: s, Number: 1, Disc: 1}}, release, a), true
}

// albumPage is the page of the album or single with id.
func albumPage(id string) (playback.AlbumDetail, bool) {
	if d, ok := catalogAlbum(id); ok {
		return d, true
	}
	for _, a := range artists {
		for _, title := range extras[a.ID].singles {
			if singleID(title) == id {
				return singleAlbum(title, a)
			}
		}
	}
	return playback.AlbumDetail{}, false
}

// playlistPage is the page of the artist playlist with id: the artist's
// songs in catalog order.
func playlistPage(id string) (playback.PlaylistDetail, bool) {
	for _, a := range artists {
		for _, name := range extras[a.ID].playlists {
			if playlistID(name) != id {
				continue
			}
			d := playback.PlaylistDetail{
				Playlist: playback.CatalogPlaylist{ID: id, Name: name, Curator: "Nu11Signal"},
				Notes:    "Handpicked by Nu11Signal: every " + a.Name + " track on the demo network, in broadcast order.",
			}
			for _, s := range catalog {
				if s.Artist == a.Name {
					d.Tracks = append(d.Tracks, s)
				}
			}
			return d, true
		}
	}
	return playback.PlaylistDetail{}, false
}

// searchAlbums lists the catalog albums whose title or artist contains
// needle (lower case), in catalog order, at most limit.
func searchAlbums(needle string, limit int) []playback.Album {
	var out []playback.Album
	seen := map[string]bool{}
	for _, s := range catalog {
		if len(out) >= limit {
			break
		}
		if seen[s.Album] || !strings.Contains(strings.ToLower(s.Album+" "+s.Artist), needle) {
			continue
		}
		seen[s.Album] = true
		if d, ok := catalogAlbum(albumID(s.Album)); ok {
			out = append(out, d.Album)
		}
	}
	return out
}

// searchPlaylists lists the artist playlists whose name or artist contains
// needle (lower case), at most limit.
func searchPlaylists(needle string, limit int) []playback.CatalogPlaylist {
	var out []playback.CatalogPlaylist
	for _, a := range artists {
		for _, name := range extras[a.ID].playlists {
			if len(out) < limit && strings.Contains(strings.ToLower(name+" "+a.Name), needle) {
				out = append(out, playback.CatalogPlaylist{ID: playlistID(name), Name: name, Curator: "Nu11Signal"})
			}
		}
	}
	return out
}

// topResults stands in for Apple Music's top results: the first match of
// each kind, at most limit.
func topResults(res playback.SearchResults, limit int) []playback.SearchItem {
	var top []playback.SearchItem
	if len(res.Artists) > 0 {
		top = append(top, playback.SearchItem{Kind: playback.ItemArtist, Artist: res.Artists[0]})
	}
	if len(res.Albums) > 0 {
		top = append(top, playback.SearchItem{Kind: playback.ItemAlbum, Album: res.Albums[0]})
	}
	if len(res.Songs) > 0 {
		top = append(top, playback.SearchItem{Kind: playback.ItemSong, Song: res.Songs[0]})
	}
	if len(res.Playlists) > 0 {
		top = append(top, playback.SearchItem{Kind: playback.ItemPlaylist, Playlist: res.Playlists[0]})
	}
	return top[:min(len(top), limit)]
}

// slug turns a title into an id fragment: "Route 77" -> "route-77".
func slug(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), "-")
}
