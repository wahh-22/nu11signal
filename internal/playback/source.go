package playback

import (
	"errors"
	"strings"
)

// Sources: a Player may join several backends (see package composite),
// told apart by the namespace of their ids.

// ErrUnsupported is returned by a Player method its source has no answer
// for: local files have no catalog, favorites or playlist editing, and a
// player without Apple Music has no catalog at all. The UI checks
// Supports first and hides what is unsupported, so it should not see it.
var ErrUnsupported = errors.New("not available for this source")

// ErrNoOutput is wrapped by the errors of a Player whose audio output
// does not work: it would not open in time, or it stopped taking audio
// while a song played (as ALSA's default device does on Linux without a
// sound card). The error's text ends with ErrNoOutput's own, then " // "
// and a hint for the user, as "no audio output // is PulseAudio or
// PipeWire running?".
var ErrNoOutput = errors.New("no audio output")

// Source names a backend.
type Source string

// The backends.
const (
	// SourceApple is Apple Music, through the helper (and the demo).
	SourceApple Source = "apple"
	// SourceLocal is the computer's own music files.
	SourceLocal Source = "local"
)

// LocalPrefix starts every id of a local song or playlist; playlists add
// "pl:". No Apple Music id has it.
const LocalPrefix = "local:"

// SourceOf is the backend an id (a song's or a playlist's) belongs to.
func SourceOf(id string) Source {
	if strings.HasPrefix(id, LocalPrefix) {
		return SourceLocal
	}
	return SourceApple
}

// Capability is something a source may not offer.
type Capability string

// The capabilities the UI checks.
const (
	// CapCatalogSearch is the catalog search and the pages it opens
	// (artist, album, song and catalog playlist pages).
	CapCatalogSearch Capability = "catalog-search"
	// CapFavorites is loving songs (Favorite, Favorites, SetFavorite).
	CapFavorites Capability = "favorites"
	// CapEditPlaylists is creating playlists and adding songs to them.
	CapEditPlaylists Capability = "edit-playlists"
)

// Capabilities is implemented by a Player whose sources do not all offer
// everything. Supports reports whether c is offered for the song or
// playlist id, or, with an empty id, by any of its sources. It is cheap
// and safe for concurrent use: the UI asks while drawing.
type Capabilities interface {
	Supports(c Capability, id string) bool
}

// Supports reports whether p offers c for id (see Capabilities); a Player
// that does not implement Capabilities offers everything.
func Supports(p Player, c Capability, id string) bool {
	if cp, ok := p.(Capabilities); ok {
		return cp.Supports(c, id)
	}
	return true
}

// LibraryWatcher is implemented by a Player whose playlists change on
// their own, as a local library whose scan ends after startup: a value on
// LibraryChanged means Playlists answers something new. Only the latest
// change is kept; the channel is closed by Close.
type LibraryWatcher interface {
	LibraryChanged() <-chan struct{}
}
