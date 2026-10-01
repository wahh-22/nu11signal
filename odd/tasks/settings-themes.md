# Settings and themes

## Objective
A settings modal with a THEMES section, a new BLUE theme built on the gentleman-blue palette (~/.pi/agent/themes/gentleman-blue.json), the choice persisted; and replace the redundant nav-bar breadcrumb with invented flavor text in the app's style.

## Problem / why
- The nav-bar breadcrumb (`PLAYLISTS // TOCAYO`) repeats what the list panel already shows (user feedback 2026-09-30).
- Colors are fixed (Night City red/cyan/yellow); the user wants selectable themes, starting with a blue one.

## Palette (gentleman-blue)
background #05070F, surface #070B1A, userSurface #10182E, border #1C2C54, muted #4A5578, foreground #DBE9FF, primary #347AFF, cyan #5CE1FF, violet #7C5CFF, yellow #FFD23D, red #FF3D81, green #4DFF88, orange #FF9F1C.
Suggested role mapping for BLUE (app role → color): main/red role (titles, frames, labels) → primary #347AFF; frame deep → a darker primary; cyan → #5CE1FF; yellow highlight/active fill → #FFD23D; dim → #1C2C54; muted → #4A5578; select background → #10182E; ink on fills → #05070F; alerts that must read as danger (NO SIGNAL, failures) → #FF3D81.

## Tasks
- [x] T1 — Nav-bar right slot: invented cyberpunk flavor instead of the breadcrumb, e.g. a net node readout `NODE 7F // NC-GRID` (deterministic from the seed). Route: delegated writer (with T2).
- [x] T2 — Themes: palette data (NIGHT CITY default, BLUE), apply a theme to every style and the visualizer inks; settings modal (`s` outside typing; esc/s close; ↑↓ + enter to pick; live preview acceptable), THEMES section listing themes with the active one marked; persist the choice in config.json (`theme`), read at startup; keys/help/README updated. Route: delegated writer.

## Acceptance criteria
- Choosing BLUE recolors the whole UI (frames, text, buttons, rain, effects) and survives restart; NIGHT CITY unchanged by default (existing goldens unchanged except intended UI additions).
- Settings reachable by key and listed in the `?` overlay and footer where room allows.
- `go test -race ./...`, vet, gofmt clean.

## Delivery
Forecast ~500 lines; ask-on-risk. Branch `feat/player-hud` (continues the stack).

## Progress
- Created 2026-09-30.
- T1+T2 done (route: delegated writer; first worker interrupted by the user, second finished from its partial tree). `netNode()` → `NODE %02X // NC-GRID` from the seed. `theme` type (9 roles incl. alert), NIGHT CITY + BLUE; `applyTheme` rebuilds package styles, `vizInks`, `noiseStyles` (globals kept: View runs on the Update goroutine; tests not parallel, reset via t.Cleanup). SETTINGS overlay (settings.go): `s`, ↑↓, enter/click applies live, s/esc close. `config.Config.Theme` + `Source.Save` (atomic temp+rename, 0700/0600, unknown fields kept, refuses invalid JSON); startup applies saved theme, unknown → NIGHT CITY; failed save → `SETTINGS NOT SAVED`. RED: build failures on new API; GREEN: `go test -race ./...` pass, vet/gofmt clean. Goldens: new settings_80x24, view_blue_80x24 (ANSI), view_night_city_ansi_80x24 (byte-identical baseline); help gains S row; nav line changed in the rest.
- Review (slice 9a833d1..d04956b, incl. 1e963dd up-next clean): high, 1268 lines, consent granted, lineage `review-2b32f9457f07fde7`, 4 lenses, APPROVED, acknowledged (burned). Boundary now d04956b. Hardened after review (inline, test-first): concurrent theme saves ordered (`configSaves` sequence, an older save finishing last is dropped); `writeAtomic` writes through a symlinked config.json (dotfiles) instead of replacing the link. RED: both new tests failed; GREEN: `go test -race ./...` pass. Other advisories (not scheduled): theme role names (red role holds blue), Save's known-fields list, pre-save copy of unknown fields, alert comment wording.
