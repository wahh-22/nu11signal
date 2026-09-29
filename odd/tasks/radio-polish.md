# Feature: radio-polish

Locator: `odd/tasks/radio-polish.md` · Engram mirror: `odd/radio-polish/tasks` · Branches (stacked to main): `fix/radio-polish` → `feat/player-backend` → `feat/playlists-volume`

## Objective

Third round of user feedback (2026-09-29, after testing main `a887d31`).

## Problem / Why

1. No volume control.
2. The BACK button shows on the SEARCH base; it should only appear inside artist/album/song/playlist (and results) pages to go back toward the initial search — tab already goes to the stations.
3. The ▶ marker in a SONG/ALBUM view stays on the searched song while another track of that album is playing (screenshot: ▶ on "BESOS USADOS", playing "FALTARÁN").
4. Rename STATIONS to PLAYLISTS; selecting a playlist should show its track list (like the search PLAYLIST view) instead of tuning immediately.
5. Recent searches should be removed individually only (drop CLEAR RECENT).
6. Vertical navigation should reach the nav tabs: pressing ↑ past the top of a list (including from the SEARCH input) focuses the tabs; ←/→ move across tabs, enter activates, ↓ returns.

## Scope / Tasks

- [x] T1 — Radio polish (internal/radio only): ▶ follows the now-playing track in track pages (the searched song only sets the initial cursor); BACK only when a page is above SEARCH/PLAYLISTS root; remove CLEAR RECENT (keep per-row ✕ and ctrl+d/delete); nav-tab focus area (↑ from the top of any list, from the SEARCH input and from the player's progress bar; ←/→ across tabs; enter; ↓ back). Branch `fix/radio-polish`. Route: delegated (writer trigger).
- [x] T2 — Player backend (parallel, isolated worktree): volume get/set (MusicKit player volume if the SDK has one; otherwise system output volume via CoreAudio in the helper), `libraryPlaylist(playlistId)` returning tracks, and playing a library playlist starting at a track; port + adapters + fake + demo + tests. Branch `feat/player-backend`. Route: delegated.
- [x] T3 — PLAYLISTS view + volume UI: rename STATIONS→PLAYLISTS, enter opens the library playlist's track view (enter on a track plays the playlist from it), volume keys (`+`/`-` outside the input), VOL−/VOL+ focusable player buttons and a volume readout. Branch `feat/playlists-volume`. Route: delegated.

## Constraints

- Keyboard and mouse stay equivalent; typing in the SEARCH input is never hijacked except ↑ from the input (explicit user request).
- Artifacts in English.

## Delivery

- Strategy `ask-on-risk`, chain `stacked-to-main`. Forecast ~1500 lines.

## Checks

- `go test -race ./...`, `go vet ./...`, `gofmt -l .`; `swift build` + `swift test` for helper changes; live helper smoke (no audio) for new commands.

## Progress

- Branch `fix/radio-polish` from `main` `a887d31`.
- T1 done (route: delegated writer). RED: 11 failing tests (marker ×3 incl. the old SONG expectation that encoded the bug, BACK visibility, 6 tab-focus, no clear-all). GREEN: `go test -race ./...` 473 passed (parent spot check), `go vet`/`gofmt` clean. Decisions: ▶ from `Model.playingIndex` (id, then title+artist case-insensitive; off when stopped); BACK only with a page on top; CLEAR RECENT removed; `areaTabs` reached by ↑ from the first row, the SEARCH input, the progress bar (or buttons when unseekable); ←/→ across drawn tabs, ↓/esc return exactly, enter acts like a click; wheel never climbs to tabs. `.claude/` (agent worktrees) excluded locally via .git/info/exclude. Commit `c7b9ff8`. RDD: high, 787 lines, consent granted, lineage `review-287ed7e41818ae0c`, 4 lenses, APPROVED, acknowledged (burned); boundary → `c7b9ff8`. Advisories → T3: space on the tabs falls through to the input (R3 WARNING: footer says PLAY/PAUSE); initial tab index literal; empty `up` case needs a comment; wheel guard relies on key handlers + only stations tested; misleading test comment; lone-song/stopped marker paths untested.
- T2 done (route: delegated writer, isolated worktree, commit `37675c7` on 7b2d43b, cherry-picked onto the T1 branch). Volume: MusicKit has no macOS player volume (swiftinterface grep; MPMusicPlayerController unavailable) → CoreAudio default output device `VirtualMainVolume`. Port: `Volume`, `SetVolume`, `ClampVolume`, `LibraryPlaylist`, `PlayPlaylistFrom` (`playPlaylist` + optional `startIndex`, Queue(playlist:startingAt:)). volume/setVolume run concurrently. Bug found live and fixed: library song durations arrive in milliseconds (`LibraryDuration.seconds`). RED/GREEN Go + Swift (LibraryDuration tests written with the fix). Verification: `go test -race` 475, `swift test` 64, live smoke (volume read/set to same value, 5 playlists, libraryPlaylist 15 songs). Open: StateEmitter durations for library songs may also be ms (check in T3); play-from-index not exercised live. Cherry-picked commit `7978f9f` (branch `feat/player-backend`). RDD: medium, 811 lines, slice budget reached, consent granted, lineage `review-b1312a65e80303a3`, reliability lens, APPROVED, acknowledged (burned); boundary → `7978f9f`. Advisories → T3: assert the level is unchanged after a refused SetVolume; test out-of-range levels reported by the helper.
- T3 done (route: delegated writer). RED: 28 failing tests (rename, playlist page open/play, PlayPlaylistFrom args, volume keys/buttons/focus/readout/errors/coalescing, space on tabs, enter-tunes assumptions); regression guards passed first (marker lone-song/stopped, wheel, helper clamp — mutation-checked). GREEN: `go test -race ./...` ok (parent spot check), `go vet`/`gofmt` clean, `swift test` 64 passed. Decisions: visible PLAYLISTS (FM flavor kept), library PLAYLIST page on the root with `▶ PLAY` row; volume `+`/`=`/`-` outside SEARCH and `shift+↑/↓` everywhere, 5% steps, single in-flight call with latest-level coalescing; VOL row `╱ - ╱ VOL ▮▮▮▮▯▯ 60% ╱ + ╱` under the transport (no room at 80 cols); StateEmitter durations via `LibraryDuration.seconds`. Pending live audio checks: PlayPlaylistFrom start, library ▶ matching, library progress length, volume on non-settable outputs. Commit `ecfc65e`. RDD: high, 1601 lines, consent granted, lineage `review-647f315223203667`, 4 lenses, APPROVED, acknowledged (burned); boundary → `ecfc65e`.
- T3f (route: inline, two small edits + test): T3 review WARNING — presses before the startup volume read started a second read and could be overwritten. `volumeBusy` now starts true (Init reads), steps pressed while a read is in flight accumulate in `volumePending` and apply when it answers. RED: `TestVolumePressesBeforeTheStartupReadWaitForIt` (commands true false); GREEN: `go test -race ./...` 548 passed, `go vet`/`gofmt` clean. Not scheduled: compact volume-row fit check tied to enum order; onPlaylistsBranch duplicated in searchAgain; old internal names (zoneTabStations, tune); dead half of a rename assertion; LibraryDuration threshold untested for catalog.

## Next step

All tasks done. The user decides push/PRs (chain: fix/radio-polish → feat/player-backend → feat/playlists-volume).
