# Quit confirmation

## Objective
Closing (or trying to close) nu11signal asks for confirmation in a modal, so a stray `q` never ends the session (user request 2026-10-01).

## Behavior
- `q` (where it quits today) and `ctrl+c` open a small centered modal in the panel style: `QUIT NU11SIGNAL?` with `[Y/ENTER] QUIT` and `[N/ESC] STAY`; playback keeps going while it is open.
- In the modal: `y` or `enter` quits; `n` or `esc` closes it; pressing `q` or `ctrl+c` again also quits (a double press is the fast way out). Other keys are ignored. A click outside closes it; clickable QUIT/STAY buttons if cheap with the zone machinery.
- Works over every view, including KEYS/SETTINGS overlays, the startup splash and the auth error screen. Typing contexts keep `q` typed (only ctrl+c opens it there, as today).
- Animates in/out with the content intro like the other overlays; footer shows the modal's keys while open; KEYS overlay and README updated.

## Tasks
- [x] Q1 — Quit confirmation modal. Route: delegated writer (2+ non-trivial files: quit.go, update/view/intro/mouse wiring, tests, README).

## Acceptance criteria
- No path quits without confirmation except the double press inside the modal.
- `go test -race ./...`, vet, gofmt clean; golden for the modal at 80x24.

## Progress
- Created 2026-10-01 on branch `feat/quit-confirm` from main 6095ff3.
- Q1 implemented (uncommitted): `internal/radio/quit.go` (modal state, keys, clicks, panel, overlay), wired in `handleKey` (q is caught before any area takes it, so focus and view stay put), `handleMouse`, `baseLayout`, `hintLine`, `withIntro`/`introRegions`. Esc on the auth error screen also asks now. KEYS label `QUIT? (CTRL+C IN TEXT)`; README keys tables and a quit paragraph.
- Checks: RED observed (12 new tests failing on a stub), GREEN `go test -race ./...` 1005 passed; build, vet, gofmt clean. New golden `quit_80x24`; `help_80x24` and `night_city_ansi_views_80x24` changed only in the KEYS quit label text (escape sequences identical).
