# Player HUD

## Objective
Cleaner NOW PLAYING controls in the cyberpunk style: user picked option D ("HUD con corchetes") from the mockups at https://claude.ai/artifact/4FeawSCVr6kzSXfaasMxwG.

## Problem / why
Current controls: doubled slanted separators (`╱ ╱`), icon and text saying the same (PREV/PAUSE/NEXT), three rows with LOOP alone, EXPAND mixed with transport, PLAY/PAUSE with no more weight than the rest.

## Target (≈60 cells)
```
[◀◀]  [ ❚❚ PAUSE ]  [▶▶]      [↻ OFF]              [⤢]
VOL [−] ▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▯▯ [+] 90%
```
- Bracket buttons; only PLAY/PAUSE keeps its label and is filled yellow (primary). PREV/NEXT/EXPAND/RESTORE icon only; LOOP icon + short mode (OFF/ALL/ONE).
- Two rows: transport row (PREV, PLAY, NEXT, then LOOP, EXPAND right-aligned) and volume row (VOL, [−], meter as wide as fits, [+], percent).
- Tabs (PLAYLISTS/SEARCH/BACK) keep their slanted style (scope: player only).

## Constraints
Keep: clickable zones, keyboard focus (focused button filled yellow with ▸), focus navigation (arrows between rows), narrow-width degradation (drop PLAY label, then less), expanded player layout, intro regions (`np*Row` constants), status/hints. Conventional commits.

## Tasks
- [ ] H1 — Implement the HUD layout (buttons, transport row with LOOP and right-aligned EXPAND, volume row), focus/zones/mouse updated, goldens regenerated. Route: delegated writer (several non-trivial radio files).

## Acceptance criteria
- Layout matches the target at ~60 cells; degrades cleanly at narrow widths without overflow.
- Every control still works by mouse and keyboard; focus moves sensibly between the two rows.
- `go test -race ./...`, vet, gofmt clean.

## Delivery
Forecast ~350 lines; ask-on-risk. Branch `feat/player-hud` stacked on `fix/local-queue`.

## Progress
- Created 2026-09-30.
