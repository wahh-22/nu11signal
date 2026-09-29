package helper

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

var _ playback.Player = (*Client)(nil)

func TestRoundTrip(t *testing.T) {
	c := startFake(t, "standard", Options{})
	ctx := t.Context()

	status, err := c.Authorize(ctx)
	if err != nil || status != playback.AuthAuthorized {
		t.Fatalf("Authorize = %q, %v; want authorized", status, err)
	}

	lists, err := c.Playlists(ctx)
	if err != nil || len(lists) != 1 || lists[0] != (playback.Playlist{ID: "p1", Name: "Night City"}) {
		t.Fatalf("Playlists = %+v, %v", lists, err)
	}

	commands := []struct {
		name string
		call func() error
	}{
		{"playSongs", func() error { return c.PlaySongs(ctx, []string{"s1", "s2"}, 1) }},
		{"playPlaylist", func() error { return c.PlayPlaylist(ctx, "p1") }},
		{"playPlaylistFrom", func() error { return c.PlayPlaylistFrom(ctx, "p1", 2) }},
		{"pause", func() error { return c.Pause(ctx) }},
		{"seek", func() error { return c.Seek(ctx, 90*time.Second+500*time.Millisecond) }},
	}
	for _, tt := range commands {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
		})
	}
}

func TestSearchCatalogRoundTrip(t *testing.T) {
	c := startFake(t, "standard", Options{})

	got, err := c.SearchCatalog(t.Context(), "daft", 3)
	if err != nil {
		t.Fatalf("SearchCatalog: %v", err)
	}
	daftPunk := playback.Artist{ID: "a1", Name: "Daft Punk", Genres: []string{"Electronic", "Dance"}}
	discovery := playback.Album{ID: "al1", Title: "Discovery", Artist: "Daft Punk", Year: 2001, TrackCount: 14}
	oneMoreTime := playback.Song{ID: "s1", Title: "One More Time", Artist: "Daft Punk", Album: "Discovery", Duration: 320*time.Second + 500*time.Millisecond}
	essentials := playback.CatalogPlaylist{ID: "pl1", Name: "Daft Punk Essentials", Curator: "Apple Music Electronic"}
	want := playback.SearchResults{
		Suggestions: []string{"daft punk", "daft punk discovery"},
		// The station top result is a kind the UI cannot open: dropped.
		Top: []playback.SearchItem{
			{Kind: playback.ItemArtist, Artist: daftPunk},
			{Kind: playback.ItemSong, Song: oneMoreTime},
			{Kind: playback.ItemAlbum, Album: discovery},
			{Kind: playback.ItemPlaylist, Playlist: essentials},
		},
		Artists:   []playback.Artist{daftPunk},
		Albums:    []playback.Album{discovery},
		Songs:     []playback.Song{oneMoreTime},
		Playlists: []playback.CatalogPlaylist{essentials},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SearchCatalog = %+v; want %+v", got, want)
	}
}

func TestSearchCatalogToleratesMissingFields(t *testing.T) {
	c := startFake(t, "sparseCatalog", Options{})
	ctx := t.Context()

	empty, err := c.SearchCatalog(ctx, "empty", 5)
	if err != nil {
		t.Fatalf("SearchCatalog(empty): %v", err)
	}
	if !reflect.DeepEqual(empty, playback.SearchResults{}) {
		t.Fatalf("SearchCatalog(empty) = %+v; want no results", empty)
	}

	sparse, err := c.SearchCatalog(ctx, "sparse", 5)
	if err != nil {
		t.Fatalf("SearchCatalog(sparse): %v", err)
	}
	if len(sparse.Suggestions) != 0 {
		t.Errorf("Suggestions = %v; want none", sparse.Suggestions)
	}
	if len(sparse.Artists) != 1 || sparse.Artists[0].ID != "a1" || sparse.Artists[0].Name != "Daft Punk" || len(sparse.Artists[0].Genres) != 0 {
		t.Errorf("Artists = %+v", sparse.Artists)
	}
	if len(sparse.Songs) != 1 || sparse.Songs[0] != (playback.Song{ID: "s1", Title: "One More Time"}) {
		t.Errorf("Songs = %+v", sparse.Songs)
	}
	if len(sparse.Albums) != 1 || sparse.Albums[0] != (playback.Album{ID: "al1", Title: "Discovery"}) {
		t.Errorf("Albums = %+v", sparse.Albums)
	}
	if len(sparse.Playlists) != 1 || sparse.Playlists[0] != (playback.CatalogPlaylist{ID: "pl1", Name: "Mix"}) {
		t.Errorf("Playlists = %+v", sparse.Playlists)
	}
	// Top results without a kind, of an unknown kind or without their
	// payload are dropped; the rest keep their order.
	wantTop := []playback.SearchItem{{Kind: playback.ItemSong, Song: playback.Song{ID: "s1", Title: "One More Time"}}}
	if !reflect.DeepEqual(sparse.Top, wantTop) {
		t.Errorf("Top = %+v; want %+v", sparse.Top, wantTop)
	}
}

func TestSearchCatalogErrorResponse(t *testing.T) {
	c := startFake(t, "sparseCatalog", Options{})

	_, err := c.SearchCatalog(t.Context(), "down", 5)
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("SearchCatalog error = %v; want *CommandError", err)
	}
	if cmdErr.Command != "searchCatalog" || cmdErr.Message != "catalog unavailable" {
		t.Fatalf("CommandError = %+v", cmdErr)
	}
}

func TestArtistRoundTrip(t *testing.T) {
	c := startFake(t, "standard", Options{})

	got, err := c.Artist(t.Context(), "a1")
	if err != nil {
		t.Fatalf("Artist: %v", err)
	}
	discovery := playback.Album{ID: "al1", Title: "Discovery", Artist: "Daft Punk", Year: 2001, TrackCount: 14}
	want := playback.ArtistDetail{
		Artist: playback.Artist{ID: "a1", Name: "Daft Punk", Genres: []string{"Electronic"}},
		TopSongs: []playback.Song{
			{ID: "s1", Title: "One More Time", Artist: "Daft Punk", Album: "Discovery", Duration: 320*time.Second + 500*time.Millisecond},
		},
		EssentialAlbums: []playback.Album{discovery},
		Albums:          []playback.Album{discovery, {ID: "al2", Title: "Homework", Artist: "Daft Punk", Year: 1997, TrackCount: 16}},
		Singles:         []playback.Album{{ID: "sg1", Title: "Get Lucky", Artist: "Daft Punk", Year: 2013, TrackCount: 1}},
		Compilations:    []playback.Album{{ID: "c1", Title: "Musique, Vol. 1", Artist: "Daft Punk", Year: 2006, TrackCount: 15}},
		Playlists:       []playback.CatalogPlaylist{{ID: "pl1", Name: "Daft Punk Essentials", Curator: "Apple Music Electronic"}},
		About:           playback.ArtistAbout{Notes: "French duo.", Genre: "Electronic", Origin: "Paris, France", Formed: "1993"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Artist = %+v; want %+v", got, want)
	}
}

func TestArtistToleratesMissingSections(t *testing.T) {
	c := startFake(t, "sparseArtist", Options{})
	ctx := t.Context()

	empty, err := c.Artist(ctx, "empty")
	if err != nil {
		t.Fatalf("Artist(empty): %v", err)
	}
	if !reflect.DeepEqual(empty, playback.ArtistDetail{}) {
		t.Fatalf("Artist(empty) = %+v; want a zero page", empty)
	}

	sparse, err := c.Artist(ctx, "sparse")
	if err != nil {
		t.Fatalf("Artist(sparse): %v", err)
	}
	want := playback.ArtistDetail{
		Artist: playback.Artist{ID: "a1", Name: "Daft Punk"},
		Albums: []playback.Album{{ID: "al1", Title: "Discovery"}},
		About:  playback.ArtistAbout{Genre: "Electronic"},
	}
	if !reflect.DeepEqual(sparse, want) {
		t.Fatalf("Artist(sparse) = %+v; want %+v", sparse, want)
	}
}

func TestArtistErrorResponse(t *testing.T) {
	c := startFake(t, "sparseArtist", Options{})

	_, err := c.Artist(t.Context(), "gone")
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Artist error = %v; want *CommandError", err)
	}
	if cmdErr.Command != "artist" || cmdErr.Message != "artist not found" {
		t.Fatalf("CommandError = %+v", cmdErr)
	}
}

func TestErrorResponseBecomesCommandError(t *testing.T) {
	c := startFake(t, "standard", Options{})

	err := c.Next(t.Context())
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Next error = %v; want *CommandError", err)
	}
	if cmdErr.Command != "next" || cmdErr.Message != "queue is empty" {
		t.Fatalf("CommandError = %+v", cmdErr)
	}
}

func TestOutOfOrderResponsesAreCorrelated(t *testing.T) {
	c := startFake(t, "reorder", Options{})
	ctx := t.Context()

	var wg sync.WaitGroup
	var found playback.SearchResults
	var lists []playback.Playlist
	var searchErr, listErr error
	wg.Go(func() { found, searchErr = c.SearchCatalog(ctx, "daft", 3) })
	wg.Go(func() { lists, listErr = c.Playlists(ctx) })
	wg.Wait()

	if searchErr != nil || len(found.Songs) != 1 || found.Songs[0].ID != "s1" {
		t.Fatalf("SearchCatalog = %+v, %v", found, searchErr)
	}
	if listErr != nil || len(lists) != 1 || lists[0].ID != "p1" {
		t.Fatalf("Playlists = %+v, %v", lists, listErr)
	}
}

func TestStateEventsAreDecoded(t *testing.T) {
	c := startFake(t, "standard", Options{})

	if err := c.Resume(t.Context()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	got := <-c.States()
	want := playback.State{
		Status: playback.StatusPlaying, Title: "One More Time", Artist: "Daft Punk",
		Album: "Discovery", SongID: "s1",
		Duration: 320*time.Second + 500*time.Millisecond,
		Position: 12*time.Second + 250*time.Millisecond,
	}
	if got != want {
		t.Fatalf("state = %+v; want %+v", got, want)
	}
}

func TestAsyncErrorsAreDelivered(t *testing.T) {
	c := startFake(t, "standard", Options{})
	ctx := t.Context()

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{"error event", func() error { return c.Previous(ctx) }, "failed to encode message"},
		{"response without id", func() error { return c.Stop(ctx) }, "malformed JSON request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if err := <-c.Errors(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("async error = %v; want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestContextCancelsPendingCall(t *testing.T) {
	c := startFake(t, "silentAuth", Options{})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Authorize(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Authorize error = %v; want deadline exceeded", err)
	}
	// The client stays usable after a cancelled call.
	if err := c.Pause(t.Context()); err != nil {
		t.Fatalf("Pause after cancel: %v", err)
	}
}

func TestCrashFailsPendingCallsAndClosesChannels(t *testing.T) {
	var stderr bytes.Buffer
	c := startFake(t, "crash", Options{Stderr: &stderr})

	_, err := c.SearchCatalog(t.Context(), "daft", 3)
	if !errors.Is(err, ErrHelperExited) {
		t.Fatalf("SearchCatalog error = %v; want ErrHelperExited", err)
	}
	if !strings.Contains(err.Error(), "fatal: helper crashed") {
		t.Fatalf("error %q does not include the helper's stderr", err)
	}
	if !strings.Contains(stderr.String(), "fatal: helper crashed") {
		t.Fatalf("caller stderr writer got %q", stderr.String())
	}
	for range c.States() {
	}
	for range c.Errors() {
	}
	if err := c.Pause(t.Context()); !errors.Is(err, ErrHelperExited) {
		t.Fatalf("Pause after crash = %v; want ErrHelperExited", err)
	}
}

func TestCloseStopsCleanHelper(t *testing.T) {
	c := startFake(t, "standard", Options{})

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := c.Pause(t.Context()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Pause after Close = %v; want ErrClosed", err)
	}
	for range c.States() {
	}
}

func TestCloseKillsHelperThatIgnoresEOF(t *testing.T) {
	c := startFake(t, "stubborn", Options{CloseTimeout: 100 * time.Millisecond})

	done := make(chan error, 1)
	go func() { done <- c.Close() }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "killed") {
			t.Fatalf("Close = %v; want a killed error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not kill the helper")
	}
	for range c.Errors() {
	}
}

func TestWriteToStuckHelperIsBoundedByContext(t *testing.T) {
	c := startFake(t, "deaf", Options{CloseTimeout: 100 * time.Millisecond})

	// Far larger than any pipe buffer, so the write cannot complete.
	term := strings.Repeat("x", 1<<20)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := c.SearchCatalog(ctx, term, 1)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("SearchCatalog error = %v; want deadline exceeded", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SearchCatalog blocked on a helper that never reads stdin")
	}
}

func TestSendAfterHelperStopsReadingIsHelperExited(t *testing.T) {
	c := startFake(t, "closedStdin", Options{CloseTimeout: 100 * time.Millisecond})

	// The helper closes stdin right after ready; retry until the write
	// observes the broken pipe. A write that lands in the pipe buffer
	// before the helper closes stdin is never answered, so every attempt
	// is bounded and a timed-out attempt is retried.
	deadline := time.Now().Add(5 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
		err := c.Pause(ctx)
		cancel()
		if errors.Is(err, ErrHelperExited) {
			return
		}
		if err == nil || time.Now().After(deadline) {
			t.Fatalf("Pause error = %v; want ErrHelperExited", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "send") {
			t.Fatalf("Pause error = %v; want a send failure", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCloseIsBoundedWhileAWriteIsStuck(t *testing.T) {
	const closeTimeout = 100 * time.Millisecond
	c := startFake(t, "deaf", Options{CloseTimeout: closeTimeout})

	// Abandon a write that can never complete, then queue another request
	// behind it; neither may hold Close past its timeout.
	term := strings.Repeat("x", 1<<20)
	for range 2 {
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		if _, err := c.SearchCatalog(ctx, term, 1); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("SearchCatalog error = %v; want deadline exceeded", err)
		}
		cancel()
	}

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- c.Close() }()
	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > closeTimeout+2*time.Second {
			t.Fatalf("Close took %s; want about %s", elapsed, closeTimeout)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked behind a stuck write")
	}
	if err := c.Pause(t.Context()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Pause after Close = %v; want ErrClosed", err)
	}
}

func TestUndecodableOutputKillsHelper(t *testing.T) {
	c := startFake(t, "oversize", Options{})

	var asyncErrs []error
	for err := range c.Errors() { // closed once the killed helper is reaped
		asyncErrs = append(asyncErrs, err)
	}
	if len(asyncErrs) == 0 || !strings.Contains(asyncErrs[0].Error(), "read output") {
		t.Fatalf("async errors = %v; want a read output error", asyncErrs)
	}
	for range c.States() {
	}
	if err := c.Pause(t.Context()); !errors.Is(err, ErrHelperExited) {
		t.Fatalf("Pause after undecodable output = %v; want ErrHelperExited", err)
	}
}

// Start's ctx bounds only the startup: cancelling it afterwards (as callers
// do with a deferred cancel) must not stop the helper.
func TestStartContextOnlyBoundsStartup(t *testing.T) {
	exe, _ := os.Executable()
	ctx, cancel := context.WithCancel(t.Context())
	c, err := Start(ctx, Options{Path: exe, Env: fakeHelperEnv("standard")})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	cancel()
	time.Sleep(50 * time.Millisecond)
	if err := c.Pause(t.Context()); err != nil {
		t.Fatalf("Pause after cancelling the startup context: %v", err)
	}
}

func TestStartHonoursCancelledContext(t *testing.T) {
	exe, _ := os.Executable()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Start(ctx, Options{Path: exe, Env: fakeHelperEnv("mute"), ReadyTimeout: 10 * time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v; want context.Canceled", err)
	}
}

func TestStartFailures(t *testing.T) {
	tests := []struct {
		scenario string
		want     string
	}{
		{"mute", "ready"},
		{"earlyExit", "fatal: no entitlement"},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			exe, _ := os.Executable()
			_, err := Start(t.Context(), Options{
				Path:         exe,
				Env:          fakeHelperEnv(tt.scenario),
				ReadyTimeout: 200 * time.Millisecond,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Start error = %v; want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestOfferKeepsLatestWithoutBlocking(t *testing.T) {
	ch := make(chan int, 3)
	for i := 1; i <= 10; i++ {
		offer(ch, i) // never blocks, even with no reader
	}
	close(ch)
	var got []int
	for v := range ch {
		got = append(got, v)
	}
	if len(got) != 3 || got[2] != 10 {
		t.Fatalf("buffered = %v; want the 3 newest ending in 10", got)
	}
}

func TestAlbumAndSongAlbumRoundTrip(t *testing.T) {
	c := startFake(t, "standard", Options{})
	song := func(id, title string, d time.Duration, n int) playback.Track {
		return playback.Track{Song: playback.Song{ID: id, Title: title, Artist: "Daft Punk", Album: "Discovery", Duration: d}, Number: n, Disc: 1}
	}
	want := playback.AlbumDetail{
		Album:       playback.Album{ID: "al1", Title: "Discovery", Artist: "Daft Punk", Year: 2001, TrackCount: 2},
		Tracks:      []playback.Track{song("s1", "One More Time", 320*time.Second+500*time.Millisecond, 1), song("s2", "Aerodynamic", 207*time.Second, 2)},
		Genre:       "Electronic",
		ReleaseDate: "2001-03-07",
		RecordLabel: "Parlophone",
		Copyright:   "℗ 2001 Daft Life Ltd.",
		Notes:       "The robots arrive.",
	}
	// The fake helper answers only the expected argument names
	// ("albumId", "songId"), so a mismatch fails the call.
	got, err := c.Album(t.Context(), "al1")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Album = %+v, %v; want %+v", got, err, want)
	}
	got, err = c.SongAlbum(t.Context(), "s1")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("SongAlbum = %+v, %v; want %+v", got, err, want)
	}
}

func TestCatalogPlaylistRoundTrip(t *testing.T) {
	c := startFake(t, "standard", Options{})
	got, err := c.CatalogPlaylist(t.Context(), "pl1")
	want := playback.PlaylistDetail{
		Playlist: playback.CatalogPlaylist{ID: "pl1", Name: "Daft Punk Essentials", Curator: "Apple Music Electronic"},
		Tracks:   []playback.Song{{ID: "s3", Title: "Get Lucky", Artist: "Daft Punk", Album: "Random Access Memories", Duration: 369 * time.Second}},
		Notes:    "Robot rock.",
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("CatalogPlaylist = %+v, %v; want %+v", got, err, want)
	}
}

func TestDetailsTolerateMissingFields(t *testing.T) {
	c := startFake(t, "sparseDetail", Options{})
	ctx := t.Context()

	for _, load := range []func(string) (playback.AlbumDetail, error){
		func(id string) (playback.AlbumDetail, error) { return c.Album(ctx, id) },
		func(id string) (playback.AlbumDetail, error) { return c.SongAlbum(ctx, id) },
	} {
		empty, err := load("empty")
		if err != nil || !reflect.DeepEqual(empty, playback.AlbumDetail{}) {
			t.Errorf("empty album = %+v, %v; want a zero page", empty, err)
		}
		sparse, err := load("sparse")
		want := playback.AlbumDetail{
			Album:  playback.Album{ID: "al1", Title: "Discovery"},
			Tracks: []playback.Track{{Song: playback.Song{ID: "s1", Title: "One More Time"}}},
		}
		if err != nil || !reflect.DeepEqual(sparse, want) {
			t.Errorf("sparse album = %+v, %v; want %+v", sparse, err, want)
		}
	}

	for name, load := range map[string]func(string) (playback.PlaylistDetail, error){
		"catalog": func(id string) (playback.PlaylistDetail, error) { return c.CatalogPlaylist(ctx, id) },
		"library": func(id string) (playback.PlaylistDetail, error) { return c.LibraryPlaylist(ctx, id) },
	} {
		empty, err := load("empty")
		if err != nil || !reflect.DeepEqual(empty, playback.PlaylistDetail{}) {
			t.Errorf("empty %s playlist = %+v, %v; want a zero page", name, empty, err)
		}
		sparse, err := load("sparse")
		want := playback.PlaylistDetail{
			Playlist: playback.CatalogPlaylist{ID: "pl1", Name: "Mix"},
			Tracks:   []playback.Song{{ID: "s1", Title: "One More Time"}},
		}
		if err != nil || !reflect.DeepEqual(sparse, want) {
			t.Errorf("sparse %s playlist = %+v, %v; want %+v", name, sparse, err, want)
		}
	}
}

func TestDetailErrorResponses(t *testing.T) {
	c := startFake(t, "sparseDetail", Options{})
	ctx := t.Context()
	tests := []struct {
		command, message string
		call             func() error
	}{
		{"album", "album not found", func() error { _, err := c.Album(ctx, "gone"); return err }},
		{"songAlbum", "song not found", func() error { _, err := c.SongAlbum(ctx, "gone"); return err }},
		{"catalogPlaylist", "playlist not found", func() error { _, err := c.CatalogPlaylist(ctx, "gone"); return err }},
		{"libraryPlaylist", "playlist not found in the library", func() error { _, err := c.LibraryPlaylist(ctx, "gone"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var cmdErr *CommandError
			if err := tt.call(); !errors.As(err, &cmdErr) || cmdErr.Command != tt.command || cmdErr.Message != tt.message {
				t.Fatalf("error = %v; want CommandError{%s, %s}", err, tt.command, tt.message)
			}
		})
	}
}

func TestVolumeRoundTrip(t *testing.T) {
	c := startFake(t, "standard", Options{})
	ctx := t.Context()
	if v, err := c.Volume(ctx); err != nil || v != 0.42 {
		t.Fatalf("Volume = %v, %v; want 0.42", v, err)
	}
	// The fake helper answers only {"level": 0.25}, so a wrong argument
	// name fails the call.
	if err := c.SetVolume(ctx, 0.25); err != nil {
		t.Fatalf("SetVolume: %v", err)
	}
}

func TestSetVolumeClampsBeforeSending(t *testing.T) {
	// The "volume" scenario stores the level it is sent and reports it back.
	c := startFake(t, "volume", Options{})
	ctx := t.Context()
	tests := []struct {
		name      string
		set, want float64
	}{
		{"within range", 0.6, 0.6},
		{"above one", 1.7, 1},
		{"below zero", -0.3, 0},
		{"not a number", math.NaN(), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := c.SetVolume(ctx, tt.set); err != nil {
				t.Fatalf("SetVolume(%v): %v", tt.set, err)
			}
			if got, err := c.Volume(ctx); err != nil || got != tt.want {
				t.Fatalf("Volume after SetVolume(%v) = %v, %v; want %v", tt.set, got, err, tt.want)
			}
		})
	}
}

func TestVolumeErrorResponses(t *testing.T) {
	// The "volume" scenario refuses a level of exactly 0.5, as a device
	// without a settable volume refuses every level.
	c := startFake(t, "volume", Options{})
	ctx := t.Context()
	var cmdErr *CommandError
	err := c.SetVolume(ctx, 0.5)
	if !errors.As(err, &cmdErr) || cmdErr.Command != "setVolume" || cmdErr.Message != "output device has no settable volume" {
		t.Fatalf("SetVolume error = %v; want a setVolume CommandError", err)
	}
	// The refused level was not applied: the scenario's level stands.
	if v, err := c.Volume(ctx); err != nil || v != 0.3 {
		t.Fatalf("Volume after a failed set = %v, %v; want the level unchanged at 0.3", v, err)
	}
}

func TestVolumeOutOfRangeFromTheHelperIsClamped(t *testing.T) {
	// The "volumeOutOfRange" scenario reports 1.4, then -0.2.
	c := startFake(t, "volumeOutOfRange", Options{})
	ctx := t.Context()
	for _, want := range []float64{1, 0} {
		if v, err := c.Volume(ctx); err != nil || v != want {
			t.Fatalf("Volume = %v, %v; want %v", v, err, want)
		}
	}
}

func TestLibraryPlaylistRoundTrip(t *testing.T) {
	c := startFake(t, "standard", Options{})
	got, err := c.LibraryPlaylist(t.Context(), "p1")
	want := playback.PlaylistDetail{
		Playlist: playback.CatalogPlaylist{ID: "p1", Name: "Night City"},
		Tracks: []playback.Song{
			{ID: "i.s1", Title: "Nightcall", Artist: "Kavinsky", Album: "OutRun", Duration: 258 * time.Second},
			{ID: "i.s2", Title: "Resonance", Artist: "Home", Album: "Odyssey", Duration: 212*time.Second + 250*time.Millisecond},
		},
		Notes: "After hours.",
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("LibraryPlaylist = %+v, %v; want %+v", got, err, want)
	}
}

func TestLibraryEditsRoundTrip(t *testing.T) {
	// The standard scenario answers only the exact arguments in
	// expectedArgs, so a wrong argument name fails the call.
	c := startFake(t, "standard", Options{})
	ctx := t.Context()

	pl, err := c.CreatePlaylist(ctx, "Night Drive", "After hours", []string{"s1", "i.s2"})
	if err != nil || pl != (playback.Playlist{ID: "p.new", Name: "Night Drive"}) {
		t.Fatalf("CreatePlaylist = %+v, %v; want p.new", pl, err)
	}
	if err := c.AddToPlaylist(ctx, "p.new", []string{"s1"}); err != nil {
		t.Fatalf("AddToPlaylist: %v", err)
	}
	if on, err := c.Favorite(ctx, "s1"); err != nil || !on {
		t.Fatalf("Favorite = %v, %v; want true", on, err)
	}
	if err := c.SetFavorite(ctx, "s1", true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
}

func TestLibraryEditEdgeCasesAndErrors(t *testing.T) {
	c := startFake(t, "libraryEdit", Options{})
	ctx := t.Context()

	// No songs and no description: songIds still travels as an array,
	// and description is left out.
	pl, err := c.CreatePlaylist(ctx, "Empty", "", nil)
	if err != nil || pl != (playback.Playlist{ID: "p.empty", Name: "Empty"}) {
		t.Fatalf("CreatePlaylist(no songs) = %+v, %v; want p.empty", pl, err)
	}
	if on, err := c.Favorite(ctx, "unrated"); err != nil || on {
		t.Fatalf("Favorite(unrated) = %v, %v; want false", on, err)
	}
	if err := c.SetFavorite(ctx, "s1", false); err != nil {
		t.Fatalf("SetFavorite(off): %v", err)
	}

	tests := []struct {
		command, message string
		call             func() error
	}{
		{"addToPlaylist", "playlist is not editable (Forbidden)", func() error { return c.AddToPlaylist(ctx, "p.locked", []string{"s1"}) }},
		{"favorite", "Apple Music did not accept the credentials", func() error { _, err := c.Favorite(ctx, "denied"); return err }},
		{"createPlaylist", "Apple Music failed (HTTP 500)", func() error { _, err := c.CreatePlaylist(ctx, "boom", "", nil); return err }},
		{"setFavorite", `song "123456789012345678" has no Apple Music API id`, func() error { return c.SetFavorite(ctx, "123456789012345678", true) }},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var cmdErr *CommandError
			if err := tt.call(); !errors.As(err, &cmdErr) || cmdErr.Command != tt.command || cmdErr.Message != tt.message {
				t.Fatalf("error = %v; want CommandError{%s, %s}", err, tt.command, tt.message)
			}
		})
	}
}
