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
- [ ] T3 — Cyberpunk radio TUI (theme, layout, stations, now playing, EQ bars, search, keys), tested with a fake Player. Route: delegated.
- [ ] T5 — Helper hardening from T1 review advisories: queue subscription leak (StateEmitter.swift:59-64), per-request ordering (main.swift:23-27), EOF hang when a request never finishes (main.swift:30-33), stdout backpressure (Output.swift:12-21), stderr write failures, silent drop of unknown ids in playSongs (Commands.swift:93), seek does not emit state; **`playPlaylist` can never receive its playlist id** because `Codec.decode` strips the `id` key as the request id (confirmed live) — read `playlistId` instead, which the Go adapter already sends. Route: delegated.
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

## Next step

T3 — Cyberpunk radio TUI against `playback.Player`, tested with `playbacktest.Fake`. T5 must fix `playPlaylist` (`playlistId`) before stations can play live.
