package demo

import (
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

// albumYears dates the demo albums; every album is by one artist.
var albumYears = map[string]int{
	"Last Call Sessions": 2076,
	"Dockside Nights":    2075,
	"Route 77":           2071,
	"Spire Tower":        2077,
	"Implants":           2074,
	"Signal Bleed":       2077,
}

// artistExtras is what an artist page adds to the catalog: its about text,
// singles and playlists. Top songs and albums come from the catalog.
type artistExtras struct {
	notes, origin, formed string
	singles               []string
	playlists             []string
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

// artistPage builds the demo page of the artist with id.
func artistPage(id string) (playback.ArtistDetail, bool) {
	var d playback.ArtistDetail
	found := false
	for _, a := range artists {
		if a.ID == id {
			d.Artist, found = a, true
		}
	}
	if !found {
		return d, false
	}
	name := d.Artist.Name
	for _, s := range catalog {
		if s.Artist != name {
			continue
		}
		d.TopSongs = append(d.TopSongs, s)
		if n := len(d.Albums); n > 0 && d.Albums[n-1].Title == s.Album {
			d.Albums[n-1].TrackCount++
			continue
		}
		d.Albums = append(d.Albums, playback.Album{
			ID: "demo-album-" + slug(s.Album), Title: s.Album, Artist: name, Year: albumYears[s.Album], TrackCount: 1,
		})
	}
	d.EssentialAlbums = append([]playback.Album(nil), d.Albums[:1]...)
	x := extras[id]
	for _, title := range x.singles {
		d.Singles = append(d.Singles, playback.Album{ID: "demo-single-" + slug(title), Title: title, Artist: name, Year: 2077, TrackCount: 1})
	}
	for _, pl := range x.playlists {
		d.Playlists = append(d.Playlists, playback.CatalogPlaylist{ID: "demo-playlist-" + slug(pl), Name: pl, Curator: "Nu11Signal"})
	}
	d.About = playback.ArtistAbout{Notes: x.notes, Genre: d.Artist.Genres[0], Origin: x.origin, Formed: x.formed}
	return d, true
}

// slug turns a title into an id fragment: "Route 77" -> "route-77".
func slug(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), "-")
}
