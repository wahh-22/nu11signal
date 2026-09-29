package helper

import (
	"context"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Authorize asks for Apple Music access; the system may prompt the user.
func (c *Client) Authorize(ctx context.Context) (playback.AuthStatus, error) {
	var res authResult
	if err := c.call(ctx, "authorize", nil, &res); err != nil {
		return "", err
	}
	return playback.AuthStatus(res.Status), nil
}

// SearchCatalog runs a mixed catalog search: suggestions, top results,
// artists, albums, songs and playlists. The helper clamps limit to 1...25
// per result type (suggestions to 10, top results to 6).
func (c *Client) SearchCatalog(ctx context.Context, term string, limit int) (playback.SearchResults, error) {
	var res searchCatalogResult
	if err := c.call(ctx, "searchCatalog", map[string]any{"term": term, "limit": limit}, &res); err != nil {
		return playback.SearchResults{}, err
	}
	return res.toDomain(), nil
}

// Artist loads an artist page. The helper leaves out sections the catalog
// has nothing for (or failed to load); they stay empty.
//
// The id travels as "artistId", never "id" (see PlayPlaylist).
func (c *Client) Artist(ctx context.Context, artistID string) (playback.ArtistDetail, error) {
	var res artistResult
	if err := c.call(ctx, "artist", map[string]any{"artistId": artistID}, &res); err != nil {
		return playback.ArtistDetail{}, err
	}
	return res.toDomain(), nil
}

// Album loads a catalog album page; the id travels as "albumId".
func (c *Client) Album(ctx context.Context, albumID string) (playback.AlbumDetail, error) {
	var res albumResult
	if err := c.call(ctx, "album", map[string]any{"albumId": albumID}, &res); err != nil {
		return playback.AlbumDetail{}, err
	}
	return res.toDomain(), nil
}

// SongAlbum loads the page of the album that contains a catalog song; the
// id travels as "songId".
func (c *Client) SongAlbum(ctx context.Context, songID string) (playback.AlbumDetail, error) {
	var res albumResult
	if err := c.call(ctx, "songAlbum", map[string]any{"songId": songID}, &res); err != nil {
		return playback.AlbumDetail{}, err
	}
	return res.toDomain(), nil
}

// CatalogPlaylist loads a catalog playlist page; the id travels as
// "playlistId".
func (c *Client) CatalogPlaylist(ctx context.Context, playlistID string) (playback.PlaylistDetail, error) {
	var res catalogPlaylistResult
	if err := c.call(ctx, "catalogPlaylist", map[string]any{"playlistId": playlistID}, &res); err != nil {
		return playback.PlaylistDetail{}, err
	}
	return res.toDomain(), nil
}

// Playlists lists the user's library playlists, alphabetically, with
// their Apple Music API library ids ("p.…") and whether they are
// editable.
func (c *Client) Playlists(ctx context.Context) ([]playback.Playlist, error) {
	var res playlistsResult
	if err := c.call(ctx, "playlists", nil, &res); err != nil {
		return nil, err
	}
	lists := make([]playback.Playlist, len(res.Playlists))
	for i, p := range res.Playlists {
		lists[i] = playback.Playlist{ID: p.ID, Name: p.Name, Editable: p.Editable}
	}
	return lists, nil
}

// LibraryPlaylist loads a library playlist page: its songs (music videos
// are left out) with their catalog ids; a song not in the catalog keeps
// its library id ("i.…") and is marked LibraryOnly. The id travels as
// "playlistId".
func (c *Client) LibraryPlaylist(ctx context.Context, playlistID string) (playback.PlaylistDetail, error) {
	var res catalogPlaylistResult
	if err := c.call(ctx, "libraryPlaylist", map[string]any{"playlistId": playlistID}, &res); err != nil {
		return playback.PlaylistDetail{}, err
	}
	return res.toDomain(), nil
}

// PlaySongs queues the catalog songs and starts playing at index start.
func (c *Client) PlaySongs(ctx context.Context, ids []string, start int) error {
	return c.call(ctx, "playSongs", map[string]any{"ids": ids, "startIndex": start}, nil)
}

// PlayPlaylist queues a library playlist and starts playing it.
//
// The playlist id travels as "playlistId": the request's own "id" key is the
// correlation id, so an argument named "id" can never reach the helper.
func (c *Client) PlayPlaylist(ctx context.Context, id string) error {
	return c.call(ctx, "playPlaylist", map[string]any{"playlistId": id}, nil)
}

// PlayPlaylistFrom queues a library playlist and starts playing at the song
// with index start in its libraryPlaylist tracks: the same command as
// PlayPlaylist, with "startIndex".
func (c *Client) PlayPlaylistFrom(ctx context.Context, playlistID string, start int) error {
	return c.call(ctx, "playPlaylist", map[string]any{"playlistId": playlistID, "startIndex": start}, nil)
}

// Pause pauses playback.
func (c *Client) Pause(ctx context.Context) error { return c.call(ctx, "pause", nil, nil) }

// Resume resumes playback.
func (c *Client) Resume(ctx context.Context) error { return c.call(ctx, "resume", nil, nil) }

// Next skips to the next queue entry.
func (c *Client) Next(ctx context.Context) error { return c.call(ctx, "next", nil, nil) }

// Previous skips to the previous queue entry.
func (c *Client) Previous(ctx context.Context) error { return c.call(ctx, "previous", nil, nil) }

// Stop stops playback.
func (c *Client) Stop(ctx context.Context) error { return c.call(ctx, "stop", nil, nil) }

// Seek moves the playhead to position within the current entry.
func (c *Client) Seek(ctx context.Context, position time.Duration) error {
	return c.call(ctx, "seek", map[string]any{"seconds": position.Seconds()}, nil)
}

// SetRepeat sets the player's repeat mode; it travels as "mode" ("off",
// "all" or "one") and waits its turn with the other playback commands.
func (c *Client) SetRepeat(ctx context.Context, mode playback.RepeatMode) error {
	return c.call(ctx, "setRepeat", map[string]any{"mode": string(mode)}, nil)
}

// Volume reports the system output volume, 0...1.
func (c *Client) Volume(ctx context.Context) (float64, error) {
	var res volumeResult
	if err := c.call(ctx, "volume", nil, &res); err != nil {
		return 0, err
	}
	return playback.ClampVolume(res.Level), nil
}

// SetVolume sets the system output volume. The level is clamped here, as
// JSON cannot carry NaN, and again by the helper.
func (c *Client) SetVolume(ctx context.Context, level float64) error {
	return c.call(ctx, "setVolume", map[string]any{"level": playback.ClampVolume(level)}, nil)
}

// CreatePlaylist creates a library playlist through the Apple Music API.
// The songs travel as "songIds", always an array (possibly empty); an
// empty description is left out. The helper accepts catalog song ids and
// API library ids ("i.…"), as LibraryPlaylist lists them, and answers
// with the playlist's API library id ("p.…").
func (c *Client) CreatePlaylist(ctx context.Context, name, description string, songIDs []string) (playback.Playlist, error) {
	args := map[string]any{"name": name, "songIds": nonNil(songIDs)}
	if description != "" {
		args["description"] = description
	}
	var res createPlaylistResult
	if err := c.call(ctx, "createPlaylist", args, &res); err != nil {
		return playback.Playlist{}, err
	}
	return playback.Playlist{ID: res.ID, Name: res.Name}, nil
}

// AddToPlaylist appends songs to a library playlist through the Apple
// Music API. The playlist id is an API library id ("p.…", as Playlists
// and CreatePlaylist return); the ids travel as "playlistId" and
// "songIds".
func (c *Client) AddToPlaylist(ctx context.Context, playlistID string, songIDs []string) error {
	return c.call(ctx, "addToPlaylist", map[string]any{"playlistId": playlistID, "songIds": nonNil(songIDs)}, nil)
}

// Favorite reports whether a song is a favorite (loved); the id travels
// as "songId".
func (c *Client) Favorite(ctx context.Context, songID string) (bool, error) {
	var res favoriteResult
	if err := c.call(ctx, "favorite", map[string]any{"songId": songID}, &res); err != nil {
		return false, err
	}
	return res.Favorite, nil
}

// Favorites reports which songs are favorites (loved) in one command; the
// ids travel as "songIds". The helper reads the ratings in batches; an id
// its answer leaves out is not a favorite. No ids send nothing.
func (c *Client) Favorites(ctx context.Context, songIDs []string) (map[string]bool, error) {
	loved := make(map[string]bool, len(songIDs))
	if len(songIDs) == 0 {
		return loved, nil
	}
	var res favoritesResult
	if err := c.call(ctx, "favorites", map[string]any{"songIds": songIDs}, &res); err != nil {
		return nil, err
	}
	for _, id := range songIDs {
		loved[id] = res.Favorites[id]
	}
	return loved, nil
}

// SetFavorite loves a song (on) or clears its rating; the arguments
// travel as "songId" and "on".
func (c *Client) SetFavorite(ctx context.Context, songID string, on bool) error {
	return c.call(ctx, "setFavorite", map[string]any{"songId": songID, "on": on}, nil)
}

// nonNil keeps a nil list from travelling as JSON null.
func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}
