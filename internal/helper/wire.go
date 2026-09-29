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
}

type wireState struct {
	Status   string  `json:"status"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	SongID   string  `json:"songId"`
	Duration float64 `json:"duration"`
	Position float64 `json:"position"`
}

type wireSong struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Duration float64 `json:"duration"`
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
	Suggestions []string     `json:"suggestions"`
	Artists     []wireArtist `json:"artists"`
	Songs       []wireSong   `json:"songs"`
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

type catalogPlaylistResult struct {
	Playlist wireCatalogPlaylist `json:"playlist"`
	Tracks   []wireSong          `json:"tracks"`
	Notes    string              `json:"notes"`
}

type playlistsResult struct {
	Playlists []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
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

// seconds converts the protocol's fractional seconds to a Duration.
func seconds(s float64) time.Duration {
	return time.Duration(math.Round(s * float64(time.Second)))
}

func (s wireState) toDomain() playback.State {
	return playback.State{
		Status:   playback.Status(s.Status),
		Title:    s.Title,
		Artist:   s.Artist,
		Album:    s.Album,
		SongID:   s.SongID,
		Duration: seconds(s.Duration),
		Position: seconds(s.Position),
	}
}

func (s wireSong) toDomain() playback.Song {
	return playback.Song{
		ID:       s.ID,
		Title:    s.Title,
		Artist:   s.Artist,
		Album:    s.Album,
		Duration: seconds(s.Duration),
	}
}

func (a wireArtist) toDomain() playback.Artist {
	return playback.Artist{ID: a.ID, Name: a.Name, Genres: a.Genres}
}

// toDomain keeps lists the helper omitted nil.
func (r searchCatalogResult) toDomain() playback.SearchResults {
	res := playback.SearchResults{Suggestions: r.Suggestions}
	for _, a := range r.Artists {
		res.Artists = append(res.Artists, a.toDomain())
	}
	for _, s := range r.Songs {
		res.Songs = append(res.Songs, s.toDomain())
	}
	return res
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
