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

- [x] H2 — Favorite mark: user chose ASCII "pulse" `<3` (favorite) / `--` (not favorite), two cells, everywhere a heart appears (player title line as a bracket button `[<3]`/`[--]`, track lists, any other ♥/♡ use). Route: delegated writer (with H3).
- [x] H3 — Volume on the transport row when it fits: `[◀◀]  [ ❚❚ PAUSE ]  [▶▶]  [↻ OFF]  VOL [−] ▮▮▮▮ [+] 90%  [⤢]`, responsive: one row when wide enough, else the current two rows. Route: delegated writer (with H2).

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
- H2+H3 done (route: delegated writer), commit `061e8f1`. `favoriteMark(on)` → `<3`/`--` (2 cells); player `[<3]`/`[--]`; list actions block 6 cells (` <3 + `), fav zone 3 cells; status `<3 LOVED` / `-- UNLOVED`. `hudRowCount(w)` (one row from `hudOneRowMin` = 61 cells) drives drawing (`hudControls`), `nowPlayingControlRows` and focus (`besideControl`, `volumeBelow` follow drawn zones). README updated. RED: 5 behavior failures with stubs; GREEN: `go test -race ./...` 894 passed; 18 goldens regenerated.
- Review (slice e09a7d6..061e8f1): high, 692 lines, consent granted, lineage `review-e15d4c175084e9a3`, 4 lenses, APPROVED, acknowledged (burned). Boundary now 061e8f1. Hardened after review: negative padding guarded with max(…, 0) in hudControls (R4). Other advisories (not scheduled): HUD width literals → derive from button widths; onVolumeRow name; compact focus walk untested; vestigial test loop; volW +1.
- [x] H4 — Rain fills every row under the controls (the 12-row cap `eqMaxRows` removed). Route: inline (one constant + vizRows). RED: `TestRainFillsTheRoomUnderTheControls` h=40: 12 rows, want 23; GREEN: `go test -race ./...` 895 passed, vet/gofmt clean; golden artist_about_120x40 regenerated. Commit after 71fbadd; assess pending in slice.
- [x] H5 — User feedback 2026-10-01: the spaced `N O W   P L A Y I N G` head row repeats the panel's frame label. Replace it: the head row shows the feed (`▌091.3 MHZ // TOCAYO`) on the left and the play status (`▮ PLAYING`/`▮ PAUSED`) on the right; the separate feed row under the progress bar goes away (the rain gains a row). Route: delegated writer. Done: `npFeedRow` removed; head = `feedLine()` cut with `…` to `headFeedWidth(iw)` + status tag; `nowPlayingControlRows` 11/10 → 10/9 (rain +1 row); intro compares the head row's feed cells only (`introFieldRows` = artist, album, head; head span capped by `headFeedWidth`, so a status change never scrambles). RED: `TestNowPlayingHeadIsTheFeedAndTheStatus`, `TestNarrowNowPlayingCutsTheFeedNotTheStatus`, `TestANewFeedIntrosOnTheHeadRow`, `TestControlRowsFollowTheHUDLayout` (10/9) failed; GREEN: `go test -race ./...` ok, vet/gofmt clean; all 21 goldens regenerated (ANSI baselines included). Commit pending.
- H5 commit `2da2e43`. Review (slice 14101fe..2da2e43, incl. 39769aa BLUE eight colors): medium, 601 lines, consent granted, consolidated lens, lineage `review-c67587340ecd2820`, APPROVED, acknowledged (burned). Boundary now 2da2e43. Advisory: in the narrow panel the feed is always cut by the status width.
- Open bug 2026-10-01: `PLAY FAILED // HELPER PLAYSONGS: TIMED OUT AFTER 6.0S`; the 09:32 session logged no playSongs line (stuck before planning: catalog/library lookup, or queued behind another command on the PlaybackChain). Awaiting the user's repro details.
