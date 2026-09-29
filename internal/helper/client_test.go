package helper

import (
	"bytes"
	"context"
	"errors"
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
	want := playback.SearchResults{
		Suggestions: []string{"daft punk", "daft punk discovery"},
		Artists:     []playback.Artist{{ID: "a1", Name: "Daft Punk", Genres: []string{"Electronic", "Dance"}}},
		Songs: []playback.Song{
			{ID: "s1", Title: "One More Time", Artist: "Daft Punk", Album: "Discovery", Duration: 320*time.Second + 500*time.Millisecond},
		},
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
	if len(empty.Suggestions) != 0 || len(empty.Artists) != 0 || len(empty.Songs) != 0 {
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
