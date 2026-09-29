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
	"authorize":    {},
	"search":       {"term": "daft punk", "limit": float64(2)},
	"playlists":    {},
	"playSongs":    {"ids": []any{"s1", "s2"}, "startIndex": float64(1)},
	"playPlaylist": {"playlistId": "p1"},
	"pause":        {},
	"resume":       {},
	"next":         {},
	"previous":     {},
	"stop":         {},
	"seek":         {"seconds": 90.5},
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
		case "silentAuth":
			if cmd == "authorize" {
				continue // never answered
			}
		}

		if want, known := expectedArgs[cmd]; !known {
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
	case "search":
		ok(id, map[string]any{"songs": []any{
			map[string]any{"id": "s1", "title": "One More Time", "artist": "Daft Punk", "album": "Discovery", "duration": 320.5},
			map[string]any{"id": "s2", "title": "Digital Love", "artist": "Daft Punk", "album": "Discovery", "duration": 301},
		}})
	case "playlists":
		ok(id, map[string]any{"playlists": []any{
			map[string]any{"id": "p1", "name": "Night City"},
		}})
	case "resume":
		emit(map[string]any{"event": "state", "state": map[string]any{
			"status": "playing", "title": "One More Time", "artist": "Daft Punk",
			"album": "Discovery", "songId": "s1", "duration": 320.5, "position": 12.25,
		}})
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
