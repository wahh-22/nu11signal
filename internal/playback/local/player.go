package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// ErrUnsupported is returned by the Player methods local files have no
// answer for: the catalog (search, artist, album and catalog playlist
// pages), playlist editing and favorites.
var ErrUnsupported = errors.New("local: not available for local files")

// ErrClosed is returned by calls made after Close.
var ErrClosed = errors.New("local: player closed")

// Options configures New.
type Options struct {
	// SampleRate is the output rate in Hz (default 48000); every file is
	// resampled to it.
	SampleRate int
}

const (
	defaultRate = 48000
	// stateInterval is how often States reports the position while playing.
	stateInterval = 500 * time.Millisecond
	// levelInterval is the levels' reading rate, as the helper's (15 Hz).
	levelInterval = time.Second / 15
	// restartAfter is how far into a song Previous restarts it rather
	// than going back a song.
	restartAfter = 3 * time.Second
	// closeBackstop bounds how long Close waits for its fade to be
	// rendered, should the device stop pulling.
	closeBackstop = 500 * time.Millisecond
	stateBuffer   = 16
	errorBuffer   = 8
)

// sink is the audio output. start begins pulling interleaved stereo
// float32 frames at rate from src, on its own goroutine (never from within
// start); close stops it. src always fills its whole buffer.
type sink interface {
	start(rate int, src func(dst []float32)) error
	close() error
}

// clock holds what the player waits on, so tests can drive it: the state
// and level tickers (nil never fires) and the Close backstop (nil is
// time.After).
type clock struct {
	after     func(time.Duration) <-chan time.Time
	stateTick <-chan time.Time
	levelTick <-chan time.Time
}

// track is an open song: its decoder at the output rate and how far it
// has played.
type track struct {
	dec    decoder
	rs     *resampler
	frames int64         // output frames from the song's start
	length time.Duration // 0 when the decoder does not know
}

func (t *track) close() {
	if t != nil {
		_ = t.dec.close()
	}
}

// Player is a playback.Player and playback.LevelSource for a Library.
// Methods are safe for concurrent use.
//
// The device pulls audio through render: the current song, decoded and
// resampled to the output rate, times a fade (a 15 ms ramp that keeps
// pauses, stops and switches free of clicks; a song left by a switch fades
// out underneath the new one), is tapped for the levels, then scaled by the
// app volume (square law, also ramped). Close fades out over 200 ms, as
// the helper's quiet exit, before closing the device.
type Player struct {
	lib       *Library
	rate      int
	sink      sink
	clk       clock
	stopTicks func()
	done      chan struct{}
	wg        sync.WaitGroup

	// sinkMu orders starting the sink against closing it.
	sinkMu     sync.Mutex
	sinkOn     atomic.Bool
	sinkClosed bool

	closeOnce sync.Once
	closeErr  error

	mu        sync.Mutex
	closing   bool // Close began: calls fail, the fade-out renders
	closed    bool // channels closed: nothing renders or is sent
	states    chan playback.State
	errs      chan error
	levels    chan playback.Spectrum
	queue     []string
	index     int
	status    playback.Status
	repeat    playback.RepeatMode
	level     float64
	step      float32 // the ramp step per frame
	cur, tail *track
	fade      ramp // the current song's
	tailFade  ramp
	gain      ramp // the app volume
	faded     chan struct{}
	fadedSent bool
	ring      *ring
	fresh     bool // playback (re)started: the smoother starts over
	scratch   []float32

	measureMu sync.Mutex
	an        *analyzer
	sm        *smoother
}

// New makes a Player for the library, playing on the default output
// device. The device is opened on the first play, so a machine without
// one can still browse the library; that play then fails.
func New(lib *Library, opts Options) *Player {
	st, lt := time.NewTicker(stateInterval), time.NewTicker(levelInterval)
	p := newPlayer(lib, opts, &otoSink{}, clock{stateTick: st.C, levelTick: lt.C})
	p.stopTicks = func() { st.Stop(); lt.Stop() }
	return p
}

func newPlayer(lib *Library, opts Options, s sink, clk clock) *Player {
	rate := opts.SampleRate
	if rate <= 0 {
		rate = defaultRate
	}
	step := rampStep(rate, rampSeconds)
	p := &Player{
		lib: lib, rate: rate, sink: s, clk: clk, done: make(chan struct{}),
		states: make(chan playback.State, stateBuffer),
		errs:   make(chan error, errorBuffer),
		levels: make(chan playback.Spectrum, 1),
		status: playback.StatusStopped,
		repeat: playback.RepeatOff,
		level:  1,
		step:   step,
		fade:   ramp{step: step},
		gain:   ramp{cur: 1, target: 1, step: step},
		faded:  make(chan struct{}),
		ring:   newRing(2 * fftSize),
		an:     newAnalyzer(rate),
		sm:     newSmoother(numBands),
	}
	p.wg.Add(1)
	go p.run()
	return p
}

func (p *Player) run() {
	defer p.wg.Done()
	for {
		select {
		case <-p.done:
			return
		case <-p.clk.stateTick:
			p.mu.Lock()
			if p.status == playback.StatusPlaying && !p.closing {
				p.emitLocked()
			}
			p.mu.Unlock()
		case <-p.clk.levelTick:
			p.measure()
		}
	}
}

// render fills dst with the next stereo frames; the sink calls it.
func (p *Player) render(dst []float32) {
	clear(dst)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if p.cur != nil && (p.status == playback.StatusPlaying && !p.closing || p.fade.cur > 0) {
		n := p.fillLocked(dst)
		for i := range n {
			f := p.fade.next()
			dst[2*i] *= f
			dst[2*i+1] *= f
		}
	}
	if p.tail != nil {
		p.mixTailLocked(dst)
	}
	if p.status == playback.StatusPlaying && !p.closing {
		p.ring.writeStereo(dst) // before the volume, as the helper measures
	}
	for i := 0; i < len(dst); i += 2 {
		g := p.gain.next()
		dst[i] *= g
		dst[i+1] *= g
	}
	if p.closing && p.fade.cur == 0 && p.tail == nil && !p.fadedSent {
		p.fadedSent = true
		close(p.faded)
	}
}

// fillLocked renders the current song into dst, before its fade, and
// returns the frames rendered: all of them while playing (running into the
// next song at the end of one), only the rest of the fade when fading out,
// so a pause does not decode past what is heard.
func (p *Player) fillLocked(dst []float32) int {
	want := len(dst) / 2
	if p.fade.target == 0 {
		want = min(want, int(math.Ceil(float64(p.fade.cur/p.fade.step))))
	}
	got, failures := 0, 0
	for got < want && p.cur != nil {
		n, err := p.cur.rs.read(dst[2*got : 2*want])
		got += n
		p.cur.frames += int64(n)
		if err == nil {
			continue
		}
		if !errors.Is(err, io.EOF) {
			p.reportLocked(err)
		}
		if n == 0 {
			failures++
		} else {
			failures = 0
		}
		if failures > len(p.queue) {
			p.stopLocked() // every song ends at once: nothing to play
			p.emitLocked()
			break
		}
		p.endOfSongLocked()
		p.emitLocked()
		if p.status != playback.StatusPlaying {
			break
		}
	}
	return got
}

// endOfSongLocked moves on when the current song ends, by the repeat
// mode: RepeatOne plays it again, RepeatAll wraps around at the end of the
// queue and RepeatOff stops there. A song that cannot be opened is
// reported on Errors and skipped.
func (p *Player) endOfSongLocked() {
	p.cur.close()
	p.cur = nil
	last := len(p.queue) - 1
	if p.repeat != playback.RepeatOne {
		if p.index == last && p.repeat == playback.RepeatOff {
			p.stopLocked()
			return
		}
		p.index = (p.index + 1) % len(p.queue)
	}
	for range p.queue {
		t, err := p.open(p.queue[p.index])
		if err == nil {
			p.cur = t // straight on: gapless, no fade
			return
		}
		p.reportLocked(err)
		if p.index == last && p.repeat == playback.RepeatOff {
			break
		}
		p.index = (p.index + 1) % len(p.queue)
	}
	p.stopLocked()
}

// mixTailLocked adds the rest of the fade of a song left by a switch.
func (p *Player) mixTailLocked(dst []float32) {
	m := min(len(dst)/2, int(math.Ceil(float64(p.tailFade.cur/p.tailFade.step))))
	if cap(p.scratch) < 2*m {
		p.scratch = make([]float32, 2*m)
	}
	buf, got := p.scratch[:2*m], 0
	for got < m {
		n, err := p.tail.rs.read(buf[2*got:])
		got += n
		if err != nil {
			break
		}
	}
	for i := range got {
		f := p.tailFade.next()
		dst[2*i] += buf[2*i] * f
		dst[2*i+1] += buf[2*i+1] * f
	}
	if got < m || p.tailFade.done() {
		p.tail.close()
		p.tail = nil
	}
}

// open opens the song with the id at the output rate.
func (p *Player) open(id string) (*track, error) {
	path := p.lib.path(id)
	if path == "" {
		return nil, fmt.Errorf("local: unknown song %q", id)
	}
	dec, err := openDecoder(path)
	if err != nil {
		return nil, err
	}
	return &track{dec: dec, rs: newResampler(dec, p.rate), length: framesToDuration(dec.length(), dec.rate())}, nil
}

// seekTrack moves t to pos: by the decoder's seek, or by reopening and
// decoding up to pos when that fails.
func (p *Player) seekTrack(id string, t *track, pos time.Duration) (*track, error) {
	frame := int64(pos) * int64(t.dec.rate()) / int64(time.Second)
	if err := t.dec.seek(frame); err != nil {
		t.close()
		if t, err = p.open(id); err != nil {
			return nil, err
		}
		buf := make([]float32, 2*4096)
		for left := frame; left > 0; {
			n, err := t.dec.read(buf[:2*min(left, 4096)])
			left -= int64(n)
			if err != nil {
				break
			}
		}
	}
	t.rs.reset()
	t.frames = int64(pos) * int64(p.rate) / int64(time.Second)
	return t, nil
}

// switchLocked makes t (nil for none) the current song. An audible song
// it replaces fades out underneath; the new one fades in when playing.
func (p *Player) switchLocked(t *track) {
	if p.cur != nil {
		if p.fade.cur > 0 && p.sinkOn.Load() {
			p.tail.close()
			p.tail, p.tailFade = p.cur, ramp{cur: p.fade.cur, step: p.step}
		} else {
			p.cur.close()
		}
	}
	p.cur = t
	p.fade = ramp{step: p.step}
	p.fadeToStatusLocked()
}

// fadeToStatusLocked aims the fade at the status: in when playing, out
// otherwise.
func (p *Player) fadeToStatusLocked() {
	p.fade.target = 0
	if p.status == playback.StatusPlaying {
		p.fade.target = 1
	}
}

// stopLocked stops and rewinds, keeping the queue and its position.
func (p *Player) stopLocked() {
	p.status = playback.StatusStopped
	p.switchLocked(nil)
}

// startedLocked marks playback as (re)started for the levels.
func (p *Player) startedLocked() {
	p.ring.reset()
	p.fresh = true
}

// emitLocked publishes the current state, dropping the oldest unread one
// when the buffer is full so the newest always gets through.
func (p *Player) emitLocked() {
	if p.closed {
		return
	}
	s := p.stateLocked()
	select {
	case p.states <- s:
		return
	default:
	}
	select {
	case <-p.states:
	default:
	}
	p.states <- s
}

func (p *Player) reportLocked(err error) {
	if p.closed {
		return
	}
	select {
	case p.errs <- err:
	default: // the oldest errors are enough
	}
}

func (p *Player) stateLocked() playback.State {
	s := playback.State{Status: p.status, Repeat: p.repeat, VolumeMode: playback.VolumeApp}
	if len(p.queue) == 0 {
		return s
	}
	song, _ := p.lib.Song(p.queue[p.index])
	s.SongID, s.Title, s.Artist, s.Album, s.Duration = song.ID, song.Title, song.Artist, song.Album, song.Duration
	if p.cur != nil {
		if p.cur.length > 0 {
			s.Duration = p.cur.length
		}
		s.Position = framesToDuration(p.cur.frames, p.rate)
	}
	return s
}

func (p *Player) state() playback.State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stateLocked()
}

// do runs fn under the lock after checking ctx and Close, and emits the
// state when fn succeeds and emit is set.
func (p *Player) do(ctx context.Context, emit bool, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing {
		return ErrClosed
	}
	if err := fn(); err != nil {
		return err
	}
	if emit {
		p.emitLocked()
	}
	return nil
}

// ensureSink starts the output on the first play.
func (p *Player) ensureSink() error {
	p.sinkMu.Lock()
	defer p.sinkMu.Unlock()
	if p.sinkClosed {
		return ErrClosed
	}
	if p.sinkOn.Load() {
		return nil
	}
	if err := p.sink.start(p.rate, p.render); err != nil {
		return err
	}
	p.sinkOn.Store(true)
	return nil
}

// Authorize always succeeds: local files need no permission.
func (p *Player) Authorize(ctx context.Context) (playback.AuthStatus, error) {
	if err := p.do(ctx, false, func() error { return nil }); err != nil {
		return "", err
	}
	return playback.AuthAuthorized, nil
}

// Playlists lists the library's playlists, alphabetically.
func (p *Player) Playlists(ctx context.Context) ([]playback.Playlist, error) {
	var out []playback.Playlist
	err := p.do(ctx, false, func() error { out = p.lib.Playlists(); return nil })
	return out, err
}

// LibraryPlaylist loads a playlist's songs, in order.
func (p *Player) LibraryPlaylist(ctx context.Context, playlistID string) (playback.PlaylistDetail, error) {
	var d playback.PlaylistDetail
	err := p.do(ctx, false, func() error {
		var ok bool
		if d, ok = p.lib.Playlist(playlistID); !ok {
			return fmt.Errorf("local: unknown playlist %q", playlistID)
		}
		return nil
	})
	return d, err
}

// PlaySongs queues the library songs and plays from ids[start]. Ids the
// library does not have are left out and reported as Missing; when the
// start song is one of them, play starts at the next song that is not (or
// the last one before it). The start song must open.
func (p *Player) PlaySongs(ctx context.Context, ids []string, start int) (playback.QueueReport, error) {
	var rep playback.QueueReport
	if start < 0 || start >= len(ids) {
		return rep, fmt.Errorf("local: start %d out of range", start)
	}
	queue, at := make([]string, 0, len(ids)), -1
	for i, id := range ids {
		if _, ok := p.lib.Song(id); !ok {
			rep.Missing = append(rep.Missing, id)
			continue
		}
		if i >= start && at < 0 {
			at = len(queue)
		}
		queue = append(queue, id)
	}
	if len(queue) == 0 {
		return rep, errors.New("local: none of the songs is in the library")
	}
	if at < 0 {
		at = len(queue) - 1
	}
	if err := p.ensureSink(); err != nil {
		return rep, err
	}
	return rep, p.do(ctx, true, func() error {
		t, err := p.open(queue[at])
		if err != nil {
			return err
		}
		p.queue, p.index, p.status = queue, at, playback.StatusPlaying
		p.switchLocked(t)
		p.startedLocked()
		return nil
	})
}

// PlayPlaylist plays a playlist from its first song.
func (p *Player) PlayPlaylist(ctx context.Context, id string) error {
	return p.PlayPlaylistFrom(ctx, id, 0)
}

// PlayPlaylistFrom plays a playlist from the song at index start.
func (p *Player) PlayPlaylistFrom(ctx context.Context, playlistID string, start int) error {
	d, ok := p.lib.Playlist(playlistID)
	if !ok {
		return fmt.Errorf("local: unknown playlist %q", playlistID)
	}
	ids := make([]string, len(d.Tracks))
	for i, s := range d.Tracks {
		ids[i] = s.ID
	}
	_, err := p.PlaySongs(ctx, ids, start)
	return err
}

// Pause fades out and holds the position; it does nothing unless playing.
func (p *Player) Pause(ctx context.Context) error {
	return p.do(ctx, true, func() error {
		if p.status == playback.StatusPlaying {
			p.status = playback.StatusPaused
			p.fadeToStatusLocked()
		}
		return nil
	})
}

// Resume plays the current song from where it paused or stopped.
func (p *Player) Resume(ctx context.Context) error {
	if err := p.ensureSink(); err != nil {
		return err
	}
	return p.do(ctx, true, func() error {
		if len(p.queue) == 0 {
			return errors.New("local: nothing queued")
		}
		if p.status == playback.StatusPlaying {
			return nil
		}
		if p.cur == nil {
			t, err := p.open(p.queue[p.index])
			if err != nil {
				return err
			}
			p.cur, p.fade = t, ramp{step: p.step}
		}
		p.status = playback.StatusPlaying
		p.fadeToStatusLocked()
		p.startedLocked()
		return nil
	})
}

// loadLocked makes the song at index current, from its start; stopped, it
// only moves there.
func (p *Player) loadLocked(index int) error {
	if p.status == playback.StatusStopped {
		p.index = index
		p.switchLocked(nil)
		return nil
	}
	t, err := p.open(p.queue[index])
	if err != nil {
		return err
	}
	p.index = index
	p.switchLocked(t)
	return nil
}

// Next skips to the next song; after the last one, RepeatOff stops and
// the other modes start the queue over.
func (p *Player) Next(ctx context.Context) error {
	return p.do(ctx, true, func() error {
		if len(p.queue) == 0 {
			return errors.New("local: nothing queued")
		}
		if p.index == len(p.queue)-1 && p.repeat == playback.RepeatOff {
			p.stopLocked()
			return nil
		}
		return p.loadLocked((p.index + 1) % len(p.queue))
	})
}

// Previous restarts the song once restartAfter into it, and goes back one
// (wrapping around) before that.
func (p *Player) Previous(ctx context.Context) error {
	return p.do(ctx, true, func() error {
		if len(p.queue) == 0 {
			return errors.New("local: nothing queued")
		}
		if p.stateLocked().Position >= restartAfter {
			return p.loadLocked(p.index)
		}
		return p.loadLocked((p.index - 1 + len(p.queue)) % len(p.queue))
	})
}

// Stop fades out and rewinds; Resume plays the song from its start.
func (p *Player) Stop(ctx context.Context) error {
	return p.do(ctx, true, func() error {
		p.stopLocked()
		return nil
	})
}

// Seek moves within the current song, clamped to its length; stopped, the
// song then plays from there on Resume.
func (p *Player) Seek(ctx context.Context, position time.Duration) error {
	return p.do(ctx, true, func() error {
		if len(p.queue) == 0 {
			return errors.New("local: nothing queued")
		}
		id := p.queue[p.index]
		t, err := p.open(id)
		if err != nil {
			return err
		}
		length := t.length
		if length <= 0 {
			length = p.stateLocked().Duration
		}
		position = max(position, 0)
		if length > 0 {
			position = min(position, length)
		}
		if t, err = p.seekTrack(id, t, position); err != nil {
			return err
		}
		p.switchLocked(t)
		return nil
	})
}

// SetRepeat sets the repeat mode; an unknown one is an error.
func (p *Player) SetRepeat(ctx context.Context, mode playback.RepeatMode) error {
	return p.do(ctx, true, func() error {
		switch mode {
		case playback.RepeatOff, playback.RepeatAll, playback.RepeatOne:
			p.repeat = mode
			return nil
		}
		return fmt.Errorf("local: unknown repeat mode %q", mode)
	})
}

// Volume reports the app volume, 1 until SetVolume changes it.
func (p *Player) Volume(ctx context.Context) (float64, error) {
	var v float64
	err := p.do(ctx, false, func() error { v = p.level; return nil })
	return v, err
}

// SetVolume sets the app volume, clamped with playback.ClampVolume; the
// gain follows over a 15 ms ramp.
func (p *Player) SetVolume(ctx context.Context, level float64) error {
	return p.do(ctx, false, func() error {
		p.level = playback.ClampVolume(level)
		p.gain.target = amplitude(p.level)
		return nil
	})
}

// SearchCatalog is unsupported: there is no catalog.
func (p *Player) SearchCatalog(context.Context, string, int) (playback.SearchResults, error) {
	return playback.SearchResults{}, ErrUnsupported
}

// Artist is unsupported: there is no catalog.
func (p *Player) Artist(context.Context, string) (playback.ArtistDetail, error) {
	return playback.ArtistDetail{}, ErrUnsupported
}

// Album is unsupported: there is no catalog.
func (p *Player) Album(context.Context, string) (playback.AlbumDetail, error) {
	return playback.AlbumDetail{}, ErrUnsupported
}

// SongAlbum is unsupported: there is no catalog.
func (p *Player) SongAlbum(context.Context, string) (playback.AlbumDetail, error) {
	return playback.AlbumDetail{}, ErrUnsupported
}

// CatalogPlaylist is unsupported: there is no catalog.
func (p *Player) CatalogPlaylist(context.Context, string) (playback.PlaylistDetail, error) {
	return playback.PlaylistDetail{}, ErrUnsupported
}

// CreatePlaylist is unsupported: the library is read-only.
func (p *Player) CreatePlaylist(context.Context, string, string, []string) (playback.Playlist, error) {
	return playback.Playlist{}, ErrUnsupported
}

// AddToPlaylist is unsupported: the library is read-only.
func (p *Player) AddToPlaylist(context.Context, string, []string) error { return ErrUnsupported }

// Favorite is unsupported: local files have no favorites.
func (p *Player) Favorite(context.Context, string) (bool, error) { return false, ErrUnsupported }

// Favorites is unsupported, but no ids answer an empty map, as the port
// requires.
func (p *Player) Favorites(_ context.Context, songIDs []string) (map[string]bool, error) {
	if len(songIDs) == 0 {
		return map[string]bool{}, nil
	}
	return nil, ErrUnsupported
}

// SetFavorite is unsupported: local files have no favorites.
func (p *Player) SetFavorite(context.Context, string, bool) error { return ErrUnsupported }

// States delivers state snapshots: on every change, at the end of each
// song, and every 500 ms while playing. Closed by Close.
func (p *Player) States() <-chan playback.State { return p.states }

// Errors delivers asynchronous failures, such as a queued song that would
// not open (it is skipped). Closed by Close.
func (p *Player) Errors() <-chan error { return p.errs }

// Levels delivers the spectrum and waveform of what plays, 15 times a
// second while playing, keeping only the latest (see playback.LevelSource).
// As the helper's, they are measured before the app volume, so the bars do
// not move with it. Closed by Close.
func (p *Player) Levels() <-chan playback.Spectrum { return p.levels }

// measure takes one reading of the newest fftSize samples played, as the
// helper's LevelMeter: nothing while not playing, or until a full window
// has played since playback (re)started.
func (p *Player) measure() {
	p.mu.Lock()
	if p.closing || p.status != playback.StatusPlaying {
		p.mu.Unlock()
		return
	}
	window, ok := p.ring.latest(fftSize)
	fresh := ok && p.fresh
	if ok {
		p.fresh = false
	}
	p.mu.Unlock()
	if !ok {
		return
	}
	p.measureMu.Lock()
	if fresh {
		p.sm.reset()
	}
	bands := p.sm.update(p.an.levels(window))
	p.measureMu.Unlock()
	reading := playback.Spectrum{Bands: quantize(bands), Wave: quantize(decimate(window, wavePoints))}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	select {
	case <-p.levels:
	default:
	}
	p.levels <- reading
}

// Close fades the music out (200 ms), closes the device and the channels.
// It is idempotent; later calls return the first one's result.
func (p *Player) Close() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closing = true
		p.fade.target, p.fade.step = 0, rampStep(p.rate, closeFadeSeconds)
		audible := p.sinkOn.Load() && (p.fade.cur > 0 || p.tail != nil)
		p.mu.Unlock()
		if audible {
			after := p.clk.after
			if after == nil {
				after = time.After
			}
			select {
			case <-p.faded:
			case <-after(closeBackstop):
			}
		}
		close(p.done)
		p.wg.Wait()
		if p.stopTicks != nil {
			p.stopTicks()
		}
		p.sinkMu.Lock()
		if p.sinkOn.Load() {
			p.closeErr = p.sink.close()
		}
		p.sinkClosed = true
		p.sinkMu.Unlock()
		p.mu.Lock()
		defer p.mu.Unlock()
		p.closed = true
		p.cur.close()
		p.tail.close()
		p.cur, p.tail = nil, nil
		close(p.states)
		close(p.errs)
		close(p.levels)
	})
	return p.closeErr
}

var (
	_ playback.Player      = (*Player)(nil)
	_ playback.LevelSource = (*Player)(nil)
)
