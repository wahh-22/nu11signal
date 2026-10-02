package local

import (
	"errors"
	"testing"

	"github.com/wahh-22/nu11signal/internal/playback"
)

func TestPlayerStartsEmptyAndTakesALibraryLater(t *testing.T) {
	p, _ := newTestPlayer(t, nil)
	if pls, err := p.Playlists(ctx); err != nil || len(pls) != 0 {
		t.Fatalf("before a library: Playlists = %v, %v; want none", pls, err)
	}
	lib, ids := testLibrary(t)
	p.SetLibrary(lib)
	select {
	case <-p.LibraryChanged():
	default:
		t.Fatal("SetLibrary did not signal LibraryChanged")
	}
	pls, err := p.Playlists(ctx)
	if err != nil || len(pls) != 1 {
		t.Fatalf("after SetLibrary: Playlists = %v, %v", pls, err)
	}
	if pls[0].Source != playback.SourceLocal {
		t.Errorf("playlist source = %q, want local", pls[0].Source)
	}
	if _, err := p.PlaySongs(ctx, ids, 0); err != nil {
		t.Fatalf("PlaySongs after SetLibrary: %v", err)
	}
}

func TestLibraryChangedIsClosedByClose(t *testing.T) {
	p, _ := newTestPlayer(t, nil)
	_ = p.Close()
	if _, ok := <-p.LibraryChanged(); ok {
		t.Error("LibraryChanged still open after Close")
	}
	p.SetLibrary(nil) // no panic after Close
}

func TestErrUnsupportedIsThePorts(t *testing.T) {
	if !errors.Is(ErrUnsupported, playback.ErrUnsupported) {
		t.Error("local.ErrUnsupported is not playback.ErrUnsupported")
	}
	if !playbackHasLocalPrefix(songID("/x.mp3")) || !playbackHasLocalPrefix(playlistID("/x")) {
		t.Error("local ids are not in the port's local namespace")
	}
}

func playbackHasLocalPrefix(id string) bool { return playback.SourceOf(id) == playback.SourceLocal }

var _ playback.LibraryWatcher = (*Player)(nil)
