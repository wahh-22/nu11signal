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
- [x] H1 — Implement the HUD layout (buttons, transport row with LOOP and right-aligned EXPAND, volume row), focus/zones/mouse updated, goldens regenerated. Route: delegated writer (several non-trivial radio files).

- [ ] H2 — Favorite mark: user chose ASCII "pulse" `<3` (favorite) / `--` (not favorite), two cells, everywhere a heart appears (player title line as a bracket button `[<3]`/`[--]`, track lists, any other ♥/♡ use). Route: delegated writer (with H3).
- [ ] H3 — Volume on the transport row when it fits: `[◀◀]  [ ❚❚ PAUSE ]  [▶▶]  [↻ OFF]  VOL [−] ▮▮▮▮ [+] 90%  [⤢]`, responsive: one row when wide enough, else the current two rows. Route: delegated writer (with H2).

## Acceptance criteria
- Layout matches the target at ~60 cells; degrades cleanly at narrow widths without overflow.
- Every control still works by mouse and keyboard; focus moves sensibly between the two rows.
- `go test -race ./...`, vet, gofmt clean.

## Delivery
Forecast ~350 lines; ask-on-risk. Branch `feat/player-hud` stacked on `fix/local-queue`.

## Progress
- Created 2026-09-30.
- H1 done (route: delegated writer), commit `e09a7d6`. Bracket style for player buttons only; transport row tries 5 layouts (label → `[ ❚❚ ]` → tighter gaps → `[❚❚]` → `[↻]`), EXPAND right-aligned unless packed; LOOP `[↻ OFF|ALL|ONE]` on transport row (loopBar/loopTail/loopButton → loopName); volume `VOL [−] meter≤20 [+] 90%`; control order Prev, Play, Next, Loop, Expand, VolDown, VolUp, Fav; nowPlayingControlRows 12/11 → 11/10 (rain +1 row); compact layout packs LOOP into transport. RED: signature + 4 behavior failures; GREEN: `go test -race ./...` 887 passed, vet/gofmt clean; all 18 goldens regenerated.
- Review (slice 41fff01..e09a7d6, incl. local-queue Q5 7aedba1): medium, 1028 lines, consent granted, consolidated lens, APPROVED, acknowledged (burned). Boundary now e09a7d6. Advisory: pressed NEXT on whole-list with repeat all; → stops at last drawn button untested.
- Open: ♥ on the title line still slanted `╱ ♡ ╱` (maybe `[♡]`); PLAY/PAUSE width differs by one cell (buttons after it shift, as before).
