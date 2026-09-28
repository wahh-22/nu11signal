# Feature: radio-tui

Locator: `odd/tasks/radio-tui.md` · Engram mirror: `odd/radio-tui/tasks` · Branch: `feat/radio-tui`

## Objective

A lightweight macOS terminal Apple Music player styled after the Cyberpunk 2077 car radio.

## Problem / Why

vibez streams through headless Chrome (hundreds of MB). The spike (`spike/`) proved that a windowless, signed Swift helper using MusicKit `ApplicationMusicPlayer` authorizes, searches the full catalog, and plays full tracks at ~31 MB RSS and ~0% CPU.

## Scope

- Swift helper (`helper/`): SwiftPM executable packaged as windowless `SoulKingHelper.app`, speaking a JSON-lines protocol on stdin/stdout.
- Go TUI (`cmd/soul-king`, `internal/...`): Bubbletea + Lipgloss, hexagonal: a `Player` port, a helper-process adapter, and a radio UI.
- Radio UX: library playlists act as "stations" with pseudo FM frequencies; now-playing panel; animated (decorative) EQ bars; progress bar; catalog search; play/pause/next/prev.
- Build tooling: Makefile, signing via env vars; README.

## Constraints

- macOS 14+ only. Signing material (provisioning profile, Team ID) comes from env/local files; never committed.
- Team `W6GZP998GQ`, bundle ID `dev.wahh.soulking.player` (configurable at build time).
- MusicKit gives no audio samples, so the EQ bars are decorative, driven by playback state.
- Artifacts in English.

## Delivery

- No remote yet: work-unit commits on `feat/radio-tui`; push/PR remain the user's decision. Strategy: `ask-on-risk` (PR slicing deferred until a remote exists).
- Forecast: ~1500 authored lines (helper ~350, Go adapter ~350, TUI ~600, tooling/docs ~200).

## Tasks

- [x] T1 — Swift helper: JSON-lines protocol (auth, search, playlists, play songs/playlist, pause, resume, next, prev, stop, periodic state events); build script bundles + signs the .app. Route: delegated (writer trigger: 2+ non-trivial files).
- [x] T2 — Go `Player` port + helper-process adapter + protocol codec, with tests against a fake helper. Route: delegated (writer trigger: 2+ non-trivial files).
- [x] T3 — Cyberpunk radio TUI (theme, layout, stations, now playing, EQ bars, search, keys), tested with a fake Player. Route: delegated (writer trigger: 2+ non-trivial files).
- [ ] T5 — Helper hardening from T1 review advisories: queue subscription leak (StateEmitter.swift:59-64), per-request ordering (main.swift:23-27), EOF hang when a request never finishes (main.swift:30-33), stdout backpressure (Output.swift:12-21), stderr write failures, silent drop of unknown ids in playSongs (Commands.swift:93), seek does not emit state Route: delegated.
- [ ] T6 — Go adapter hardening from T2 review advisories: CWD-relative helper lookup allows binary planting (locate.go:47-49) — restrict to explicit env or executable-relative paths; writes to helper stdin are not ctx-bounded and send errors are not mapped to ErrHelperExited (client.go:212-221); untested failure paths (client.go:257-262); fake buffer magic number; unasserted fake args. Route: delegated.
- [ ] T4 — Makefile, README (setup: App ID, profile, env vars), end-to-end manual run. Route: delegated or inline, depending on size.

## Acceptance criteria

- `make build` produces the Go binary and a signed helper bundle; `soul-king` launches the helper invisibly.
- Search, play, pause/resume, next/prev, and station (playlist) playback work against real Apple Music.
- `go test ./...` passes; `go vet ./...` is clean.
- The UI reads as Cyberpunk 2077 radio: dark background, red/cyan/yellow neon accents, frequency list, EQ bars, uppercase condensed labels.

## Checks

- `go test ./...`, `go vet ./...`
- `swift build -c release` in `helper/`
- Manual: run `soul-king`, play a station and a search result.

## Progress

- Baseline commit `e2ed8a5` (spike). Branch `feat/radio-tui` created.
- T1 done (route: delegated writer; trigger: writer, 2+ non-trivial files). Commit: `89d103c` (`feat(helper): add MusicKit JSON-lines helper`).
  - Layout: `helper/Package.swift`; `SoulKingProtocol` (pure codec, unit tested); `soulking-helper` (`main.swift`, `Output.swift`, `Commands.swift`, `StateEmitter.swift`); templates in `helper/Resources/*.plist.in`; `helper/build.sh`.
  - Test-first exception: MusicKit behavior only runs inside a signed bundle, so no deterministic RED exists for commands; the pure codec has XCTest coverage (8 tests) and commands were checked by runtime smoke tests.
  - Evidence: `swift build -c release` → Build complete, no warnings; `swift test` → 8 tests, 0 failures; `./helper/build.sh` → `valid on disk`, `satisfies its Designated Requirement`; missing profile → clear error, exit 1; smoke (authorize/search/playlists/malformed/unknown) → ready, authorized, 3 songs, 5 playlists, error responses, clean exit 0 on EOF; short playback (playSongs/pause/seek/resume/next/stop, ~4 s audio) → ok responses and state events, exit 0.
  - Protocol notes: `search` limit is clamped to 1...25 (catalog page maximum); on EOF the helper waits for in-flight requests before stopping playback and exiting.

  - Native review (RDD on): tier high; consent granted; 4-lens review approved, receipt acknowledged (lineage `review-eb84c4e5dcc38eb1`, authority burned). Advisory findings moved to T5. Reviewed boundary advances to `89d103c`.

- T2 done (route: delegated writer; trigger: writer, 2+ non-trivial files). Commit: `c889017` (`feat(playback): add Player port and helper process adapter`).
  - Layout: `go.mod` (module `soulking`, go 1.26.2, no deps); `internal/playback` (domain types + `Player` port); `internal/playback/playbacktest` (in-memory `Fake` recording calls, `PushState`/`PushError`); `internal/helper` (`client.go` process/correlation, `commands.go` Player methods, `wire.go` codec, `locate.go`, `stderr.go` bounded tail).
  - Test-first: RED observed as build failures (`undefined: Client`, `Options`, `CommandError`; later `undefined: Fake`, `New`, `Call`), then GREEN. First GREEN run caught the `playPlaylist` id collision (fake rejected `map[]` args).
  - Tests re-exec the test binary as a fake helper: round trip with exact wire args, out-of-order correlation, `CommandError` mapping, state decoding (seconds → Duration), async error events and id-less error responses, ctx cancellation, crash fails pending calls with stderr in the error and closes channels, clean Close, kill of a helper ignoring EOF, ready timeout and early exit, `Locate` order.
  - Evidence: `go test -race -count=5 ./...` → 150 passed; `go vet ./...` clean; `gofmt -l .` empty. Live (throwaway test, not committed) against `build/SoulKingHelper.app`: Authorize → authorized; Search("Daft Punk", 3) → 3 songs with durations; Playlists → 5; PlayPlaylist → helper error (T5 defect above); Close → nil.
  - Size: ~830 production + ~620 test lines, above the ~400 advisory heuristic because the process adapter, fake Player, and subprocess test harness form one coherent unit.

  - Fix (inline, 1-line mechanical): helper `playPlaylist` reads `playlistId` (the codec reserves `id`); verified live: playlist queued and state `playing`.

  - T2 native review: tier high; consent granted; 4-lens review approved, receipt acknowledged (lineage `review-d9aa5c1667992405`, authority burned). Advisories moved to T6. Reviewed boundary advances to `658b5fc`.

- T3 done (route: delegated writer; trigger: writer, 2+ non-trivial files). Commits: `78f628c` (`feat(playback): add simulated demo player`), `1b3722a` (`feat(radio): add Cyberpunk radio TUI`).
  - Dependencies: Bubble Tea v2 line (`charm.land/bubbletea/v2` v2.0.10, `charm.land/lipgloss/v2` v2.0.6, `charm.land/bubbles/v2` v2.2.1), `github.com/charmbracelet/x/ansi` v0.11.8.
  - Layout: `internal/radio` (`model.go` state + Player cmds, `update.go`, `view.go`, `theme.go` palette + notched `panel`, `eq.go` seeded EQ + title glitch, `format.go` frequency/mm:ss/progress, `keys.go`); `internal/playback/demo` (simulated Player, fictional library); `cmd/soul-king/main.go` (`--demo`, else `helper.Locate` → `helper.Start`).
  - Behavior: stations at 088.1 + 1.6 MHz steps; enter plays station (PlayPlaylist) or result (PlaySongs, all ids, cursor index); space pauses/resumes by status; n/p; ←/→ seek ±10s from the interpolated position, clamped; `/` CATALOG SCAN → Search(term, 25); tab/esc switch lists; q/ctrl+c close the player then quit. States/Errors re-arm per message; closed channels → SIGNAL LOST. Call errors and async errors → yellow status line expiring after 4s. Ticks: 100ms while animating, 1s idle, generation-tagged so rescheduling never doubles the rate. Layouts: full (≥60x16), compact single column, tiny wordmark; never exceeds the terminal size.
  - Test-first: RED observed as build failures (`undefined: Model`, `eq`, `frequency`; demo: `undefined: Player`), then GREEN.
  - Evidence: `go test -race ./...` → 102 passed (6 packages); `go test -race -count=2 ./...` stable; `go vet ./...` clean; `gofmt -l .` empty; `go build -o bin/soul-king ./cmd/soul-king` OK. Golden `internal/radio/testdata/view_80x24.golden` (ANSI stripped, fixed clock and seed). Demo frame rendered at 100x30 via a throwaway test (not committed).
  - Size: ~1180 production + ~720 test lines in `internal/radio`, ~350 + 136 in the demo player; above the ~400 advisory heuristic because the TUI's model, update, and view are one coherent unit whose tests must land with it.

## Next step

Native review of T3 (candidate range `658b5fc..1b3722a`), then T5/T6 hardening and T4 tooling.
