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

// SearchCatalog runs a mixed catalog search: suggestions, artists and songs.
// The helper clamps limit to 1...25 per result type.
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

// Playlists lists the user's library playlists, sorted by name.
func (c *Client) Playlists(ctx context.Context) ([]playback.Playlist, error) {
	var res playlistsResult
	if err := c.call(ctx, "playlists", nil, &res); err != nil {
		return nil, err
	}
	lists := make([]playback.Playlist, len(res.Playlists))
	for i, p := range res.Playlists {
		lists[i] = playback.Playlist{ID: p.ID, Name: p.Name}
	}
	return lists, nil
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
