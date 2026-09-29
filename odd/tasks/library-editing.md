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

- [ ] B1 — Playback start fixes (helper + adapter): honor the start track for library playlists; make `playSongs` robust to Code=6 (root cause first: start item not in the prepared queue / id mismatch / race; fix or retry once, never loop). Branch `fix/playback-start`. Route: delegated (isolated worktree, parallel with U1).
- [ ] U1 — Volume on j/k and click-to-focus panels (internal/radio). Branch `feat/volume-keys-click`. Route: delegated (parallel with B1).
- [ ] B2 — Library editing backend: verify MusicKit macOS availability for `MusicLibrary` createPlaylist / edit (rename, add/remove items) and for favorites (love rating via Apple Music API `me/ratings` or a MusicKit API); port + helper + adapters + fake + demo. Branch `feat/library-edit-backend`. Route: delegated.
- [ ] U2 — Library editing UI: new playlist, rename, add song to a playlist (picker), remove track, favorite toggle; keyboard + mouse. Branch `feat/library-edit-ui`. Route: delegated.

## Constraints

- Never play audio in automated smoke tests without the user's consent; live read-only commands are fine.
- Artifacts in English.

## Delivery

- Strategy `ask-on-risk`, chain `stacked-to-main`. Forecast ~2000 lines.

## Progress

- Branch `fix/playback-start` from `main` `bc4f8b5`.

## Next step

B1 and U1 in parallel.
