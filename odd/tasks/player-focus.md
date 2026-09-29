# Feature: player-focus

Locator: `odd/tasks/player-focus.md` · Engram mirror: `odd/player-focus/tasks` · Branches (stacked to main): `feat/recent-delete` → `feat/player-focus`

## Objective

Second round of user feedback (2026-09-29): delete recent searches; keep the left panel at its maximum width; let the player panel expand to full width; and make the player's buttons reachable with the keyboard cursor, moving focus between the left list and the right player.

## Scope

- P1 — Delete recent searches: `Recents` gains `Remove(term)` and `Clear()`; SEARCH RECENT rows get a clickable `✕`, the selected recent row is deleted with `ctrl+d`/`delete`, and a `CLEAR RECENT` action (keyboard row + mouse button) empties the list; persisted by the file adapter.
- P2 — Layout and focus:
  - Left panel always uses its maximum width (the current page width) in every view.
  - Player expand: a toggle (key + `EXPAND` button) makes NOW PLAYING take the full width (list hidden); toggle back restores.
  - Focus: `←`/`→` move focus between the list and the player (user choice); inside the player `←`/`→` walk the buttons (PREV, PLAY/PAUSE, NEXT, EXPAND), `↑` reaches the progress bar where `←`/`→` seek, `↓` returns to the buttons; `enter` activates the focused control; focused control is highlighted in the neon style. Seeking moves to `shift+←/→` and `,`/`.` everywhere else.

## Constraints

- Keyboard and mouse stay equivalent; typing in the SEARCH input is never hijacked (arrows edit the input while it has focus).
- Compact/tiny layouts must not break; cell widths only.
- Artifacts in English.

## Delivery

- Strategy: `ask-on-risk`; chain `stacked-to-main` (as before). Forecast ~900 lines (P1 ~300, P2 ~600).

## Tasks

- [x] P1 — Delete recent searches. Branch `feat/recent-delete`. Route: delegated (writer trigger: 2+ non-trivial files).
- [x] P2f — P2 review follow-ups: returning from the player refocuses the SEARCH input while a row is selected (R3 WARNING); focusList/focusPlayer constant vs method name clash (R2 WARNING); expand toggle duplicated (keyboard vs button focus differs); dead EXPAND tone guard; fragile hint patching; onBar not cleared when unseekable; test → on pages; stations footer lost [/] SCAN and [ENTER] TUNE. Branch `feat/player-focus`. Route: delegated.
- [x] P2 — Max-width list, expandable player, keyboard focus across panels. Branch `feat/player-focus`. Route: delegated.

## Acceptance criteria

- A recent term can be removed individually (keyboard and mouse) and all can be cleared; the change persists across runs.
- The left panel keeps the same (maximum) width in stations, SEARCH and pages.
- The player can be expanded to full width and restored (keyboard and mouse).
- From the list, `→` focuses the player; buttons are reachable and activatable with the keyboard; `←` returns to the list.

## Checks

- `go test -race ./...`, `go vet ./...`, `gofmt -l .`.
- Manual: `make build && ./bin/nu11signal`.

## Progress

- Branch `feat/recent-delete` from `main` `73573e1`. Key decision (user): arrows move focus; seek moves to `shift+←/→` and `,`/`.`.
- P1 done (route: delegated writer). RED: history build failure then 7 radio behaviour failures (✕ click, clear button/row, delete keys, cursor after delete, failure path, hints). GREEN: `go test -race ./...` ok (parent spot check), `go vet`/`gofmt` clean. Clear writes `{"terms":[]}`; per-row ✕ zone overrides the row click; `[DEL] DROP` hint. Open: no undo for CLEAR RECENT (suggest `u` undo later); store command ordering and late initial load are pre-existing races. Commit `8d37005`. RDD: high, 614 lines, consent granted, lineage `review-b11c0aee27b3fc03`, 4 lenses, APPROVED, acknowledged (burned); boundary → `8d37005`. Advisories (folded into P2): serialize recents store writes (Remove/Clear vs Add ordering, R4 WARNING); name the ✕ cell width once (R2 WARNING); deleteRecentAt comment vs last-row behavior; CLEAR RECENT label/style shared; `[DEL] DROP` hint only when a recent row is selected (and mention ctrl+d if it fits); clear-button test asserts on rendered header.
- P2 done (route: delegated writer). RED: 24 failing tests (widths, expand key/button/compact, focus walking, bar seek, enter, esc, input arrows, seek keys, clicks, hints, recents order) + missing goldens. GREEN: `go test -race ./...` 434 passed (parent spot check), `go vet`/`gofmt` clean. Decisions: list width `max(28, min(w*3/5, 72, w-31))` in every view (player keeps ≥30 cols; glyph-only buttons at 80 cols); expand `ctrl+f` everywhere + `f` outside the input + ⤢/⤡ button; focus rules per README; lit ▮ / dim ▯ panel frames; seek `shift+←/→` and `,`/`.`; recents writes serialized by an in-order queue (`recents.go`). P1 advisories (a)–(f) fixed. Open: page/SEARCH hints omit `→ PLAYER`; late first recents load unordered; no CLEAR undo; no manual run yet. Commit `a33ec03`. RDD: high, 1380 lines, consent granted, lineage `review-43afff6ce2be984a`, 4 lenses, APPROVED, acknowledged (burned); boundary → `a33ec03`. Advisories → P2f.
- P2f done (route: delegated writer). Item 1 deviation accepted by parent: SEARCH keeps the input focused while a row is selected (typing always edits the term), so returning from the player now restores the exact prior input focus (`inputHadFocus`) instead of forcing or blurring it; pinned by `TestReturningFromThePlayerRestoresTheSearchAsItWas`. Items 2–8 fixed (areaList/areaPlayer, single toggleExpand path, declarative player hints, onBar cleared when unseekable, → from every page tested, stations footer priority `[ENTER] TUNE  [/] SCAN  [SPACE] PLAY/PAUSE  [→] PLAYER  [,/.] SEEK  [Q] QUIT`). RED: bar fallback and footer tests; GREEN: `go test -race ./...` 455 passed (parent spot check), `go vet`/`gofmt` clean. Commit `76691d4`. RDD: high, 374 lines, consent granted, lineage `review-367d078fb5b66941`, 4 lenses, APPROVED, acknowledged (burned); boundary → `76691d4`.
- P2g (route: inline, one mechanical guard + test): P2f review WARNING — focusList refocused the input from `inputHadFocus` even when SEARCH was no longer on top; restored the `viewSearch` guard and always clear the flag. RED: `TestReturningFromThePlayerOffSearchLeavesTheInputAlone` failed on the old code (`input focused true`); GREEN with the fix; `go test -race ./...` ok. Remaining advisories (not scheduled): `typing` param name in playerFocusHints, expand-from-bar test does not assert onBar. Commit `e848349` reviewed anyway (final slice): lineage `review-62c4b7d22e0d649d`, reliability lens, APPROVED, acknowledged (burned); advisory: the regression test reaches the state by truncating the stack, not a user action (the guard is defensive).

## Next step

All tasks done. The user decides push/PRs (chain: feat/recent-delete → feat/player-focus).
