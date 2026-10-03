package local

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

const testRate = 8000

// testLibrary is a folder "Songs" of three 4 s mono songs at testRate:
// song i holds the constant level levelOf(i), so the audio says which
// song (and, with a ramp, where in it) is playing.
func testLibrary(t *testing.T) (*Library, []string) {
	t.Helper()
	root := t.TempDir()
	for i, name := range []string{"1 one.wav", "2 two.wav", "3 three.wav"} {
		writeWAV(t, filepath.Join(root, "Songs", name), testRate, 1, 4*testRate, wavPCM16,
			func(f, _ int) float64 { return levelOf(i) + float64(f)/(4*testRate)*0.1 })
	}
	lib, err := Scan(context.Background(), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := lib.Playlist(lib.Playlists()[0].ID)
	if !ok || len(d.Tracks) != 3 {
		t.Fatalf("library = %+v", d)
	}
	var ids []string
	for _, s := range d.Tracks {
		ids = append(ids, s.ID)
	}
	return lib, ids
}

func levelOf(song int) float64 { return 0.2 * float64(song+1) }

// songAt says which song a rendered sample belongs to, by its level.
func songAt(v float32) int {
	return int(math.Floor(float64(v)/0.2+0.25)) - 1
}

func newTestPlayer(t *testing.T, lib *Library) (*Player, *fakeSink) {
	t.Helper()
	s := &fakeSink{}
	p := newPlayer(lib, Options{SampleRate: testRate}, s, clock{after: func(time.Duration) <-chan time.Time {
		c := make(chan time.Time, 1)
		c <- time.Time{}
		return c
	}})
	t.Cleanup(func() { _ = p.Close() })
	return p, s
}

// lastState drains the states delivered so far and returns the newest.
func lastState(t *testing.T, p *Player) playback.State {
	t.Helper()
	var s playback.State
	got := false
	for {
		select {
		case s = <-p.States():
			got = true
		default:
			if !got {
				t.Fatal("no state emitted")
			}
			return s
		}
	}
}

var ctx = context.Background()

func TestPlaySongsReportsMissingAndPlays(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	if st, err := p.Authorize(ctx); err != nil || st != playback.AuthAuthorized {
		t.Fatalf("Authorize = %v, %v", st, err)
	}
	rep, err := p.PlaySongs(ctx, []string{"local:nope", ids[1], ids[2]}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Missing) != 1 || rep.Missing[0] != "local:nope" || len(rep.Skipped) != 0 {
		t.Errorf("report = %+v", rep)
	}
	s := lastState(t, p)
	if s.Status != playback.StatusPlaying || s.SongID != ids[1] || s.Title != "2 two" || s.Album != "Songs" ||
		s.Duration != 4*time.Second || s.VolumeMode != playback.VolumeApp || s.Repeat != playback.RepeatOff {
		t.Errorf("state = %+v", s)
	}
	out := sink.pull(testRate / 2)
	if v := out[len(out)-2]; songAt(v) != 1 || out[len(out)-1] != v {
		t.Errorf("rendered %v, want song 2 on both channels", v)
	}
	if out[0] > out[200] {
		t.Error("playback should fade in")
	}
	if _, err := p.PlaySongs(ctx, []string{"local:a", "local:b"}, 0); err == nil {
		t.Error("PlaySongs with only unknown ids should fail")
	}
	if _, err := p.PlaySongs(ctx, ids, 3); err == nil {
		t.Error("PlaySongs with start out of range should fail")
	}
}

func TestQueueAdvancesAndRepeats(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	if _, err := p.PlaySongs(ctx, ids, 1); err != nil {
		t.Fatal(err)
	}
	// The end of song 2 runs straight into song 3.
	out := sink.pull(4*testRate + testRate/2)
	if songAt(out[2*(4*testRate-10)]) != 1 || songAt(out[2*(4*testRate+10)]) != 2 {
		t.Errorf("around the end: %v then %v", out[2*(4*testRate-10)], out[2*(4*testRate+10)])
	}
	if s := lastState(t, p); s.SongID != ids[2] || s.Status != playback.StatusPlaying {
		t.Errorf("state after the end = %+v", s)
	}
	// RepeatOne starts song 3 over.
	if err := p.SetRepeat(ctx, playback.RepeatOne); err != nil {
		t.Fatal(err)
	}
	sink.pull(4 * testRate)
	if s := lastState(t, p); s.SongID != ids[2] || s.Repeat != playback.RepeatOne {
		t.Errorf("RepeatOne state = %+v", s)
	}
	// RepeatAll wraps to song 1.
	if err := p.SetRepeat(ctx, playback.RepeatAll); err != nil {
		t.Fatal(err)
	}
	sink.pull(4 * testRate)
	if s := lastState(t, p); s.SongID != ids[0] {
		t.Errorf("RepeatAll state = %+v", s)
	}
	if err := p.SetRepeat(ctx, "sometimes"); err == nil {
		t.Error("an unknown repeat mode should fail")
	}
	// RepeatOff stops after the last song.
	_ = p.SetRepeat(ctx, playback.RepeatOff)
	if err := p.Next(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Next(ctx); err != nil {
		t.Fatal(err)
	}
	sink.pull(4*testRate + 1000)
	s := lastState(t, p)
	if s.Status != playback.StatusStopped || s.SongID != ids[2] || s.Position != 0 {
		t.Errorf("end of queue state = %+v", s)
	}
	if tail := sink.pull(500); peak(tail) != 0 {
		t.Error("audio after the end of the queue")
	}
}

func TestNextPreviousAndStop(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	_, _ = p.PlaySongs(ctx, ids, 0)
	if err := p.Next(ctx); err != nil {
		t.Fatal(err)
	}
	if s := lastState(t, p); s.SongID != ids[1] {
		t.Errorf("after Next = %+v", s)
	}
	out := sink.pull(testRate / 2)
	if songAt(out[len(out)-2]) != 1 {
		t.Errorf("after Next renders %v", out[len(out)-2])
	}
	// Near the start, Previous goes back a song.
	_ = p.Previous(ctx)
	if s := lastState(t, p); s.SongID != ids[0] {
		t.Errorf("Previous near the start = %+v", s)
	}
	// Past 3 s it restarts the song.
	sink.pull(3*testRate + 100)
	_ = p.Previous(ctx)
	if s := lastState(t, p); s.SongID != ids[0] || s.Position != 0 {
		t.Errorf("Previous past 3 s = %+v", s)
	}
	if err := p.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if s := lastState(t, p); s.Status != playback.StatusStopped || s.Position != 0 {
		t.Errorf("after Stop = %+v", s)
	}
	out = sink.pull(testRate / 10)
	if out[0] == 0 || peak(out[2*400:]) != 0 {
		t.Error("Stop should fade out, then be silent")
	}
	if err := p.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if s := lastState(t, p); s.Status != playback.StatusPlaying || s.SongID != ids[0] {
		t.Errorf("Resume after Stop = %+v", s)
	}
}

func TestPauseFadesAndHoldsPosition(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	_, _ = p.PlaySongs(ctx, ids, 0)
	sink.pull(testRate)
	_ = p.Pause(ctx)
	paused := lastState(t, p)
	if paused.Status != playback.StatusPaused {
		t.Fatalf("state = %+v", paused)
	}
	out := sink.pull(testRate)
	if out[0] == 0 || peak(out[2*200:]) != 0 {
		t.Error("Pause should fade out, then be silent")
	}
	if got := p.state().Position; got-paused.Position > 20*time.Millisecond {
		t.Errorf("position moved from %v to %v while paused", paused.Position, got)
	}
	_ = p.Resume(ctx)
	if s := lastState(t, p); s.Status != playback.StatusPlaying {
		t.Errorf("state = %+v", s)
	}
	out = sink.pull(testRate / 10)
	if songAt(out[len(out)-2]) != 0 {
		t.Errorf("after Resume renders %v", out[len(out)-2])
	}
}

func TestSeek(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	_, _ = p.PlaySongs(ctx, ids, 1)
	sink.pull(1000)
	if err := p.Seek(ctx, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if s := lastState(t, p); s.Position != 2*time.Second {
		t.Errorf("position after Seek = %v", s.Position)
	}
	out := sink.pull(400)
	// After the crossfade, the audio is the song 2 s in.
	want := levelOf(1) + float64(2*testRate+300)/(4*testRate)*0.1
	if got := float64(out[2*300]); math.Abs(got-want) > 1e-3 {
		t.Errorf("rendered %v after Seek, want %v", got, want)
	}
	if err := p.Seek(ctx, time.Hour); err != nil {
		t.Fatal(err)
	}
	if s := lastState(t, p); s.Position != 4*time.Second {
		t.Errorf("Seek past the end = %v, want clamped to 4s", s.Position)
	}
}

func TestVolumeIsSoftwareGain(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	_, _ = p.PlaySongs(ctx, ids, 0)
	sink.pull(1000)
	if err := p.SetVolume(ctx, 0.5); err != nil {
		t.Fatal(err)
	}
	if v, err := p.Volume(ctx); err != nil || v != 0.5 {
		t.Errorf("Volume = %v, %v", v, err)
	}
	out := sink.pull(1000)
	want := (levelOf(0) + float64(2000-1)/(4*testRate)*0.1) * 0.25
	if got := float64(out[len(out)-2]); math.Abs(got-want) > 1e-3 {
		t.Errorf("rendered %v at volume 0.5, want %v", got, want)
	}
	_ = p.SetVolume(ctx, 7)
	if v, _ := p.Volume(ctx); v != 1 {
		t.Errorf("volume = %v, want clamped to 1", v)
	}
}

func TestLevelsWhilePlaying(t *testing.T) {
	root := t.TempDir()
	writeWAV(t, filepath.Join(root, "Tone", "tone.wav"), 48000, 1, 48000, wavPCM16, sine(1000, 0.5, 48000))
	lib, err := Scan(ctx, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := lib.Playlist(lib.Playlists()[0].ID)
	s := &fakeSink{}
	p := newPlayer(lib, Options{SampleRate: 48000}, s, clock{})
	defer p.Close()
	var _ playback.LevelSource = p
	_, _ = p.PlaySongs(ctx, []string{d.Tracks[0].ID}, 0)
	_ = p.SetVolume(ctx, 0.1) // measured before the gain
	p.measure()
	select {
	case r := <-p.Levels():
		t.Fatalf("reading before a full window: %+v", r)
	default:
	}
	s.pull(4096)
	p.measure()
	var r playback.Spectrum
	select {
	case r = <-p.Levels():
	default:
		t.Fatal("no reading after a full window")
	}
	if len(r.Bands) != numBands || len(r.Wave) != wavePoints {
		t.Fatalf("reading has %d bands, %d wave points", len(r.Bands), len(r.Wave))
	}
	band := bandOf(1000, numBands, minHz, maxHz)
	for i, b := range r.Bands {
		if b > r.Bands[band] {
			t.Errorf("band %d (%v) is louder than the tone's band %d (%v)", i, b, band, r.Bands[band])
		}
	}
	if r.Bands[band] < 0.5 || peak32(r.Wave) < 0.45 {
		t.Errorf("reading = %+v", r)
	}
	_ = p.Pause(ctx)
	s.pull(4096)
	p.measure()
	select {
	case r := <-p.Levels():
		t.Errorf("reading while paused: %+v", r)
	default:
	}
}

func peak32(w []float64) float64 {
	var m float64
	for _, v := range w {
		m = max(m, math.Abs(v))
	}
	return m
}

func TestCloseFadesAndIsIdempotent(t *testing.T) {
	lib, ids := testLibrary(t)
	s := &fakeSink{}
	p := newPlayer(lib, Options{SampleRate: testRate}, s, clock{})
	_, _ = p.PlaySongs(ctx, ids, 0)
	s.pull(1000)
	done := make(chan error)
	go func() { done <- p.Close() }()
	var out []float32
	for closed := false; !closed; {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			closed = true
		default:
			out = append(out, s.pull(64)...)
		}
	}
	if len(out)/2 < testRate/5 {
		t.Fatalf("Close returned after %d frames; the fade takes %d", len(out)/2, testRate/5)
	}
	if peak(out[:20]) < 0.15 || peak(out[len(out)-20:]) != 0 {
		t.Errorf("Close should fade from the music to silence: starts %v, ends %v", peak(out[:20]), peak(out[len(out)-20:]))
	}
	if _, closes := s.counts(); closes != 1 {
		t.Errorf("sink closed %d times", closes)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close = %v", err)
	}
	if _, closes := s.counts(); closes != 1 {
		t.Errorf("sink closed %d times after a second Close", closes)
	}
	for range p.States() {
	}
	if _, ok := <-p.Errors(); ok {
		t.Error("Errors still open")
	}
	if _, ok := <-p.Levels(); ok {
		t.Error("Levels still open")
	}
	if err := p.Pause(ctx); !errors.Is(err, ErrClosed) {
		t.Errorf("Pause after Close = %v, want ErrClosed", err)
	}
}

// TestCloseSignalsFadedOnlyAfterSilence pins the order the idempotence test
// above relies on: Close may return as soon as the fade is signalled, so the
// render that signals it must already be silence, not the fade's last frames
// (which a fast Close would otherwise leave as the final samples rendered).
func TestCloseSignalsFadedOnlyAfterSilence(t *testing.T) {
	lib, ids := testLibrary(t)
	s := &fakeSink{}
	p := newPlayer(lib, Options{SampleRate: testRate}, s, clock{})
	_, _ = p.PlaySongs(ctx, ids, 0)
	s.pull(1000)
	done := make(chan error)
	go func() { done <- p.Close() }()
	for closing := false; !closing; {
		p.mu.Lock()
		closing = p.closing
		p.mu.Unlock()
	}
	for frames := 0; ; frames += 64 {
		chunk := s.pull(64)
		select {
		case <-p.faded:
			if got := peak(chunk); got != 0 {
				t.Errorf("the fade was signalled by a render holding audio (peak %v)", got)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
		default:
		}
		if frames > testRate {
			t.Fatal("the fade was never signalled")
		}
	}
}

func TestCloseWithoutAPullingDeviceStillReturns(t *testing.T) {
	lib, ids := testLibrary(t)
	p, _ := newTestPlayer(t, lib)
	_, _ = p.PlaySongs(ctx, ids, 0)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryMethodsAndUnsupported(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	pls, err := p.Playlists(ctx)
	if err != nil || len(pls) != 1 || pls[0].Name != "Songs" || pls[0].Editable {
		t.Fatalf("Playlists = %+v, %v", pls, err)
	}
	d, err := p.LibraryPlaylist(ctx, pls[0].ID)
	if err != nil || len(d.Tracks) != 3 || d.Playlist.Name != "Songs" {
		t.Fatalf("LibraryPlaylist = %+v, %v", d, err)
	}
	if _, err := p.LibraryPlaylist(ctx, "local:pl:nope"); err == nil {
		t.Error("unknown playlist should fail")
	}
	if err := p.PlayPlaylistFrom(ctx, pls[0].ID, 2); err != nil {
		t.Fatal(err)
	}
	if s := lastState(t, p); s.SongID != ids[2] {
		t.Errorf("PlayPlaylistFrom state = %+v", s)
	}
	if err := p.PlayPlaylist(ctx, pls[0].ID); err != nil {
		t.Fatal(err)
	}
	if s := lastState(t, p); s.SongID != ids[0] {
		t.Errorf("PlayPlaylist state = %+v", s)
	}
	if starts, _ := sink.counts(); starts != 1 {
		t.Errorf("sink started %d times, want once", starts)
	}
	unsupported := map[string]error{}
	_, unsupported["SearchCatalog"] = p.SearchCatalog(ctx, "x", 5)
	_, unsupported["Artist"] = p.Artist(ctx, "a")
	_, unsupported["Album"] = p.Album(ctx, "a")
	_, unsupported["SongAlbum"] = p.SongAlbum(ctx, ids[0])
	_, unsupported["CatalogPlaylist"] = p.CatalogPlaylist(ctx, "p")
	_, unsupported["CreatePlaylist"] = p.CreatePlaylist(ctx, "n", "", nil)
	unsupported["AddToPlaylist"] = p.AddToPlaylist(ctx, pls[0].ID, ids)
	_, unsupported["Favorite"] = p.Favorite(ctx, ids[0])
	_, unsupported["Favorites"] = p.Favorites(ctx, ids)
	unsupported["SetFavorite"] = p.SetFavorite(ctx, ids[0], true)
	for name, err := range unsupported {
		if !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s = %v, want ErrUnsupported", name, err)
		}
	}
}

func TestUnplayableSongsAreReportedAndSkipped(t *testing.T) {
	lib, ids := testLibrary(t)
	p, sink := newTestPlayer(t, lib)
	if err := os.Remove(lib.path(ids[1])); err != nil {
		t.Fatal(err)
	}
	if _, err := p.PlaySongs(ctx, ids, 1); err == nil {
		t.Error("starting at a deleted file should fail")
	}
	_, _ = p.PlaySongs(ctx, ids, 0)
	sink.pull(4*testRate + 1000)
	if s := lastState(t, p); s.SongID != ids[2] {
		t.Errorf("after a missing song: %+v, want the one after it", s)
	}
	select {
	case err := <-p.Errors():
		if err == nil {
			t.Error("nil error")
		}
	default:
		t.Error("the skipped song was not reported")
	}
}

func TestSinkFailureIsReturned(t *testing.T) {
	lib, ids := testLibrary(t)
	s := &fakeSink{failure: errors.New("no device")}
	p := newPlayer(lib, Options{SampleRate: testRate}, s, clock{})
	defer p.Close()
	if _, err := p.PlaySongs(ctx, ids, 0); err == nil {
		t.Error("PlaySongs without an output device should fail")
	}
}
