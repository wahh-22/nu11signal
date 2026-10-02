package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// musicTree builds a small music folder:
//
//	root/Album A/a.mp3 (track 2, "Second"), b.mp3 (track 1, "First"), c.wav
//	root/Album A/loop -> root (a symlink loop)
//	root/Album A/.hidden/h.wav, root/.git/g.wav, root/Loose/.x.wav (hidden)
//	root/Loose/z.wav, root/Loose/tones.ogg (tagged fixture)
//	root/mix.m3u8 (b.mp3, Loose/z.wav absolute, a missing entry)
func musicTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mp3 := readFixture(t, "sine.mp3")
	writeFile(t, filepath.Join(root, "Album A", "a.mp3"), retagMP3(t, mp3, "Second", "2"))
	writeFile(t, filepath.Join(root, "Album A", "b.mp3"), retagMP3(t, mp3, "First", "1/9"))
	tone := sine(440, 0.5, 8000)
	writeWAV(t, filepath.Join(root, "Album A", "c.wav"), 8000, 1, 4000, wavPCM16, tone)
	writeWAV(t, filepath.Join(root, "Album A", ".hidden", "h.wav"), 8000, 1, 800, wavPCM16, tone)
	writeWAV(t, filepath.Join(root, ".git", "g.wav"), 8000, 1, 800, wavPCM16, tone)
	writeWAV(t, filepath.Join(root, "Loose", ".x.wav"), 8000, 1, 800, wavPCM16, tone)
	writeWAV(t, filepath.Join(root, "Loose", "z.wav"), 8000, 2, 8000, wavPCM16, tone)
	writeFile(t, filepath.Join(root, "Loose", "tones.ogg"), readFixture(t, "sine.ogg"))
	writeFile(t, filepath.Join(root, "Loose", "notes.txt"), []byte("not audio"))
	if err := os.Symlink(root, filepath.Join(root, "Album A", "loop")); err != nil {
		t.Fatal(err)
	}
	m3u := strings.Join([]string{
		"#EXTM3U",
		"#EXTINF:1,First",
		"Album A/b.mp3",
		"",
		filepath.Join(root, "Loose", "z.wav"),
		"missing.mp3",
	}, "\r\n")
	writeFile(t, filepath.Join(root, "mix.m3u8"), []byte(m3u))
	return root
}

func titles(lib *Library, d playback.PlaylistDetail) []string {
	var out []string
	for _, s := range d.Tracks {
		out = append(out, s.Title)
	}
	return out
}

func TestScanBuildsFolderAndM3UPlaylists(t *testing.T) {
	root := musicTree(t)
	lib, err := Scan(context.Background(), []string{root, filepath.Join(root, "does-not-exist")})
	if err != nil {
		t.Fatal(err)
	}
	pls := lib.Playlists()
	var names []string
	for _, p := range pls {
		names = append(names, p.Name)
		if !strings.HasPrefix(p.ID, "local:pl:") || p.Editable {
			t.Errorf("playlist %+v: want a local:pl: id, not editable", p)
		}
	}
	if want := []string{"Album A", "Loose", "mix"}; !slices.Equal(names, want) {
		t.Fatalf("playlists = %q, want %q", names, want)
	}

	album, ok := lib.Playlist(pls[0].ID)
	if !ok {
		t.Fatal("Album A not found by id")
	}
	// Numbered tracks first, in order; untagged ones after, by file name.
	if got, want := titles(lib, album), []string{"First", "Second", "c"}; !slices.Equal(got, want) {
		t.Errorf("Album A = %q, want %q", got, want)
	}
	c := album.Tracks[2]
	if c.Album != "Album A" || c.Artist != "" || c.Duration != 500*time.Millisecond {
		t.Errorf("untagged song = %+v, want the folder as album and a 500ms duration", c)
	}
	first := album.Tracks[0]
	if first.Duration < 400*time.Millisecond || first.Duration > 700*time.Millisecond {
		t.Errorf("mp3 duration = %v, want about 500ms", first.Duration)
	}
	if !strings.HasPrefix(first.ID, "local:") {
		t.Errorf("song id %q lacks the local: prefix", first.ID)
	}

	loose, _ := lib.Playlist(pls[1].ID)
	if got, want := titles(lib, loose), []string{"Sine", "z"}; !slices.Equal(got, want) {
		t.Errorf("Loose = %q, want %q (hidden files skipped)", got, want)
	}
	if s := loose.Tracks[0]; s.Artist != "Fixture" || s.Album != "Tones" {
		t.Errorf("tagged ogg = %+v, want artist Fixture, album Tones", s)
	}

	mix, _ := lib.Playlist(pls[2].ID)
	if got, want := titles(lib, mix), []string{"First", "z"}; !slices.Equal(got, want) {
		t.Errorf("mix = %q, want %q", got, want)
	}
	if mix.Tracks[0].ID != first.ID {
		t.Error("an m3u entry and the folder song have different ids")
	}
	if _, ok := lib.Song(first.ID); !ok {
		t.Error("Song does not find a scanned song")
	}
	if lib.path(first.ID) != filepath.Join(root, "Album A", "b.mp3") {
		t.Errorf("path = %q", lib.path(first.ID))
	}
}

func TestScanIDsAreStableAndOrderDeterministic(t *testing.T) {
	root := musicTree(t)
	a, err := Scan(context.Background(), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Scan(context.Background(), []string{root, root})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.Playlists(), b.Playlists()) {
		t.Fatalf("playlists differ between scans:\n%v\n%v", a.Playlists(), b.Playlists())
	}
	for _, p := range a.Playlists() {
		da, _ := a.Playlist(p.ID)
		db, _ := b.Playlist(p.ID)
		if !slices.Equal(da.Tracks, db.Tracks) {
			t.Errorf("%s: tracks differ between scans", p.Name)
		}
	}
	if songID("/x/y.mp3") != songID("/x/./z/../y.mp3") || songID("/x/y.mp3") == songID("/x/z.mp3") {
		t.Error("song ids must hash the clean path")
	}
	if playlistID("/x") == songID("/x") {
		t.Error("playlist and song ids must not collide")
	}
}

func TestScanCancelled(t *testing.T) {
	root := musicTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, []string{root}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestMP3DurationEstimate(t *testing.T) {
	d, err := mp3Duration(filepath.Join("testdata", "sine.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	if d < 400*time.Millisecond || d > 700*time.Millisecond {
		t.Errorf("duration = %v, want about 500ms", d)
	}
}
