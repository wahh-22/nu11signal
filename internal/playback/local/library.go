// Package local is a playback.Player for the computer's own music files:
// a Library scanned from folders (mp3, flac, ogg vorbis and wav, tagged or
// not), decoded in Go into one PCM pipeline and played through the system
// output with ebitengine/oto. Only the output (oto.go) touches a device;
// scanning, decoding, resampling, gain, the queue and the levels are pure
// Go and are tested against a fake sink.
package local

import (
	"bufio"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/dhowden/tag"

	"github.com/wahh-22/nu11signal/internal/playback"
)

// Id prefixes: every id this package hands out starts with songPrefix, so
// a router can tell local ids from Apple Music ones; playlists add "pl:".
const (
	songPrefix     = playback.LocalPrefix
	playlistPrefix = playback.LocalPrefix + "pl:"
)

// audioExts are the extensions Scan picks up, lower case.
var audioExts = map[string]bool{".mp3": true, ".flac": true, ".ogg": true, ".oga": true, ".wav": true}

// Library is a scanned music collection: its songs by id and its playlists,
// one per folder that directly holds audio and one per .m3u/.m3u8 file. It
// is immutable once Scan returns, so it is safe for concurrent use.
type Library struct {
	songs     map[string]entry
	playlists []list
}

// entry is a song and where it lives.
type entry struct {
	song  playback.Song
	path  string
	disc  int
	track int
}

// list is a playlist and its song ids, in order.
type list struct {
	playlist playback.Playlist
	songs    []string
}

// songID is the id of the song at path: songPrefix and a hash of the clean
// absolute path, stable across scans and runs.
func songID(path string) string { return songPrefix + pathHash(path) }

// playlistID is the id of the folder or m3u file at path.
func playlistID(path string) string { return playlistPrefix + pathHash(path) }

func pathHash(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:8])
}

// Scan walks the roots for audio files and playlists. Hidden files and
// folders (a leading dot) are skipped; symlinks are followed, each real
// folder once, so a link loop ends. A root that does not exist or cannot
// be read is skipped, as are unreadable files; only a cancelled ctx fails
// the scan. Tags are read with dhowden/tag: a song without a title is
// named after its file, without an album after its folder. Run it off the
// UI path: it reads every file's tags.
func Scan(ctx context.Context, roots []string) (*Library, error) {
	s := scanner{ctx: ctx, lib: &Library{songs: map[string]entry{}}, seen: map[string]bool{}, folders: map[string][]string{}}
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if err := s.walk(abs); err != nil {
			return nil, err
		}
	}
	for _, m3u := range s.m3us {
		if err := s.readM3U(m3u); err != nil {
			return nil, err
		}
	}
	for dir, ids := range s.folders {
		slices.SortFunc(ids, s.lib.trackOrder)
		s.lib.playlists = append(s.lib.playlists, list{playback.Playlist{ID: playlistID(dir), Name: filepath.Base(dir), Source: playback.SourceLocal}, ids})
	}
	slices.SortFunc(s.lib.playlists, func(a, b list) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.playlist.Name), strings.ToLower(b.playlist.Name)),
			cmp.Compare(a.playlist.Name, b.playlist.Name), cmp.Compare(a.playlist.ID, b.playlist.ID))
	})
	return s.lib, nil
}

type scanner struct {
	ctx context.Context
	lib *Library
	// seen holds the real paths of the folders walked.
	seen    map[string]bool
	folders map[string][]string
	m3us    []string
}

// walk scans dir, a folder reached by its (possibly symlinked) path.
func (s *scanner) walk(dir string) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil || s.seen[real] {
		return nil
	}
	s.seen[real] = true
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, it := range items {
		name := it.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		mode := it.Type()
		if mode&fs.ModeSymlink != 0 {
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			mode = info.Mode().Type()
		}
		switch ext := strings.ToLower(filepath.Ext(name)); {
		case mode.IsDir():
			if err := s.walk(path); err != nil {
				return err
			}
		case !mode.IsRegular():
		case audioExts[ext]:
			if id, ok := s.add(path); ok {
				s.folders[dir] = append(s.folders[dir], id)
			}
		case ext == ".m3u" || ext == ".m3u8":
			s.m3us = append(s.m3us, path)
		}
	}
	return nil
}

// add reads the song at path into the library once; false if it cannot
// be read.
func (s *scanner) add(path string) (string, bool) {
	id := songID(path)
	if _, ok := s.lib.songs[id]; ok {
		return id, true
	}
	e, err := readEntry(path)
	if err != nil {
		return "", false
	}
	e.song.ID = id
	s.lib.songs[id] = e
	return id, true
}

// readM3U adds the playlist at path: its entries are file paths, absolute
// or relative to its folder (or file:// URLs); comments (#) and entries
// that are missing or not audio are left out.
func (s *scanner) readM3U(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var ids []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if u, err := url.Parse(line); err == nil && u.Scheme == "file" {
			line = u.Path
		}
		line = filepath.FromSlash(line)
		if !filepath.IsAbs(line) {
			line = filepath.Join(filepath.Dir(path), line)
		}
		if !audioExts[strings.ToLower(filepath.Ext(line))] {
			continue
		}
		if info, err := os.Stat(line); err != nil || !info.Mode().IsRegular() {
			continue
		}
		if id, ok := s.add(filepath.Clean(line)); ok {
			ids = append(ids, id)
		}
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	s.lib.playlists = append(s.lib.playlists, list{playback.Playlist{ID: playlistID(path), Name: name, Source: playback.SourceLocal}, ids})
	return nil
}

// readEntry reads a song's tags and duration; tags are optional, but the
// file must open.
func readEntry(path string) (entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return entry{}, err
	}
	defer f.Close()
	e := entry{path: path}
	if m, err := tag.ReadFrom(f); err == nil {
		e.song.Title, e.song.Artist, e.song.Album = strings.TrimSpace(m.Title()), strings.TrimSpace(m.Artist()), strings.TrimSpace(m.Album())
		if e.song.Artist == "" {
			e.song.Artist = strings.TrimSpace(m.AlbumArtist())
		}
		e.track, _ = m.Track()
		e.disc, _ = m.Disc()
	}
	if e.song.Title == "" {
		e.song.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if e.song.Album == "" {
		e.song.Album = filepath.Base(filepath.Dir(path))
	}
	e.song.Duration = probeDuration(path)
	return e, nil
}

// probeDuration is a song's length without decoding it: from the header
// for wav, flac and ogg, estimated from the frame headers for mp3; 0 when
// unknown.
func probeDuration(path string) time.Duration {
	if strings.ToLower(filepath.Ext(path)) == ".mp3" {
		d, _ := mp3Duration(path)
		return d
	}
	dec, err := openDecoder(path)
	if err != nil {
		return 0
	}
	defer dec.close()
	return framesToDuration(dec.length(), dec.rate())
}

func framesToDuration(frames int64, rate int) time.Duration {
	if rate <= 0 || frames <= 0 {
		return 0
	}
	return time.Duration(frames * int64(time.Second) / int64(rate))
}

// trackOrder sorts a folder's songs by disc, then track (numbered tracks
// before unnumbered ones), then file name.
func (l *Library) trackOrder(a, b string) int {
	ea, eb := l.songs[a], l.songs[b]
	num := func(n int) int {
		if n <= 0 {
			return int(^uint(0) >> 1)
		}
		return n
	}
	return cmp.Or(cmp.Compare(ea.disc, eb.disc), cmp.Compare(num(ea.track), num(eb.track)),
		cmp.Compare(filepath.Base(ea.path), filepath.Base(eb.path)), cmp.Compare(ea.path, eb.path))
}

// Playlists lists the playlists alphabetically (case-insensitively); none
// is editable.
func (l *Library) Playlists() []playback.Playlist {
	out := make([]playback.Playlist, len(l.playlists))
	for i, p := range l.playlists {
		out[i] = p.playlist
	}
	return out
}

// Playlist is a playlist's page: its songs in order.
func (l *Library) Playlist(id string) (playback.PlaylistDetail, bool) {
	for _, p := range l.playlists {
		if p.playlist.ID != id {
			continue
		}
		d := playback.PlaylistDetail{Playlist: playback.CatalogPlaylist{ID: id, Name: p.playlist.Name}}
		for _, sid := range p.songs {
			d.Tracks = append(d.Tracks, l.songs[sid].song)
		}
		return d, true
	}
	return playback.PlaylistDetail{}, false
}

// Song is the song with the id.
func (l *Library) Song(id string) (playback.Song, bool) {
	e, ok := l.songs[id]
	return e.song, ok
}

// path is the file of the song with the id; empty when unknown.
func (l *Library) path(id string) string { return l.songs[id].path }
