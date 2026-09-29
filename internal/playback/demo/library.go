package demo

import (
	"time"

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
