// Package playbacktest provides an in-memory playback.Player for tests.
package playbacktest

import (
	"context"
	"sync"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// ChannelBuffer is how many states and errors the Fake buffers before
// PushState and PushError block.
const ChannelBuffer = 64

// Call is one recorded Player method invocation.
type Call struct {
	Method string
	Args   []any
}

// Fake is an in-memory playback.Player. Configure its exported fields before
// handing it to the code under test; methods are safe for concurrent use.
type Fake struct {
	AuthStatus          playback.AuthStatus
	SearchCatalogResult playback.SearchResults
	ArtistResult        playback.ArtistDetail
	// AlbumResult answers Album, SongAlbumResult SongAlbum and
	// CatalogPlaylistResult CatalogPlaylist.
	AlbumResult           playback.AlbumDetail
	SongAlbumResult       playback.AlbumDetail
	CatalogPlaylistResult playback.PlaylistDetail
	PlaylistsResult       []playback.Playlist
	LibraryPlaylistResult playback.PlaylistDetail
	// VolumeResult answers Volume; a successful SetVolume stores its
	// level there, clamped.
	VolumeResult float64
	// CreatePlaylistResult answers CreatePlaylist.
	CreatePlaylistResult playback.Playlist
	// Loved answers Favorite and Favorites (absent means false); a
	// successful SetFavorite stores its value there.
	Loved map[string]bool
	// Err, when set, is returned by every method; MethodErr overrides it
	// per method name (for example "SearchCatalog").
	Err       error
	MethodErr map[string]error

	mu     sync.Mutex
	repeat playback.RepeatMode
	calls  []Call
	closed bool
	states chan playback.State
	levels chan playback.Spectrum
	errs   chan error
}

// New returns a Fake that authorizes successfully and returns no results.
func New() *Fake {
	return &Fake{
		AuthStatus: playback.AuthAuthorized,
		states:     make(chan playback.State, ChannelBuffer),
		levels:     make(chan playback.Spectrum, 1),
		errs:       make(chan error, ChannelBuffer),
	}
}

// Calls returns a copy of the recorded calls, in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Closed reports whether Close was called.
func (f *Fake) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// PushState delivers s on States. It blocks when ChannelBuffer states are
// unread.
func (f *Fake) PushState(s playback.State) { f.states <- s }

// PushLevels delivers a spectrum reading on Levels, replacing one still
// unread, as the helper does. Call it from one goroutine at a time.
func (f *Fake) PushLevels(levels playback.Spectrum) {
	for {
		select {
		case f.levels <- levels:
			return
		default:
		}
		select {
		case <-f.levels:
		default:
		}
	}
}

// PushError delivers err on Errors. It blocks when ChannelBuffer errors are
// unread.
func (f *Fake) PushError(err error) { f.errs <- err }

func (f *Fake) record(method string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Method: method, Args: args})
	if err, ok := f.MethodErr[method]; ok {
		return err
	}
	return f.Err
}

func (f *Fake) Authorize(context.Context) (playback.AuthStatus, error) {
	if err := f.record("Authorize"); err != nil {
		return "", err
	}
	return f.AuthStatus, nil
}

func (f *Fake) SearchCatalog(_ context.Context, term string, limit int) (playback.SearchResults, error) {
	if err := f.record("SearchCatalog", term, limit); err != nil {
		return playback.SearchResults{}, err
	}
	return f.SearchCatalogResult, nil
}

func (f *Fake) Artist(_ context.Context, artistID string) (playback.ArtistDetail, error) {
	if err := f.record("Artist", artistID); err != nil {
		return playback.ArtistDetail{}, err
	}
	return f.ArtistResult, nil
}

func (f *Fake) Album(_ context.Context, albumID string) (playback.AlbumDetail, error) {
	if err := f.record("Album", albumID); err != nil {
		return playback.AlbumDetail{}, err
	}
	return f.AlbumResult, nil
}

func (f *Fake) SongAlbum(_ context.Context, songID string) (playback.AlbumDetail, error) {
	if err := f.record("SongAlbum", songID); err != nil {
		return playback.AlbumDetail{}, err
	}
	return f.SongAlbumResult, nil
}

func (f *Fake) CatalogPlaylist(_ context.Context, playlistID string) (playback.PlaylistDetail, error) {
	if err := f.record("CatalogPlaylist", playlistID); err != nil {
		return playback.PlaylistDetail{}, err
	}
	return f.CatalogPlaylistResult, nil
}

func (f *Fake) Playlists(context.Context) ([]playback.Playlist, error) {
	if err := f.record("Playlists"); err != nil {
		return nil, err
	}
	return f.PlaylistsResult, nil
}

func (f *Fake) LibraryPlaylist(_ context.Context, playlistID string) (playback.PlaylistDetail, error) {
	if err := f.record("LibraryPlaylist", playlistID); err != nil {
		return playback.PlaylistDetail{}, err
	}
	return f.LibraryPlaylistResult, nil
}

func (f *Fake) PlaySongs(_ context.Context, ids []string, start int) error {
	return f.record("PlaySongs", append([]string(nil), ids...), start)
}

func (f *Fake) PlayPlaylist(_ context.Context, id string) error {
	return f.record("PlayPlaylist", id)
}

func (f *Fake) PlayPlaylistFrom(_ context.Context, playlistID string, start int) error {
	return f.record("PlayPlaylistFrom", playlistID, start)
}

func (f *Fake) Pause(context.Context) error    { return f.record("Pause") }
func (f *Fake) Resume(context.Context) error   { return f.record("Resume") }
func (f *Fake) Next(context.Context) error     { return f.record("Next") }
func (f *Fake) Previous(context.Context) error { return f.record("Previous") }
func (f *Fake) Stop(context.Context) error     { return f.record("Stop") }

func (f *Fake) Seek(_ context.Context, position time.Duration) error {
	return f.record("Seek", position)
}

// SetRepeat records the call and, when it succeeds, stores the mode that
// RepeatMode reports.
func (f *Fake) SetRepeat(_ context.Context, mode playback.RepeatMode) error {
	if err := f.record("SetRepeat", mode); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.repeat = mode
	return nil
}

// RepeatMode reports the mode the last successful SetRepeat set;
// RepeatOff before any.
func (f *Fake) RepeatMode() playback.RepeatMode {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.repeat == "" {
		return playback.RepeatOff
	}
	return f.repeat
}

func (f *Fake) Volume(context.Context) (float64, error) {
	if err := f.record("Volume"); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.VolumeResult, nil
}

func (f *Fake) SetVolume(_ context.Context, level float64) error {
	if err := f.record("SetVolume", level); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.VolumeResult = playback.ClampVolume(level)
	return nil
}

func (f *Fake) CreatePlaylist(_ context.Context, name, description string, songIDs []string) (playback.Playlist, error) {
	if err := f.record("CreatePlaylist", name, description, append([]string(nil), songIDs...)); err != nil {
		return playback.Playlist{}, err
	}
	return f.CreatePlaylistResult, nil
}

func (f *Fake) AddToPlaylist(_ context.Context, playlistID string, songIDs []string) error {
	return f.record("AddToPlaylist", playlistID, append([]string(nil), songIDs...))
}

func (f *Fake) Favorite(_ context.Context, songID string) (bool, error) {
	if err := f.record("Favorite", songID); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Loved[songID], nil
}

// Favorites answers every id from Loved.
func (f *Fake) Favorites(_ context.Context, songIDs []string) (map[string]bool, error) {
	if err := f.record("Favorites", append([]string(nil), songIDs...)); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	loved := make(map[string]bool, len(songIDs))
	for _, id := range songIDs {
		loved[id] = f.Loved[id]
	}
	return loved, nil
}

func (f *Fake) SetFavorite(_ context.Context, songID string, on bool) error {
	if err := f.record("SetFavorite", songID, on); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Loved == nil {
		f.Loved = map[string]bool{}
	}
	f.Loved[songID] = on
	return nil
}

func (f *Fake) States() <-chan playback.State { return f.states }
func (f *Fake) Errors() <-chan error          { return f.errs }

// Levels delivers what PushLevels pushes (see playback.LevelSource).
func (f *Fake) Levels() <-chan playback.Spectrum { return f.levels }

// Close closes the States, Levels and Errors channels. It is idempotent; pushing
// after Close panics, as sending on a closed channel does.
func (f *Fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.states)
		close(f.levels)
		close(f.errs)
	}
	return nil
}
