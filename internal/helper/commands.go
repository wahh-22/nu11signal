package helper

import (
	"context"
	"time"

	"soulking/internal/playback"
)

// Authorize asks for Apple Music access; the system may prompt the user.
func (c *Client) Authorize(ctx context.Context) (playback.AuthStatus, error) {
	var res authResult
	if err := c.call(ctx, "authorize", nil, &res); err != nil {
		return "", err
	}
	return playback.AuthStatus(res.Status), nil
}

// Search looks up catalog songs; the helper clamps limit to 1...25.
func (c *Client) Search(ctx context.Context, term string, limit int) ([]playback.Song, error) {
	var res searchResult
	if err := c.call(ctx, "search", map[string]any{"term": term, "limit": limit}, &res); err != nil {
		return nil, err
	}
	songs := make([]playback.Song, len(res.Songs))
	for i, s := range res.Songs {
		songs[i] = s.toDomain()
	}
	return songs, nil
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
