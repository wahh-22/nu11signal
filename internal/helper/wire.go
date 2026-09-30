package helper

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Wire types for the helper's JSON Lines protocol (see
// helper/Sources/Nu11SignalProtocol/Codec.swift).
//
// Requests:  {"id":"<id>","cmd":"<name>", ...args}
// Responses: {"id":"<id>","ok":true,"result":{...}} | {"id":"<id>","ok":false,"error":"<msg>"}
// Events:    {"event":"ready"} | {"event":"state","state":{...}} | {"event":"error","message":"<msg>"}
//            | {"event":"levels","bands":[0...100, ...],"wave":[-100...100, ...]}

// inbound is any line the helper writes to stdout. Lines carrying "event"
// are events; everything else is a response. A response with an empty id
// answers a request the helper could not parse.
type inbound struct {
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Result  json.RawMessage `json:"result"`
	Error   string          `json:"error"`
	Event   string          `json:"event"`
	State   *wireState      `json:"state"`
	Message string          `json:"message"`
	// Bands are a levels event's readings, percentages from low to high
	// frequencies.
	Bands []float64 `json:"bands"`
	// Wave is a levels event's waveform, hundredths of full scale, oldest
	// first; helpers older than the oscilloscope leave it out.
	Wave []float64 `json:"wave"`
}

type wireState struct {
	Status   string  `json:"status"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	SongID   string  `json:"songId"`
	Duration float64 `json:"duration"`
	Position float64 `json:"position"`
	Repeat   string  `json:"repeat"`
	// VolumeMode is "app" or "system" (see playback.VolumeMode).
	VolumeMode string `json:"volumeMode"`
}

type wireSong struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Duration float64 `json:"duration"`
	// LibraryOnly is sent, as true, only for a library playlist's song
	// that is not in the catalog.
	LibraryOnly bool `json:"libraryOnly"`
}

type authResult struct {
	Status string `json:"status"`
}

type wireArtist struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Genres []string `json:"genres"`
}

type searchCatalogResult struct {
	Suggestions []string              `json:"suggestions"`
	Top         []wireSearchItem      `json:"top"`
	Artists     []wireArtist          `json:"artists"`
	Albums      []wireAlbum           `json:"albums"`
	Songs       []wireSong            `json:"songs"`
	Playlists   []wireCatalogPlaylist `json:"playlists"`
}

// wireSearchItem is a top search result: {"kind":"artist","artist":{...}},
// and likewise for album, song and playlist.
type wireSearchItem struct {
	Kind     string               `json:"kind"`
	Artist   *wireArtist          `json:"artist"`
	Album    *wireAlbum           `json:"album"`
	Song     *wireSong            `json:"song"`
	Playlist *wireCatalogPlaylist `json:"playlist"`
}

type wireAlbum struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Year       int    `json:"year"`
	TrackCount int    `json:"trackCount"`
}

type wireCatalogPlaylist struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Curator string `json:"curator"`
}

type artistResult struct {
	Artist          wireArtist            `json:"artist"`
	TopSongs        []wireSong            `json:"topSongs"`
	EssentialAlbums []wireAlbum           `json:"essentialAlbums"`
	Albums          []wireAlbum           `json:"albums"`
	Singles         []wireAlbum           `json:"singles"`
	Compilations    []wireAlbum           `json:"compilations"`
	Playlists       []wireCatalogPlaylist `json:"playlists"`
	About           struct {
		Notes  string `json:"notes"`
		Genre  string `json:"genre"`
		Origin string `json:"origin"`
		Formed string `json:"formed"`
	} `json:"about"`
}

// wireTrack is a song on an album, with its position.
type wireTrack struct {
	wireSong
	TrackNumber int `json:"trackNumber"`
	DiscNumber  int `json:"discNumber"`
}

type albumResult struct {
	Album       wireAlbum   `json:"album"`
	Tracks      []wireTrack `json:"tracks"`
	Genre       string      `json:"genre"`
	ReleaseDate string      `json:"releaseDate"`
	RecordLabel string      `json:"recordLabel"`
	Copyright   string      `json:"copyright"`
	Notes       string      `json:"notes"`
}

// catalogPlaylistResult answers catalogPlaylist and libraryPlaylist.
type catalogPlaylistResult struct {
	Playlist wireCatalogPlaylist `json:"playlist"`
	Tracks   []wireSong          `json:"tracks"`
	Notes    string              `json:"notes"`
}

// createPlaylistResult answers createPlaylist: the new library playlist.
type createPlaylistResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type favoriteResult struct {
	Favorite bool `json:"favorite"`
}

// favoritesResult answers favorites: loved or not, by song id.
type favoritesResult struct {
	Favorites map[string]bool `json:"favorites"`
}

type volumeResult struct {
	Level float64 `json:"level"`
}

type playlistsResult struct {
	Playlists []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Editable bool   `json:"editable"`
	} `json:"playlists"`
}

// encodeRequest builds one request line, terminated by "\n". Arguments
// share the top-level object with "id" and "cmd", so those names are reserved.
func encodeRequest(id, cmd string, args map[string]any) ([]byte, error) {
	fields := make(map[string]any, len(args)+2)
	for k, v := range args {
		if k == "id" || k == "cmd" {
			return nil, fmt.Errorf("argument %q collides with the request envelope", k)
		}
		fields[k] = v
	}
	fields["id"] = id
	fields["cmd"] = cmd
	line, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return append(line, '\n'), nil
}

// levels converts a levels event's percentages to 0...1 levels.
func levels(bands []float64) []float64 {
	out := make([]float64, len(bands))
	for i, b := range bands {
		out[i] = min(max(b/100, 0), 1)
	}
	return out
}

// wave converts a levels event's waveform to -1...1 points; no waveform
// stays nil.
func wave(points []float64) []float64 {
	if points == nil {
		return nil
	}
	out := make([]float64, len(points))
	for i, p := range points {
		out[i] = min(max(p/100, -1), 1)
	}
	return out
}

// seconds converts the protocol's fractional seconds to a Duration.
func seconds(s float64) time.Duration {
	return time.Duration(math.Round(s * float64(time.Second)))
}

func (s wireState) toDomain() playback.State {
	return playback.State{
		Status:     playback.Status(s.Status),
		Title:      s.Title,
		Artist:     s.Artist,
		Album:      s.Album,
		SongID:     s.SongID,
		Duration:   seconds(s.Duration),
		Position:   seconds(s.Position),
		Repeat:     repeatMode(s.Repeat),
		VolumeMode: volumeMode(s.VolumeMode),
	}
}

// volumeMode reads the state's "volumeMode"; a helper that sends none (an
// older one), or a mode this build does not know, reports none.
func volumeMode(s string) playback.VolumeMode {
	switch mode := playback.VolumeMode(s); mode {
	case playback.VolumeApp, playback.VolumeSystem:
		return mode
	}
	return ""
}

// repeatMode reads the state's "repeat"; a helper that sends none (an
// older one), or a mode this build does not know, reports RepeatOff.
func repeatMode(s string) playback.RepeatMode {
	switch mode := playback.RepeatMode(s); mode {
	case playback.RepeatAll, playback.RepeatOne:
		return mode
	}
	return playback.RepeatOff
}

func (s wireSong) toDomain() playback.Song {
	return playback.Song{
		ID:          s.ID,
		Title:       s.Title,
		Artist:      s.Artist,
		Album:       s.Album,
		Duration:    seconds(s.Duration),
		LibraryOnly: s.LibraryOnly,
	}
}

func (a wireArtist) toDomain() playback.Artist {
	return playback.Artist{ID: a.ID, Name: a.Name, Genres: a.Genres}
}

// toDomain keeps lists the helper omitted nil.
func (r searchCatalogResult) toDomain() playback.SearchResults {
	res := playback.SearchResults{Suggestions: r.Suggestions, Albums: albums(r.Albums)}
	for _, it := range r.Top {
		if item, ok := it.toDomain(); ok {
			res.Top = append(res.Top, item)
		}
	}
	for _, a := range r.Artists {
		res.Artists = append(res.Artists, a.toDomain())
	}
	for _, s := range r.Songs {
		res.Songs = append(res.Songs, s.toDomain())
	}
	for _, p := range r.Playlists {
		res.Playlists = append(res.Playlists, p.toDomain())
	}
	return res
}

// toDomain converts a top result; ok is false for a kind the UI cannot
// open, or one without its payload.
func (it wireSearchItem) toDomain() (item playback.SearchItem, ok bool) {
	item.Kind = playback.SearchItemKind(it.Kind)
	switch {
	case item.Kind == playback.ItemArtist && it.Artist != nil:
		item.Artist = it.Artist.toDomain()
	case item.Kind == playback.ItemAlbum && it.Album != nil:
		item.Album = it.Album.toDomain()
	case item.Kind == playback.ItemSong && it.Song != nil:
		item.Song = it.Song.toDomain()
	case item.Kind == playback.ItemPlaylist && it.Playlist != nil:
		item.Playlist = it.Playlist.toDomain()
	default:
		return playback.SearchItem{}, false
	}
	return item, true
}

func (a wireAlbum) toDomain() playback.Album {
	return playback.Album{ID: a.ID, Title: a.Title, Artist: a.Artist, Year: a.Year, TrackCount: a.TrackCount}
}

// albums converts a section, keeping one the helper omitted nil.
func albums(in []wireAlbum) []playback.Album {
	var out []playback.Album
	for _, a := range in {
		out = append(out, a.toDomain())
	}
	return out
}

// toDomain keeps sections the helper omitted nil.
func (r artistResult) toDomain() playback.ArtistDetail {
	d := playback.ArtistDetail{
		Artist:          r.Artist.toDomain(),
		EssentialAlbums: albums(r.EssentialAlbums),
		Albums:          albums(r.Albums),
		Singles:         albums(r.Singles),
		Compilations:    albums(r.Compilations),
		About:           playback.ArtistAbout(r.About),
	}
	for _, s := range r.TopSongs {
		d.TopSongs = append(d.TopSongs, s.toDomain())
	}
	for _, p := range r.Playlists {
		d.Playlists = append(d.Playlists, p.toDomain())
	}
	return d
}

func (p wireCatalogPlaylist) toDomain() playback.CatalogPlaylist {
	return playback.CatalogPlaylist{ID: p.ID, Name: p.Name, Curator: p.Curator}
}

// toDomain keeps tracks the helper omitted nil.
func (r albumResult) toDomain() playback.AlbumDetail {
	d := playback.AlbumDetail{
		Album:       r.Album.toDomain(),
		Genre:       r.Genre,
		ReleaseDate: r.ReleaseDate,
		RecordLabel: r.RecordLabel,
		Copyright:   r.Copyright,
		Notes:       r.Notes,
	}
	for _, t := range r.Tracks {
		d.Tracks = append(d.Tracks, playback.Track{Song: t.wireSong.toDomain(), Number: t.TrackNumber, Disc: t.DiscNumber})
	}
	return d
}

// toDomain keeps tracks the helper omitted nil.
func (r catalogPlaylistResult) toDomain() playback.PlaylistDetail {
	d := playback.PlaylistDetail{Playlist: r.Playlist.toDomain(), Notes: r.Notes}
	for _, s := range r.Tracks {
		d.Tracks = append(d.Tracks, s.toDomain())
	}
	return d
}
