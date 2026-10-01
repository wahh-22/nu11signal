# HUD info

## Objective
Replace decorative HUD texts with meaningful information, and make every keyboard shortcut discoverable.

## Problem / why
- `RDO-77 // NC-NET` (nav bar right), `NC-NET 0x2077` (NOW PLAYING bottom label) and `BUF 0x5EF6` (idle status line) are fixed decoration.
- The footer shows a per-view priority list and drops entries when narrow; several keys never appear in some views (e.g. on track pages: `,/.` SEEK, `J/K` VOL, `F` EXPAND, `O` LOOP, `X` FX, `→` PLAYER, `TAB`), and some never appear anywhere (`shift+←/→`, `shift+↑/↓`, `R` retry, `DEL`/`ctrl+d` delete recent search, `ctrl+f`, `G` outside results).

## Scope
- I1 labels: nav bar right → breadcrumb of where the user is (e.g. `PLAYLISTS // TOCAYO`, `SEARCH // RESULTS`), keeping the cyberpunk frame style; NOW PLAYING bottom label → spectrum source (`SPECTRUM LIVE` with real readings, `SPECTRUM SIM` synthetic, `SPECTRUM HOLD` paused); idle status line → `UP NEXT // <title> · <artist>` when the next song of the on-air list is known, else volume source + FX state (`APP VOLUME // FX ON`).
- I2 keys: a `?` help overlay listing every shortcut grouped (Playback, Navigation, Library, Search, Player focus, App), closed with `?`/esc; the footer always keeps `[?] KEYS` next to QUIT.

## Constraints
Deterministic rendering (goldens), no new polling/timers, typing contexts keep `?` typed in SEARCH/name inputs (help reachable via another route there only if cheap), narrow widths degrade. Conventional commits.

## Tasks
- [x] I1 — Meaningful labels. Route: delegated writer (with I2).
- [x] I2 — `?` help overlay + `[?] KEYS` hint. Route: delegated writer (with I1).

## Acceptance criteria
- No fixed decorative code strings remain in those three places; each label reflects real state.
- Every binding in keys.go appears in the help overlay (test enumerates bindings vs overlay).
- `go test -race ./...`, vet, gofmt clean.

## Delivery
Forecast ~450 lines; ask-on-risk. Branch `feat/player-hud` (continues the stack).

## Progress
- Created 2026-09-30.
- I1+I2 done (route: delegated writer). Breadcrumb `breadcrumb()` (lit tab + editor/page title, cut with …, dropped under 6 cells); `spectrumCode()` LIVE/SIM/HOLD in both panels; `onAirQueue` recorded from each play (minus missing/skipped, none if started alone), `upNext()` only when the playing song appears once; last song wraps only with loop ALL/ONE (natural end stops otherwise); fallback `APP|SYS VOLUME // FX ON|OFF`. `?` overlay (help.go, `helpGroups` single table, `keyHelp`), typed in SEARCH/name inputs, esc/? close, only ctrl+c while open; footer keeps `[?] KEYS` before quit, `shortHints` shortens SPACE/←→ labels when narrow. RED: 12 failures with stubs; GREEN: `go test -race ./...` pass (radio 711), vet/gofmt clean; 18 goldens + new help_80x24. README updated. TestTheHelpListsEveryKey parses keys.go.
- Review (slice 061e8f1..9a833d1, incl. 71fbadd padding guard + 41d90e1 full-height rain): high, 1180 lines, consent granted, lineage `review-688b595a32a5e6aa`, 4 lenses, APPROVED, acknowledged (burned). Boundary now 9a833d1. Hardened after review: UP NEXT title/artist pass through cleanLine (R3). Other advisories (not scheduled): shortHints literal label matching; mouse close of overlay untested; upNext edge cases (duplicates, missing) partially tested.
