package helper

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

// fakeEnv selects a fake helper scenario. When it is set, the test binary
// re-executes itself as the helper process instead of running tests.
const fakeEnv = "NU11SIGNAL_FAKE_HELPER"

func TestMain(m *testing.M) {
	if scenario := os.Getenv(fakeEnv); scenario != "" {
		os.Exit(runFakeHelper(scenario))
	}
	os.Exit(m.Run())
}

// fakeHelperEnv is the environment for a fake helper process. The race
// runtime otherwise sleeps a second before every exit.
func fakeHelperEnv(scenario string) []string {
	return append(os.Environ(), fakeEnv+"="+scenario, "GORACE=atexit_sleep_ms=0")
}

// startFake starts the test binary as a helper running the given scenario.
func startFake(t *testing.T, scenario string, opts Options) *Client {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	opts.Path = exe
	opts.Env = fakeHelperEnv(scenario)
	if opts.ReadyTimeout == 0 {
		opts.ReadyTimeout = 10 * time.Second
	}
	c, err := Start(t.Context(), opts)
	if err != nil {
		t.Fatalf("Start(%s): %v", scenario, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// expectedArgs is what the standard scenario requires for each command;
// any mismatch is answered with an error response.
var expectedArgs = map[string]map[string]any{
	"authorize":       {},
	"searchCatalog":   {"term": "daft", "limit": float64(3)},
	"artist":          {"artistId": "a1"},
	"album":           {"albumId": "al1"},
	"songAlbum":       {"songId": "s1"},
	"catalogPlaylist": {"playlistId": "pl1"},
	"playlists":       {},
	"playSongs":       {"ids": []any{"s1", "s2"}, "startIndex": float64(1)},
	"playPlaylist":    {"playlistId": "p1"},
	"pause":           {},
	"resume":          {},
	"next":            {},
	"previous":        {},
	"stop":            {},
	"seek":            {"seconds": 90.5},
	"volume":          {},
	"setVolume":       {"level": 0.25},
	"libraryPlaylist": {"playlistId": "p1"},
	"createPlaylist":  {"name": "Night Drive", "description": "After hours", "songIds": []any{"s1", "i.s2"}},
	"addToPlaylist":   {"playlistId": "p.new", "songIds": []any{"s1"}},
	"favorite":        {"songId": "s1"},
	"setFavorite":     {"songId": "s1", "on": true},
	"setRepeat":       {"mode": "all"},
	"favorites":       {"songIds": []any{"s1", "i.s2", "s3"}},
}

// playPlaylistFromArgs is what the standard scenario requires for a
// playPlaylist that starts at a track; without "startIndex" the request
// must match expectedArgs["playPlaylist"] instead.
var playPlaylistFromArgs = map[string]any{"playlistId": "p1", "startIndex": float64(2)}

// wantArgs returns the arguments the standard scenario requires for req.
func wantArgs(cmd string, req map[string]any) (map[string]any, bool) {
	if _, from := req["startIndex"]; cmd == "playPlaylist" && from {
		return playPlaylistFromArgs, true
	}
	want, known := expectedArgs[cmd]
	return want, known
}

var stdout = bufio.NewWriter(os.Stdout)

func emit(v any) {
	line, _ := json.Marshal(v)
	stdout.Write(append(line, '\n'))
	stdout.Flush()
}

func ok(id string, result map[string]any) {
	emit(map[string]any{"id": id, "ok": true, "result": result})
}

func fail(id, message string) {
	emit(map[string]any{"id": id, "ok": false, "error": message})
}

func runFakeHelper(scenario string) int {
	if scenario == "mute" {
		time.Sleep(time.Hour)
		return 0
	}
	if scenario == "earlyExit" {
		fmt.Fprintln(os.Stderr, "fatal: no entitlement")
		return 2
	}
	emit(map[string]any{"event": "ready"})

	switch scenario {
	case "deaf":
		// Never reads stdin, so the client's writes eventually block.
		time.Sleep(time.Hour)
		return 0
	case "closedStdin":
		// Stops accepting requests while staying alive.
		os.Stdin.Close()
		time.Sleep(time.Hour)
		return 0
	case "oversize":
		// A line longer than the client accepts makes the stream undecodable.
		stdout.Write(bytes.Repeat([]byte("x"), maxLineBytes+1))
		stdout.WriteString("\n")
		stdout.Flush()
		time.Sleep(time.Hour)
		return 0
	}

	in := bufio.NewScanner(os.Stdin)
	var held []map[string]any
	level := 0.3 // the "volume" scenario's current level
	outOfRange := []float64{1.4, -0.2}
	for in.Scan() {
		var req map[string]any
		if err := json.Unmarshal(in.Bytes(), &req); err != nil {
			fail("", "malformed JSON request")
			continue
		}
		id, _ := req["id"].(string)
		cmd, _ := req["cmd"].(string)
		delete(req, "id")
		delete(req, "cmd")

		switch scenario {
		case "crash":
			fmt.Fprintln(os.Stderr, "fatal: helper crashed")
			return 3
		case "reorder":
			// Hold two requests, then answer them in reverse order.
			req["id"], req["cmd"] = id, cmd
			held = append(held, req)
			if len(held) == 2 {
				for i := len(held) - 1; i >= 0; i-- {
					answer(held[i]["id"].(string), held[i]["cmd"].(string))
				}
				held = nil
			}
			continue
		case "levelsNoWave":
			// A helper older than the waveform sends bands only.
			if cmd == "pause" {
				emit(map[string]any{"event": "levels", "bands": []int{30, 60}})
				ok(id, map[string]any{})
				continue
			}
		case "silentAuth":
			if cmd == "authorize" {
				continue // never answered
			}
		case "sparseArtist":
			// Answers artist by id: sections missing, or an error.
			if cmd == "artist" {
				answerSparseArtist(id, req["artistId"])
				continue
			}
		case "sparseDetail":
			// Answers the album and playlist pages by id: fields
			// missing, or an error.
			switch cmd {
			case "album":
				answerSparseDetail(id, req["albumId"], "album not found")
				continue
			case "songAlbum":
				answerSparseDetail(id, req["songId"], "song not found")
				continue
			case "catalogPlaylist":
				answerSparseDetail(id, req["playlistId"], "playlist not found")
				continue
			case "libraryPlaylist":
				answerSparseDetail(id, req["playlistId"], "playlist not found in the library")
				continue
			}
		case "volume":
			// Keeps the level it is sent and reports it back; refuses
			// exactly 0.5.
			switch cmd {
			case "volume":
				ok(id, map[string]any{"level": level, "mode": "app"})
				continue
			case "setVolume":
				v, isNumber := req["level"].(float64)
				switch {
				case !isNumber:
					fail(id, `setVolume requires a number "level"`)
				case v == 0.5:
					fail(id, "output device has no settable volume")
				default:
					level = v
					ok(id, map[string]any{"level": v, "mode": "app"})
				}
				continue
			}
		case "volumeOutOfRange":
			// Reports levels a broken device might: above one, then
			// below zero.
			if cmd == "volume" {
				ok(id, map[string]any{"level": outOfRange[0]})
				outOfRange = outOfRange[1:]
				continue
			}
		case "libraryEdit":
			// Library edit edge cases and failures, by argument.
			if answerLibraryEdit(id, cmd, req) {
				continue
			}
		case "leftOut":
			// Answers playSongs as a helper that left songs out of the
			// queue: two not in the catalog, one it could not queue, and
			// the rest refused by the player.
			if cmd == "playSongs" {
				ok(id, map[string]any{"missing": []any{"s8", "s9"}, "skipped": []any{"s3"}, "startedAlone": true})
				continue
			}
		case "sparseCatalog":
			// Answers searchCatalog by term: empty result, fields
			// missing, or an error response.
			if cmd == "searchCatalog" {
				answerSparseCatalog(id, req["term"])
				continue
			}
		}

		if want, known := wantArgs(cmd, req); !known {
			fail(id, "unknown command: "+cmd)
		} else if !reflect.DeepEqual(req, want) {
			fail(id, fmt.Sprintf("unexpected args for %s: %v", cmd, req))
		} else {
			answer(id, cmd)
		}
	}
	if scenario == "stubborn" {
		time.Sleep(time.Hour) // ignores EOF
	}
	return 0
}

func answer(id, cmd string) {
	switch cmd {
	case "authorize":
		ok(id, map[string]any{"status": "authorized"})
	case "searchCatalog":
		artist := map[string]any{"id": "a1", "name": "Daft Punk", "genres": []any{"Electronic", "Dance"}}
		album := map[string]any{"id": "al1", "title": "Discovery", "artist": "Daft Punk", "year": 2001, "trackCount": 14}
		song := map[string]any{"id": "s1", "title": "One More Time", "artist": "Daft Punk", "album": "Discovery", "duration": 320.5}
		playlist := map[string]any{"id": "pl1", "name": "Daft Punk Essentials", "curator": "Apple Music Electronic"}
		ok(id, map[string]any{
			"suggestions": []any{"daft punk", "daft punk discovery"},
			"top": []any{
				map[string]any{"kind": "artist", "artist": artist},
				map[string]any{"kind": "station", "station": map[string]any{"id": "st1"}},
				map[string]any{"kind": "song", "song": song},
				map[string]any{"kind": "album", "album": album},
				map[string]any{"kind": "playlist", "playlist": playlist},
			},
			"artists":   []any{artist},
			"albums":    []any{album},
			"songs":     []any{song},
			"playlists": []any{playlist},
		})
	case "artist":
		album := func(id, title string, year, tracks int) map[string]any {
			return map[string]any{"id": id, "title": title, "artist": "Daft Punk", "year": year, "trackCount": tracks}
		}
		ok(id, map[string]any{
			"artist": map[string]any{"id": "a1", "name": "Daft Punk", "genres": []any{"Electronic"}},
			"topSongs": []any{
				map[string]any{"id": "s1", "title": "One More Time", "artist": "Daft Punk", "album": "Discovery", "duration": 320.5},
			},
			"essentialAlbums": []any{album("al1", "Discovery", 2001, 14)},
			"albums":          []any{album("al1", "Discovery", 2001, 14), album("al2", "Homework", 1997, 16)},
			"singles":         []any{album("sg1", "Get Lucky", 2013, 1)},
			"compilations":    []any{album("c1", "Musique, Vol. 1", 2006, 15)},
			"playlists":       []any{map[string]any{"id": "pl1", "name": "Daft Punk Essentials", "curator": "Apple Music Electronic"}},
			"about":           map[string]any{"notes": "French duo.", "genre": "Electronic", "origin": "Paris, France", "formed": "1993"},
		})
	case "album", "songAlbum":
		track := func(id, title string, secs float64, number int) map[string]any {
			return map[string]any{"id": id, "title": title, "artist": "Daft Punk", "album": "Discovery", "duration": secs, "trackNumber": number, "discNumber": 1}
		}
		ok(id, map[string]any{
			"album":       map[string]any{"id": "al1", "title": "Discovery", "artist": "Daft Punk", "year": 2001, "trackCount": 2},
			"tracks":      []any{track("s1", "One More Time", 320.5, 1), track("s2", "Aerodynamic", 207, 2)},
			"genre":       "Electronic",
			"releaseDate": "2001-03-07",
			"recordLabel": "Parlophone",
			"copyright":   "℗ 2001 Daft Life Ltd.",
			"notes":       "The robots arrive.",
		})
	case "catalogPlaylist":
		ok(id, map[string]any{
			"playlist": map[string]any{"id": "pl1", "name": "Daft Punk Essentials", "curator": "Apple Music Electronic"},
			"tracks": []any{
				map[string]any{"id": "s3", "title": "Get Lucky", "artist": "Daft Punk", "album": "Random Access Memories", "duration": 369},
			},
			"notes": "Robot rock.",
		})
	case "playlists":
		ok(id, map[string]any{"playlists": []any{
			map[string]any{"id": "p1", "name": "Night City", "editable": true},
			map[string]any{"id": "p.fav", "name": "Favorite Songs", "editable": false},
		}})
	case "libraryPlaylist":
		ok(id, map[string]any{
			"playlist": map[string]any{"id": "p1", "name": "Night City"},
			"tracks": []any{
				map[string]any{"id": "1440857781", "title": "Nightcall", "artist": "Kavinsky", "album": "OutRun", "duration": 258},
				map[string]any{"id": "i.s2", "title": "Resonance", "artist": "Home", "album": "Odyssey", "duration": 212.25, "libraryOnly": true},
			},
			"notes": "After hours.",
		})
	case "volume":
		ok(id, map[string]any{"level": 0.42, "mode": "app"})
	case "createPlaylist":
		ok(id, map[string]any{"id": "p.new", "name": "Night Drive"})
	case "favorite":
		ok(id, map[string]any{"favorite": true})
	case "favorites":
		// s3 is left out: every requested id must still be answered.
		ok(id, map[string]any{"favorites": map[string]any{"s1": true, "i.s2": false}})
	case "resume":
		emit(map[string]any{"event": "state", "state": map[string]any{
			"status": "playing", "title": "One More Time", "artist": "Daft Punk",
			"album": "Discovery", "songId": "s1", "duration": 320.5, "position": 12.25,
			"repeat": "one",
		}})
		ok(id, map[string]any{})
	case "pause":
		emit(map[string]any{"event": "levels", "bands": []int{10, 20}, "wave": []int{1, 2}})
		emit(map[string]any{"event": "levels", "bands": []int{0, 50, 100, 120, -5}, "wave": []int{0, 50, -100, 130, -150}})
		emit(map[string]any{"event": "levels"})
		ok(id, map[string]any{})
	case "previous":
		emit(map[string]any{"event": "error", "message": "failed to encode message"})
		ok(id, map[string]any{})
	case "stop":
		fail("", "malformed JSON request")
		ok(id, map[string]any{})
	case "next":
		fail(id, "queue is empty")
	default:
		ok(id, map[string]any{})
	}
}

func answerSparseCatalog(id string, term any) {
	switch term {
	case "empty":
		ok(id, map[string]any{})
	case "sparse":
		ok(id, map[string]any{
			"top": []any{
				map[string]any{"artist": map[string]any{"id": "a1"}},
				map[string]any{"kind": "album"},
				map[string]any{"kind": "curator", "curator": map[string]any{"id": "c1"}},
				map[string]any{"kind": "song", "song": map[string]any{"id": "s1", "title": "One More Time"}},
			},
			"artists":   []any{map[string]any{"id": "a1", "name": "Daft Punk"}},
			"albums":    []any{map[string]any{"id": "al1", "title": "Discovery"}},
			"songs":     []any{map[string]any{"id": "s1", "title": "One More Time"}},
			"playlists": []any{map[string]any{"id": "pl1", "name": "Mix"}},
		})
	default:
		fail(id, "catalog unavailable")
	}
}

func answerSparseArtist(id string, artistID any) {
	switch artistID {
	case "empty":
		ok(id, map[string]any{})
	case "sparse":
		ok(id, map[string]any{
			"artist": map[string]any{"id": "a1", "name": "Daft Punk"},
			"albums": []any{map[string]any{"id": "al1", "title": "Discovery"}},
			"about":  map[string]any{"genre": "Electronic"},
		})
	default:
		fail(id, "artist not found")
	}
}

func answerSparseDetail(id string, entityID any, missing string) {
	switch entityID {
	case "empty":
		ok(id, map[string]any{})
	case "sparse":
		ok(id, map[string]any{
			"album":    map[string]any{"id": "al1", "title": "Discovery"},
			"playlist": map[string]any{"id": "pl1", "name": "Mix"},
			"tracks":   []any{map[string]any{"id": "s1", "title": "One More Time"}},
		})
	default:
		fail(id, missing)
	}
}

// answerLibraryEdit answers the "libraryEdit" scenario; it reports false
// for a request it does not handle.
func answerLibraryEdit(id, cmd string, req map[string]any) bool {
	switch {
	case cmd == "createPlaylist" && req["name"] == "Empty":
		// Only an explicit empty array and no description are accepted.
		if songs, isList := req["songIds"].([]any); !isList || len(songs) != 0 || req["description"] != nil {
			fail(id, fmt.Sprintf("unexpected args for createPlaylist: %v", req))
		} else {
			ok(id, map[string]any{"id": "p.empty", "name": "Empty"})
		}
	case cmd == "createPlaylist" && req["name"] == "boom":
		fail(id, "Apple Music failed (HTTP 500)")
	case cmd == "addToPlaylist" && req["playlistId"] == "p.locked":
		fail(id, "playlist is not editable (Forbidden)")
	case cmd == "favorite" && req["songId"] == "unrated":
		ok(id, map[string]any{"favorite": false})
	case cmd == "favorite" && req["songId"] == "denied":
		fail(id, "Apple Music did not accept the credentials")
	case cmd == "setFavorite" && req["songId"] == "s1" && req["on"] == false:
		ok(id, map[string]any{})
	case cmd == "setFavorite" && req["songId"] == "123456789012345678":
		fail(id, `song "123456789012345678" has no Apple Music API id`)
	default:
		return false
	}
	return true
}
