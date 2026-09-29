// Package playback is the domain core of Nu11Signal: the music types the UI
// works with and the Player port that any playback backend implements.
package playback

import (
	"context"
	"time"
)

// Song is a catalog track.
type Song struct {
	ID       string
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
}

// Artist is a catalog artist.
type Artist struct {
	ID     string
	Name   string
	Genres []string
}

// SearchResults is a mixed catalog search, as Apple Music shows it: term
// suggestions to refine the query, the top results across every kind, then
// matching artists, albums, songs and playlists, each in relevance order.
type SearchResults struct {
	Suggestions []string
	Top         []SearchItem
	Artists     []Artist
	Albums      []Album
	Songs       []Song
	Playlists   []CatalogPlaylist
}

// SearchItemKind names what a SearchItem is.
type SearchItemKind string

// Kinds of top search results.
const (
	ItemArtist   SearchItemKind = "artist"
	ItemAlbum    SearchItemKind = "album"
	ItemSong     SearchItemKind = "song"
	ItemPlaylist SearchItemKind = "playlist"
)

// SearchItem is one top search result: Kind says which of the other fields
// holds it; the rest stay zero.
type SearchItem struct {
	Kind     SearchItemKind
	Artist   Artist
	Album    Album
	Song     Song
	Playlist CatalogPlaylist
}

// Album is a catalog album, single or compilation.
type Album struct {
	ID     string
	Title  string
	Artist string
	// Year is the release year; 0 when unknown.
	Year       int
	TrackCount int
}

// CatalogPlaylist is a catalog playlist, such as an artist's essentials.
type CatalogPlaylist struct {
	ID      string
	Name    string
	Curator string
}

// ArtistAbout is the "About" section of an artist page. Every field is
// empty when the catalog does not know it.
type ArtistAbout struct {
	// Notes is the editorial text, as plain text.
	Notes  string
	Genre  string
	Origin string
	Formed string
}

// ArtistDetail is an artist page: its sections in Apple Music order, each
// empty when the catalog has nothing for it.
type ArtistDetail struct {
	Artist          Artist
	TopSongs        []Song
	EssentialAlbums []Album
	Albums          []Album
	Singles         []Album
	Compilations    []Album
	Playlists       []CatalogPlaylist
	About           ArtistAbout
}

// Track is a song as it appears on an album: the song and its position.
type Track struct {
	Song
	// Number is the track number on its disc; 0 when unknown.
	Number int
	// Disc is the disc number; 0 when unknown.
	Disc int
}

// AlbumDetail is an album page: the album, its tracks in order and the
// facts Apple Music lists under them. Every field but Album may be empty.
type AlbumDetail struct {
	Album  Album
	Tracks []Track
	Genre  string
	// ReleaseDate is the release date as "2006-01-02"; empty when unknown.
	ReleaseDate string
	RecordLabel string
	Copyright   string
	// Notes is the editorial text, as plain text.
	Notes string
}

// PlaylistDetail is a catalog playlist page: the playlist, its songs in
// order and its description as plain text.
type PlaylistDetail struct {
	Playlist CatalogPlaylist
	Tracks   []Song
	Notes    string
}

// Playlist is a library playlist; the UI presents it as a radio station.
type Playlist struct {
	ID   string
	Name string
}

// Status is the player's playback status.
type Status string

// Playback statuses reported by the player.
const (
	StatusPlaying     Status = "playing"
	StatusPaused      Status = "paused"
	StatusStopped     Status = "stopped"
	StatusInterrupted Status = "interrupted"
	StatusSeeking     Status = "seeking"
)

// State is a point-in-time snapshot of the player.
type State struct {
	Status   Status
	Title    string
	Artist   string
	Album    string
	SongID   string
	Duration time.Duration
	Position time.Duration
}

// AuthStatus is the outcome of a music library authorization request.
type AuthStatus string

// Authorization outcomes.
const (
	AuthAuthorized    AuthStatus = "authorized"
	AuthDenied        AuthStatus = "denied"
	AuthRestricted    AuthStatus = "restricted"
	AuthNotDetermined AuthStatus = "notDetermined"
)

// Player is the port through which the application drives playback.
//
// Methods are safe for concurrent use. States delivers snapshots as they
// change and Errors delivers asynchronous backend failures; both channels are
// closed when the player shuts down, whether through Close or a backend crash.
type Player interface {
	Authorize(ctx context.Context) (AuthStatus, error)
	SearchCatalog(ctx context.Context, term string, limit int) (SearchResults, error)
	Artist(ctx context.Context, artistID string) (ArtistDetail, error)
	// Album loads a catalog album page.
	Album(ctx context.Context, albumID string) (AlbumDetail, error)
	// SongAlbum loads the page of the album that contains the catalog song.
	SongAlbum(ctx context.Context, songID string) (AlbumDetail, error)
	CatalogPlaylist(ctx context.Context, playlistID string) (PlaylistDetail, error)
	Playlists(ctx context.Context) ([]Playlist, error)
	PlaySongs(ctx context.Context, ids []string, start int) error
	PlayPlaylist(ctx context.Context, id string) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Next(ctx context.Context) error
	Previous(ctx context.Context) error
	Stop(ctx context.Context) error
	Seek(ctx context.Context, position time.Duration) error
	States() <-chan State
	Errors() <-chan error
	Close() error
}
