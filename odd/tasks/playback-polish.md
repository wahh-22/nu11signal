# Feature: playback-polish

Locator: `odd/tasks/playback-polish.md` · Engram mirror: `odd/playback-polish/tasks` · Branches (stacked on `feat/library-edit-ui`, then to main): `fix/prepare-repeat` → `feat/queue-loop-ui`

## Objective

Fifth round of user feedback (2026-09-29, testing `feat/library-edit-ui`).

## Problem / Why

1. Playing a specific song opened from search does not continue with the rest of that list; it should queue the rest of the list it was picked from.
2. Loop: repeat the list or the song.
3. Volume is read from the system; the user wants an independent app volume. Decision: Core Audio tap (V3).
4. Duplicate buttons (e.g. NEW PLAYLIST in the nav and as a row): one control must serve keyboard and mouse.
5. Loved hearts sometimes take long to appear the first time (per-song lazy reads on the tick).
6. `MPMusicPlayerControllerErrorDomain Code=6 "failed to prepare to play"` still happens, mostly for searched songs and some albums (e.g. Andrés Cepeda — Bogotá (Deluxe), 2025, track 4 "Prométeme"); NOW PLAYING stays STANDBY.

## Tasks

- [x] V1 — Backend: diagnose and fix "failed to prepare to play" (inspect catalog songs' playParameters/availability read-only; drop unplayable items from queues, fall back gracefully, report clearly); repeat mode (`setRepeat` off/all/one via `ApplicationMusicPlayer.state.repeatMode`, reported in state); batch favorites read (`GET /v1/me/ratings/songs?ids=…`). Branch `fix/prepare-repeat`. Route: delegated.
- [ ] V3 — Independent volume via Core Audio process tap (user decision 2026-09-29, accepting the private `responsibility_spawnattrs_setdisclaim` launch): helper re-launches itself disclaimed (or the Go spawn does), permission UX with fallback to system volume, tap on the RemotePlayerService copy that serves the helper, aggregate device + RT-safe gain ramp, lifecycle (stop on pause, rebuild on output/service changes), measure CPU/memory/wakeups with music playing (user consented) and report before merge. Branch `feat/app-volume`. Route: delegated.
- [x] V2 — UI: play the rest of the list from any song row (results, top songs, album, playlist, search rows); loop control; prefetch favorites for a page in one batch; single controls for keyboard + mouse (remove mouse-only duplicates). Branch `feat/queue-loop-ui`. Route: delegated.

## Constraints

- No audio and no library writes in automated live checks. Artifacts in English.

## Progress

- Branch `fix/prepare-repeat` from `feat/library-edit-ui` `c036721` (not yet merged to main).
- V1 done (route: delegated writer). Bug 6 root cause (reproduced with `prepareToPlay`, no audio): a multi-song queue whose start is a catalog song fails to prepare if it contains any song stored in this Mac's local library ("Para Qué", "Tú"); it prepares when the start is the library copy. Fix: pure `PreparedQueue` — if the picked song is local, use library copies for local songs; otherwise skip local songs (reported `skipped`); on any Code=6 fall back to the picked song alone (`startedAlone`) with a clear error if that fails. Repeat: `SetRepeat` off/all/one + `State.Repeat` (serialized playback command). Batch favorites: `Favorites(ctx, ids)` via `GET /v1/me/ratings/{songs|library-songs}?ids=` (100 per request). RED/GREEN: Swift 133, Go 649. Live read-only checks OK (album plays with skipped "Para Qué"; favorites read). Open: skipped/startedAlone not surfaced to Go; skipping loved local songs is a trade-off.
- Volume spike (isolated worktree, branch `spike/audio-tap`, commit `053a522`, user consented to audio + permission): MusicKit audio renders in `com.apple.MediaPlayer.RemotePlayerService`; a muted Core Audio process tap captures real audio and gain re-render works (0.3× measured); TCC is attributed to the terminal unless the helper is spawned with private `responsibility_spawnattrs_setdisclaim`. Integration ~300–500 lines; decision pending with the user.
  - V1 commit `58c338a`. RDD: medium, 961 lines, consent granted, lineage `review-085e2e653967ddf5`, reliability lens, APPROVED, acknowledged (burned). Advisories → V2: library-copy entries report `i.…` ids in state (map back to the requested catalog id); malformed ratings body must fail, not report all unloved; unknown wire repeat mode → off; flaky demo RepeatOne test wait.
- V2 done (route: delegated writer). Enter on SEARCH/RESULTS song rows now plays and queues that list (TOP RESULTS song → SONGS section); `g` opens the SONG view; helper appends the following songs after a lone-start fallback (`queue.insert(.tail)`); LOOP key `o` (OFF→ALL→ONE) + focusable `╱ ↻ LOOP … ╱` row; page-level favorites prefetch in one `Favorites` call; nav `+ NEW PLAYLIST` removed (row only), NOW PLAYING ♡ made keyboard-focusable; V1 advisories fixed (catalog id reported for library copies, malformed ratings fail, unknown repeat → off, deterministic demo test). RED/GREEN: Go 370, Swift 137. Audio checks pending.

## Next step

Review V2, then V3 on `feat/app-volume`.
