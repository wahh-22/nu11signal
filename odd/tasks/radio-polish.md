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

- [ ] T1 — Radio polish (internal/radio only): ▶ follows the now-playing track in track pages (the searched song only sets the initial cursor); BACK only when a page is above SEARCH/PLAYLISTS root; remove CLEAR RECENT (keep per-row ✕ and ctrl+d/delete); nav-tab focus area (↑ from the top of any list, from the SEARCH input and from the player's progress bar; ←/→ across tabs; enter; ↓ back). Branch `fix/radio-polish`. Route: delegated (writer trigger).
- [ ] T2 — Player backend (parallel, isolated worktree): volume get/set (MusicKit player volume if the SDK has one; otherwise system output volume via CoreAudio in the helper), `libraryPlaylist(playlistId)` returning tracks, and playing a library playlist starting at a track; port + adapters + fake + demo + tests. Branch `feat/player-backend`. Route: delegated.
- [ ] T3 — PLAYLISTS view + volume UI: rename STATIONS→PLAYLISTS, enter opens the library playlist's track view (enter on a track plays the playlist from it), volume keys (`+`/`-` outside the input), VOL−/VOL+ focusable player buttons and a volume readout. Branch `feat/playlists-volume`. Route: delegated.

## Constraints

- Keyboard and mouse stay equivalent; typing in the SEARCH input is never hijacked except ↑ from the input (explicit user request).
- Artifacts in English.

## Delivery

- Strategy `ask-on-risk`, chain `stacked-to-main`. Forecast ~1500 lines.

## Checks

- `go test -race ./...`, `go vet ./...`, `gofmt -l .`; `swift build` + `swift test` for helper changes; live helper smoke (no audio) for new commands.

## Progress

- Branch `fix/radio-polish` from `main` `a887d31`.

## Next step

T1 and T2 in parallel.
