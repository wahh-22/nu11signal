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
- [x] T5 — Helper hardening from T1 review advisories: queue subscription leak (StateEmitter.swift:59-64), per-request ordering (main.swift:23-27), EOF hang when a request never finishes (main.swift:30-33), stdout backpressure (Output.swift:12-21), stderr write failures, silent drop of unknown ids in playSongs (Commands.swift:93), seek does not emit state; codec int truncation and encode fallback. Route: delegated (writer trigger: 2+ non-trivial files).
- [x] T6 — Go adapter hardening from T2 review advisories: CWD-relative helper lookup allows binary planting (locate.go:47-49) — restrict to explicit env or executable-relative paths; writes to helper stdin are not ctx-bounded and send errors are not mapped to ErrHelperExited (client.go:212-221); untested failure paths (client.go:257-262); fake buffer magic number; unasserted fake args. Route: delegated.
- [x] T7 — Radio TUI hardening from T3 review advisories: CWD helper lookup now reachable from main (main.go:54-60, pairs with T6); helper start ctx cancel (main.go:58-60); ON-AIR mark width uses bytes not cells (view.go:299-313); playing-station marker set optimistically before the helper confirms (update.go:217-221); no retry when loading playlists fails (update.go:30-34); rapid seeks don't accumulate (update.go:224-232); unbounded Close on quit (model.go:185-190). Route: delegated.
- [x] T8 — Minor advisories from T6/T7 review: play-confirmation ordering when two tunes overlap (update.go:47-53); closed-stdin test could hang without a timeout (client_test.go:231-243); abandoned write goroutine holds the write lock (client.go:250-267); naming nits (model.go:36, view.go:305, update.go:242-248). From T5 review: a hung playback command blocks all later ones (main.swift:29-35) — add a per-command timeout; int boundary inconsistency (Codec.swift:38); writer shutdown untested (Output.swift:39-48). Route: delegated.
- [x] T4 — Makefile, README (setup: App ID, profile, env vars), end-to-end manual run. Route: delegated (writer trigger: Makefile, README, build.sh).

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

  - T3 native review: tier high; consent granted; 4-lens review approved, receipt acknowledged (lineage `review-41fd22bf8297d85e`, authority burned). Advisories moved to T7. Reviewed boundary advances to `1b3722a`.

- Manual e2e (user, 2026-09-28): `bin/soul-king --demo` and live mode — interaction works and audio plays correctly.

- T6 + T7 done (route: delegated writer; trigger: writer, 2+ non-trivial files across `internal/helper`, `internal/radio`, `cmd/soul-king`). Commits: `90d581a` (`fix(helper): resolve helper only from trusted locations`), `03bbee1` (`fix(helper): bound stdin writes by context and map send failures`), `88a36a9` (`fix(radio): confirm tuning, accumulate seeks, retry station list, bound quit`), `51af09a` (`fix(soul-king): scope helper startup context and abort it on interrupt`).
  - Helper lookup (security): the working directory is never searched. Order: `$SOULKING_HELPER` (absolute path to a regular executable; relative rejected) → `<exeDir>/SoulKingHelper.app/...` → `<exeDir>/../libexec/...` (future Homebrew) → `<exeDir>/../build/...` (dev: `bin/` + `build/`); `exeDir` from `os.Executable` through `filepath.EvalSymlinks`; the error lists every path tried.
  - Adapter: stdin writes run in a goroutine under `writeMu` and the caller waits on ctx (an abandoned write finishes or fails when stdin closes, so lines never interleave); write failures map to `ErrClosed` while closing, else `ErrHelperExited`. New tests: stuck helper bounded by ctx, closed helper stdin → `ErrHelperExited`, oversize output → read-output error + kill + channels closed, Start ctx scope. Fake: `playbacktest.ChannelBuffer` constant; call args asserted with `reflect.DeepEqual`.
  - TUI: ON-AIR mark width in cells (`ansi.StringWidth`); on-air station set only from a successful `playMsg` (failures keep the previous one); playlist failure shows `[R] RETRY` in the status line and the station panel, `r` reloads; seeks accumulate from a pending target (sequence-tagged, cleared on the latest answer or a song change; a confirmed seek updates the position); quit waits for `Close` at most `Options.CloseTimeout` (default 3s). main: startup ctx documented (bounds only the handshake; deferred cancel is correct) and aborted on SIGINT.
  - Test-first: RED observed — `TestLocateResolutionOrder` (old locator panicked on the removed working-dir lookup), `TestWriteToStuckHelperIsBoundedByContext` (blocked 5s), `TestSendAfterHelperStopsReadingIsHelperExited` (`send: write |1: broken pipe`, not `ErrHelperExited`), `TestStationMarkedOnAirOnlyAfterHelperConfirms` (3 subtests, marked before confirm / kept after failure), `TestStationListFailureOffersRetry` (no retry offer), `TestRapidSeeksAccumulate` (targets `[1m10s 1m10s 1m10s]`), `TestQuitDoesNotHangOnAStuckPlayer` (command blocked), `TestOnAirStationRowFillsWidth` (rows 2 cells short); then GREEN. No RED (guards for existing behavior): `TestUndecodableOutputKillsHelper`, `TestStartContextOnlyBoundsStartup`, `TestStartHonoursCancelledContext`, `TestFailedSeekDropsPendingTarget`, fake snapshot/buffer tests.
  - Evidence: `go test -race ./...` all packages ok; `go vet ./...` clean; `gofmt -l .` empty; `go build -o bin/soul-king ./cmd/soul-king` OK; golden unchanged. CWD planting check: with CWD `/tmp/soulking-plant.*` containing a planted `build/SoulKingHelper.app` helper, `Locate` (exe = repo `bin/soul-king`) returned the repo's `build/` helper and the planted one never ran; `SOULKING_HELPER=build/... bin/soul-king` from that dir exits 1 with "must be an absolute path" before any UI.

  - T6+T7 native review: tier high; consent granted; 4-lens review approved, receipt acknowledged (lineage `review-04ac8b75759713c3`, authority burned). Advisories moved to T8. Reviewed boundary advances to `51af09a`.

- T5 done (route: delegated writer; trigger: writer, 2+ non-trivial files in `helper/`). Commits: `c9439b3` (`fix(helper): reject non-integral ints and keep response ids on encode failure`), `ab58284` (`fix(helper): serialize playback commands and bound shutdown`).
  - Protocol stays backward compatible with the Go adapter; the only addition is an optional `"missing":[...]` in the `playSongs` result.
  - Codec: `Request.int` rejects fractional values and values beyond ±2^53 instead of truncating; `Codec.encode` validates with `isValidJSONObject` first (JSONSerialization raises an uncatchable ObjC exception on NaN/infinity/non-JSON types, which would crash the helper); a response that cannot be encoded becomes `{"id":<same id>,"ok":false,"error":"failed to encode response"}`, an event becomes an `error` event naming it.
  - Ordering: `Request.mutatesPlayback` (playSongs, playPlaylist, pause, resume, next, previous, stop, seek) — main.swift chains those tasks so each awaits the previous; authorize/search/playlists/unknown run concurrently.
  - Output: `SoulKingProtocol.Outbox` (bounded FIFO, capacity 256) feeds a dedicated stdout writer thread using `write(2)` with EINTR retry; when full it evicts the oldest queued `state` event (or drops the new one), never responses; a write failure closes the outbox and shuts down (EPIPE → exit 0). stderr `log` uses `write(2)` and ignores failures.
  - Shutdown (`Lifecycle`): at stdin EOF, waits up to 3s for in-flight requests and queued output, then stops playback on the main actor and exits; a watchdog `_exit`s 2s later if the main thread never gets there. Only the first shutdown call acts.
  - Also: StateEmitter keeps one `AnyCancellable` for the current queue (cancelled on replacement); seek emits a state event right after its response; `playSongs` errors listing ids when none are found, otherwise plays the found ones (start falls forward to the next found song) and returns `missing`; a present but non-integral `limit`/`startIndex` is an error.
  - Test-first: RED observed — `testIntRejectsFractionalNumbersInsteadOfTruncating` (got 1 and -2), `testIntRejectsValuesOutsideTheExactRange` (got Int.max and 2^53+1), `testUnencodableSuccessKeepsResponseID` / `testUnencodableEventBecomesErrorEvent` / `testNonJSONValueFallsBackInsteadOfCrashing` (`NSInvalidArgumentException`); then routing/outbox tests failed to build (`cannot find type 'Outbox'`); then GREEN. MusicKit behavior (ordering, seek event, playSongs, shutdown) can't run outside the signed bundle, so it was verified by live smoke tests.
  - Evidence: `swift build -c release` → Build complete, no warnings; `swift test` → 24 tests, 0 failures; `./helper/build.sh` → `valid on disk`, `satisfies its Designated Requirement`. Live smoke: authorize → authorized; search → 2 songs; search `limit:1.5` → `"limit" must be an integer`; playlists → 5; playSongs [bogus, real] → ok `{"missing":["0000000000"]}`; playSongs [bogus, bogus] → error listing both; playPlaylist → ok; burst pause/seek(30)/resume/stop → responses in order `m-pause, m-seek, state(position 30), m-resume, m-stop`; EOF → exit 0 in 0.04s; EOF right after a search → response delivered, exit 0 in 0.34s. Never-read stdout (20000 malformed lines, ~1.2 MB of responses into a 64 KB pipe) → stdin fully consumed in 0.12s, exit 0 after 3.25s (bounded grace). Reader closes stdout → `stdout write failed: Broken pipe; shutting down`, exit 0 in 0.12s. `go test -race -count=1 ./...` → all packages ok. Audio played ≈ 2 s.

  - T5 native review: tier medium (slice budget); consent granted; consolidated review approved, receipt acknowledged (lineage `review-0bce8ddac98d6980`, authority burned). Reviewed boundary advances to `ab58284`.

- T8 done (route: delegated writer; trigger: writer, 2+ non-trivial files across `internal/radio`, `internal/helper`, `helper/`). Commits: `22cf93f` (`fix(radio): let only the latest play request set the on-air station`), `4d914ba` (`fix(helper): bound serialized playback commands and align int range`).
  - Go: play requests carry a sequence number (`playSeq`); `onPlay` reports every failure but only the latest request sets `playingStation`. Helper client: writes take a capacity-1 `writeSlot` channel under the caller's ctx (waiters no longer park on a mutex); documented trade-off: a write abandoned mid-line keeps its goroutine and the slot until the helper reads, exits, or Close closes stdin (closing the pipe wakes it); Close never takes the slot, so it stays bounded by CloseTimeout. Closed-stdin test bounds each attempt (250ms) and retries timeouts. Naming: `defaultCallTimeout`/`defaultCloseTimeout`, `textWidth`, `playCmd(seq, op, station, request)`.
  - Swift: `SoulKingProtocol.Deadline.run(seconds:)` races the operation (unstructured task) against a timer and resumes once, so it holds even when the operation ignores cancellation; serialized playback commands use a 10s bound (`CommandHandler.playbackTimeout`) and a timeout is answered as an error so the chain continues (a late MusicKit result is discarded). `Request.int` uses one exclusive bound `|n| < 2^53` for integer and floating-point numbers. The stdout writer loop moved into `Outbox.drain` (closes the outbox and returns errno on a failed write), now unit tested; `Output.writeLoop` only logs and shuts down.
  - Test-first: RED observed — `TestOnlyTheLatestTuneSetsTheOnAirStation` (`playingStation = "pl-1"` after a stale confirmation), `testIntRangeIsTheSameForIntegerAndFloatingPointNumbers` (±9007199254740992 accepted on the integer path), Deadline/drain tests failed to build (`cannot find 'Deadline' in scope`); then GREEN. No RED (guard): `TestCloseIsBoundedWhileAWriteIsStuck` passes on the old client too (Close never took the write lock); the closed-stdin test change is robustness only.
  - Evidence: `go test -race ./...` → 126 passed (6 packages); `go test -race -count=3 ./internal/helper` ok; `swift test` → 31 tests, 0 failures; `swift build -c release` (clean) → 0 warnings. Live smoke via `build/SoulKingHelper.app`: authorize → authorized; search → songs; pause → ok; `limit: 9007199254740992` → `"limit" must be an integer`; EOF → exit 0.

- T4 done (route: delegated writer; trigger: writer, Makefile + README + build.sh). Commit: `f90b9d1` (`build: add Makefile and README with signing setup`).
  - Makefile: `build` (helper + go), `helper` (`helper/build.sh`), `go`, `demo` (Go only, `bin/soul-king --demo`), `test` (`go test -race ./...` + `swift test`), `vet`, `fmt-check`, `clean` (`bin/`, `build/`); exports `SOULKING_BUNDLE_ID`, `SOULKING_TEAM_ID`, `SOULKING_PROFILE`, `SOULKING_SIGN_IDENTITY`, `SOULKING_HELPER`.
  - `helper/build.sh`: profile default is `signing/SoulKing_Player.provisionprofile` (gitignored via `*.provisionprofile`), falling back to `spike/SoulKing_Player.provisionprofile` when only that exists; the missing-profile error names the expected path. The profile file was not moved.
  - README: purpose, architecture diagram, requirements, one-time signing setup, env vars, make targets, keys, helper lookup order, protocol summary, troubleshooting, resource footprint, `spike/` as historical, License: TBD.
  - Evidence: `make test` → exit 0 (Go all ok, Swift 31 tests 0 failures); `make vet` → exit 0; `make fmt-check` → exit 0; `make build` → exit 0 (`valid on disk`, `satisfies its Designated Requirement`, via the spike fallback profile; `bin/soul-king` built); `SOULKING_PROFILE=/nonexistent ./helper/build.sh` → clear error, exit 1; live smoke authorize → authorized, search "night city" → 2 songs, EOF exit 0. `make demo` not run here (interactive TUI).

  - T8+T4 native review: tier high; consent granted; 4-lens review approved, receipt acknowledged (lineage `review-394334f67e866a39`, authority burned). Reviewed boundary advances to `1073c9e`.

## Follow-ups (not started; need user decision)

- Helper playback timeout (10s, Commands.swift:26) exceeds the Go client's per-call deadline — align them.
- `Deadline` timer is not cancelled after the command wins (Deadline.swift:30-36); seconds conversion could trap on extreme values.
- Release: Developer ID certificate + Developer ID provisioning profile + notarization; Homebrew cask.
- License choice; move the profile to `signing/`; remote + push/PR.

## Next step

User decides on follow-ups, license, push/PR, and release signing.
