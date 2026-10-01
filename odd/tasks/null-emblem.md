# Null emblem

## Objective
Adopt the null emblem logo (a block-drawn Ø next to the name, mockups https://claude.ai/artifact/GRZUpA6jV6zyPubxZYzCg9) chosen by the user on 2026-10-01, centered.

## Emblem (cells; mask r = ring/main color, s = slash/accent color)
Large (5 rows):
```
  ▄████▄▄▀   rrrrrrss
▄█▀   ▄▀█▄   rrr   srrr
██  ▄▀  ██   rr  ss  rr
▀█▄▀   ▄█▀   rrrs   rrr
▄▀▀████▀     ssrrrrrr
```
Compact (3 rows):
```
 ▄▀▀▄▀   rrrrs
█ ▄▀ █   r ss r
▄▀▄▄▀    srrrr
```
Text beside it, three spaces right: `N U 1 1` / `S I G N A L` (label bold) / `◢◤◢◤◢◤◢◤◢◤` (accent), vertically centered on the emblem (rows 2–4 of the large one, rows 1–3 of the compact one).

## Tasks
- [x] E1 — Startup screen: while Apple Music access is linking (auth not yet OK or failed), the body shows the large emblem with its text, the whole block centered horizontally and vertically, and under it, centered on the screen too, `L I N K I N G   A P P L E   M U S I C . . .` (muted); header, nav, status and footer stay. Compact emblem where the large one does not fit; nothing but the text where neither fits. Theme colors (ring label, slash accent), animated by the content intro like other content when it appears/disappears. Route: delegated writer.
- [x] E2 — `nu11signal --version` prints the compact emblem with the version beside it (plain text when stdout is not a terminal). README shows the large emblem at the top. Route: delegated writer (with E1).

## Acceptance criteria
- Logo block and LINKING line both centered on the screen width (user feedback: in the mockup the block sat left of the line's center).
- `go test -race ./...`, vet, gofmt clean; goldens added for the startup screen in both themes.

## Progress
- Created 2026-10-01 on branch `feat/null-emblem` from main 26d64de.
- 2026-10-01 E1+E2 implemented by the delegated writer (uncommitted): `internal/radio/emblem.go` (emblem data, masks, `splash`, `EmblemRows`), splash wired into `renderFull`/`renderCompact` while `auth == authPending` (KEYS/SETTINGS overlays still win), intro region for the splash body plus a startup intro on the first size; `--version` draws the compact emblem only when stdout is a character device (injected `deps.stdoutTerminal`), bare line otherwise; README top emblem. RED observed (compile, then 13 behavioral failures); GREEN: `go test -race ./...` 966 passed, vet/gofmt clean, `make test-scripts` 34 passed. New goldens `linking_80x24`, `linking_blue_80x24`; no existing golden changed. `TestStartupReadsTheVolume` now authorizes first (the splash has no volume row).
