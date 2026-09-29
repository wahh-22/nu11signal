# Feature: playback-polish

Locator: `odd/tasks/playback-polish.md` · Engram mirror: `odd/playback-polish/tasks` · Branches (stacked on `feat/library-edit-ui`, then to main): `fix/prepare-repeat` → `feat/queue-loop-ui`

## Objective

Fifth round of user feedback (2026-09-29, testing `feat/library-edit-ui`).

## Problem / Why

1. Playing a specific song opened from search does not continue with the rest of that list; it should queue the rest of the list it was picked from.
2. Loop: repeat the list or the song.
3. Volume is read from the system; the user wants an independent app volume (PENDING decision: macOS has no per-app volume API for MusicKit playback).
4. Duplicate buttons (e.g. NEW PLAYLIST in the nav and as a row): one control must serve keyboard and mouse.
5. Loved hearts sometimes take long to appear the first time (per-song lazy reads on the tick).
6. `MPMusicPlayerControllerErrorDomain Code=6 "failed to prepare to play"` still happens, mostly for searched songs and some albums (e.g. Andrés Cepeda — Bogotá (Deluxe), 2025, track 4 "Prométeme"); NOW PLAYING stays STANDBY.

## Tasks

- [ ] V1 — Backend: diagnose and fix "failed to prepare to play" (inspect catalog songs' playParameters/availability read-only; drop unplayable items from queues, fall back gracefully, report clearly); repeat mode (`setRepeat` off/all/one via `ApplicationMusicPlayer.state.repeatMode`, reported in state); batch favorites read (`GET /v1/me/ratings/songs?ids=…`). Branch `fix/prepare-repeat`. Route: delegated.
- [ ] V2 — UI: play the rest of the list from any song row (results, top songs, album, playlist, search rows); loop control; prefetch favorites for a page in one batch; single controls for keyboard + mouse (remove mouse-only duplicates). Branch `feat/queue-loop-ui`. Route: delegated.

## Constraints

- No audio and no library writes in automated live checks. Artifacts in English.

## Progress

- Branch `fix/prepare-repeat` from `feat/library-edit-ui` `c036721` (not yet merged to main).

## Next step

V1; ask the user about the independent volume.
