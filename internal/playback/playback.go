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

// SearchResults is a mixed catalog search: term suggestions to refine the
// query, then matching artists and songs, each in relevance order.
type SearchResults struct {
	Suggestions []string
	Artists     []Artist
	Songs       []Song
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
	Search(ctx context.Context, term string, limit int) ([]Song, error)
	SearchCatalog(ctx context.Context, term string, limit int) (SearchResults, error)
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
