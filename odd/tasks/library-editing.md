# Feature: library-editing

Locator: `odd/tasks/library-editing.md` · Engram mirror: `odd/library-editing/tasks` · Branches (stacked to main): `fix/playback-start` → `feat/volume-keys-click` → `feat/library-edit-backend` → `feat/library-edit-ui`

## Objective

Fourth round of user feedback (2026-09-29, main `bc4f8b5`).

## Problem / Why

1. Library playlist page: selecting track 4 plays track 1 (`PlayPlaylistFrom` start not honored).
2. Sometimes playing a song fails with `MPMusicPlayerControllerErrorDomain Code=6 "prepare queue failed with unexpected start item"` from `playSongs`; the user has to press enter several times.
3. Volume needs a letter shortcut. Decision (parent, user asked to evaluate): `k` volume up, `j` volume down; arrows alone move rows/focus (j/k stop moving rows); `shift+↑/↓` and `+`/`-` stay.
4. Clicking a panel should give it focus immediately (not only its controls).
5. Create and edit playlists, and add songs to favorites.

## Tasks

- [x] B1 — Playback start fixes (helper + adapter): honor the start track for library playlists; make `playSongs` robust to Code=6 (root cause first: start item not in the prepared queue / id mismatch / race; fix or retry once, never loop). Branch `fix/playback-start`. Route: delegated (isolated worktree, parallel with U1).
- [x] U1 — Volume on j/k and click-to-focus panels (internal/radio). Branch `feat/volume-keys-click`. Route: delegated (parallel with B1).
- [ ] B2 — Library editing backend via the Apple Music API (`MusicDataRequest`, user decision 2026-09-29): create playlist (with songs), add songs to a library playlist, favorite (love rating) a song; no rename/remove/delete (not exposed by the API). Port + helper + adapters + fake + demo. Branch `feat/library-edit-backend`. Route: delegated.
- [ ] U2 — Library editing UI: new playlist, rename, add song to a playlist (picker), remove track, favorite toggle; keyboard + mouse. Branch `feat/library-edit-ui`. Route: delegated.

## Constraints

- Never play audio in automated smoke tests without the user's consent; live read-only commands are fine.
- Artifacts in English.

## Delivery

- Strategy `ask-on-risk`, chain `stacked-to-main`. Forecast ~2000 lines.

## Progress

- Branch `fix/playback-start` from `main` `bc4f8b5`.
- Finding (parent, SDK `MacOSX.sdk` MusicKit swiftinterface): `MusicLibrary.createPlaylist/edit/add` are `@available(macOS, unavailable)`; no favorites API in MusicKit. User chose the Apple Music web API via `MusicDataRequest` (create, add, favorite; no rename/remove).
- B1 done (route: delegated writer, isolated worktree, `72e6851` cherry-picked as `f9b6f6d`). Inferred causes (no audio allowed): `Queue(playlist:startingAt:)` with a playlist loaded without entries ignores the start → queue built from the exact `libraryPlaylist` song list; Code=6 → start named as a queue entry (`Queue(entries, startingAt:)`, `SongQueue` pure helper) plus ONE retry on Code=6. RED: `SongQueue` missing; GREEN: `swift test` 72. Needs audio verification by the user.
- U1 done (route: delegated writer). `k`/`j` volume up/down everywhere outside the SEARCH input; arrows alone move rows; panel-wide click zones registered under rows/controls give focus. RED: key/volume and panel-zone tests; GREEN: `go test -race ./...` 575 passed; commit `453104e` (rebased onto B1).
  - B1+U1 slice review (base `90ee87d`): high, 638 lines, consent granted, lineage `review-40d080d7ed45bdad`, 4 lenses, APPROVED, acknowledged (burned). Advisories → B2: log Code=6 retries to stderr; explicit range check in playPlaylist (no bare `_ =`); named Code=6 constant; test the retry path via a seam if feasible. Not scheduled: panel-zone ordering comment, compact click precedence test.

## Next step

B2 on `feat/library-edit-backend`.
