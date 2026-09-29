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
- [x] B2 — Library editing backend via the Apple Music API (`MusicDataRequest`, user decision 2026-09-29): create playlist (with songs), add songs to a library playlist, favorite (love rating) a song; no rename/remove/delete (not exposed by the API). Port + helper + adapters + fake + demo. Branch `feat/library-edit-backend`. Route: delegated.
- [x] B3 — Library read via the web API: `playlists` from `GET /v1/me/library/playlists` (paginated, `canEdit`), `libraryPlaylist` from `GET /v1/me/library/playlists/{id}/tracks` (paginated; Song.ID = catalog id from `playParams.catalogId`, falling back to the library id; non-catalog tracks not playable); playback of library playlists via catalog ids; B2 advisories. Branch `feat/library-web`. Route: delegated.
- [ ] U2f — U2 review follow-ups: unknown-outcome failures of create/add must not invite a blind retry (re-read playlists/tracks, warn); keep editor busy until an in-flight write answers; serialize favorite writes per song (latest intent wins); keep [ESC] BACK visible at 80 cols; openPicker focusList side effect; retry failed favorite reads after a while; small readability items. Branch `feat/library-edit-ui`. Route: delegated.
- [x] U2 — Library editing UI: new playlist, add song to a playlist (picker), favorite toggle; keyboard + mouse. (Rename/remove are not available: the Apple Music API does not expose them.) Branch `feat/library-edit-ui`. Route: delegated.

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
- B2 done (route: delegated writer). Commands `createPlaylist`, `addToPlaylist`, `favorite`, `setFavorite` via `MusicDataRequest` (POST /v1/me/library/playlists, POST …/{id}/tracks, GET/PUT/DELETE /v1/me/ratings/{songs|library-songs}/{id}; 404 = not favorite); pure `LibraryEdit.swift`; budget `CatalogBudget.libraryEdit`; B1+U1 advisories fixed (stderr line per Code=6 retry, named code, `PlaylistStart.checkRange`, tested `startRetryingOnce` seam). RED/GREEN: Swift 93 tests, Go all ok. Live read-only: `favorite` on catalog song → `{"favorite":false}` (user token works). GAP (needs user decision): MusicKit's local library ids (numeric, e.g. `7193945518659293268`) are unknown to the web API (404 playlists, 400 library-song ratings); only catalog songs and web ids (`i.…`, `p.…`) work today. Commit `f6080ed`. RDD: medium, 1105 lines, consent granted, lineage `review-373d78b0e9dbe345`, reliability lens, APPROVED, acknowledged (burned). Advisories → B3: unfavorite of an unrated song (404) must succeed; validate ids before interpolating into API paths (allow-list chars, no `/` or `..`); test the MusicDataRequest error mapping and favorite 404 path via a pure seam; timeout on non-idempotent writes should say the outcome is unknown.
- User decision (2026-09-29): read the library through the Apple Music web API (web ids + catalog ids) instead of local MusicKit ids.
- B3 done (route: delegated writer). `playlists` via `GET /v1/me/library/playlists` (pages of 100, cap 500, `editable`); `libraryPlaylist` via `…/{id}/tracks` (cap 1000; Song.ID = catalogId, else `i.…` + `LibraryOnly`); `playPlaylist` re-reads tracks and plays catalog ids through the playSongs path (batches of 300), mapping startIndex past library-only songs; local MusicLibraryRequest paths removed; pure `LibraryRead.swift`; budget `libraryRead = 2 * lookup`, Go `Playlists` on `detailCallTimeout`. B2 advisories (a)–(d) fixed incl. `APIPathID.checked` on every path id. UI: library-only tracks muted, enter shows a notice. RED/GREEN: Swift 116, Go all ok. Live read-only: 5 playlists (4 editable, "Canciones favoritas" not), Bach 15 tracks all with catalog ids, unsafe id rejected. Open: audio checks; large playlists may exceed the 6s play budget (cache later); README 10s vs 6s mismatch (pre-existing). Commit `7050a05`. RDD: medium, 1195 lines, consent granted, lineage `review-5ec5538dfcba81c1`, reliability lens, APPROVED, acknowledged (burned). Advisories → U2: paging stops early when a full page is entirely filtered out (music videos) — stop on API emptiness/next instead; playPlaylist re-reads pages inside the 6s playback budget — play library playlists from the ids the page already loaded; guard against a `next` link that repeats an offset.
- U2 done (route: delegated writer). `l` love/unlove, `a` add to playlist (selected song row, else the song playing; typed in SEARCH unless a song row is selected); `♥`/`♡` row controls + NOW PLAYING `╱ ♡ ╱`; favorites read lazily on the tick for selected/now-playing, cached, optimistic toggle; ADD TO PLAYLIST overlay (editable playlists + `+ NEW PLAYLIST`); NEW PLAYLIST inline input from the root row (cursor -1), nav button and picker; new playlist inserted locally and selected; library playlists play via `PlaySongs` from loaded ids (index mapped past library-only); Swift paging stops on raw API emptiness and repeated offsets. RED/GREEN: Swift 118; Go playback 4 + library_test build failure → all ok (parent spot check). Live checks pending (create, add, favorite, playback, paging with videos). Commit `92e37b9`. RDD: high, 1934 lines, consent granted, lineage `review-012abf6776c02e9e`, 4 lenses, APPROVED, acknowledged (burned). Advisories → U2f.

## Next step

U2f; then the user decides push/PRs (chain: fix/playback-start → feat/volume-keys-click → feat/library-edit-backend → feat/library-web → feat/library-edit-ui).
