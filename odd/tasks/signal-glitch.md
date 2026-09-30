# Feature: signal-glitch

Locator: `odd/tasks/signal-glitch.md` · Engram mirror: `odd/signal-glitch/tasks` · Branch: `feat/signal-glitch`

## Objective

More Cyberpunk 2077 atmosphere (user request, 2026-09-30): the screen recurrently seems to lose signal, numbers and letters appear, and alert signals show — without hurting usability or resources.

## Scope

- Recurrent signal loss: every ~20–45 s (seeded random), a 0.2–0.6 s glitch burst: horizontal line shifts/tears, a few corrupted cells, static bars; occasionally a brief (<1 s) `NO SIGNAL` flash. Never longer.
- Data rain: subtle changing hex/codes/coordinates in free space only (header, borders, empty panel areas), never over readable content.
- Alerts: rotating Night City style alerts on the status line with a blinking `▲` (e.g. `▲ SIGNAL DEGRADED // RETUNING`, `▲ ICE TRACE DETECTED`, `▲ PACKET LOSS 37%`), yielding to real status messages.
- Controls: a key to toggle effects and a `--calm` launch flag (and demo keeps working); effects pause while typing in the SEARCH input.
- Resources: reuse the existing animation tick; only raise the frame rate during bursts; measure CPU.

## Constraints

- Deterministic under tests (seeded, injectable clock); goldens stay stable (effects off or seeded in goldens).
- Cell-width safe glyphs; readability first. Artifacts in English.

## Tasks

- [x] G1 — Glitch engine + data rain + alerts + controls. Route: delegated (writer trigger: 2+ non-trivial files).

## Checks

- `go test -race ./...`, `go vet ./...`, `gofmt -l .`; CPU sample of `bin/nu11signal --demo`.

## Progress

- Branch `feat/signal-glitch` from `main` `ddf3b86`.
- G1 done (route: delegated writer). Bursts every 20–45 s lasting 0.2–0.6 s (row tears, noise cells, static bar; 1 in 4 flash `N O   S I G N A L`), data rain in free space only (long `─` runs, header gap, blank NOW PLAYING rows, empty rows below the list), alerts every 30–60 s for 4 s with blinking `▲` yielding to real status; toggle `x`, `--calm` / `NU11SIGNAL_CALM=1`; effects off by default in `radio.Options` (binary opts in), paused while typing / tiny layout; zones from `baseLayout()` unchanged. RED: vet unknown field `Effects`/`calm`; GREEN: `go test -race ./...` ok (parent spot check), `go vet`/`gofmt` clean. CPU (60 s demo, 80x24): idle ~0.5–0.6% either way; playing 0.6% calm vs ~1.0% effects on.
- G1 review: commit `20c6f0c`, RDD high, 1018 lines, consent granted, lineage `review-f5975847bbaebc01`, 4 lenses, APPROVED, acknowledged (burned).
- G1f (route: inline, one file + tests): rain's trailing-rows scan could index past a short frame (RED: panic index out of range [20] with length 12) → bounded by the frame; bursts spare the status and hint lines while a real status shows (RED: status line torn). GREEN: `go test -race ./...` 718 passed, `go vet`/`gofmt` clean. Not scheduled: name the layout offsets in rain, `blank` naming, rain x salt, precedence parentheses, `calmEnv` in the test, a test that `x` types in the inputs.
- G1f review: lineage `review-066712dbecf9665c`, reliability lens, APPROVED, acknowledged (burned); the status-sparing test now also asserts the burst is active and draws on the rest of the frame.

## Round 2 (user feedback, 2026-09-30)

The user did not want new letters/codes in the background. They want the song-title change glitch (existing text scrambling and resolving) applied to EXISTING text, randomly and very often across the whole TUI, while keeping the periodic whole-screen burst.

- [x] G2 — Remove data rain; add frequent micro-glitches that scramble random spans of existing visible text (anywhere: titles, rows, labels, buttons, player, footer) and resolve back like the title glitch; keep bursts and alerts; spare the SEARCH input while typing and real status; measure CPU. Branch `feat/text-glitch`. Route: delegated.
- G2 done (route: delegated writer). Data rain removed; micro-glitches scramble 1–3 spans (3–12 existing text cells, borders/blank excluded, wide chars never split) every 0.5–2 s for 150–300 ms, resolving left to right with the title-glitch glyphs; status line spared while a real status shows; tick 100 ms only while a glitch resolves idle, rides the 10 fps playing tick. RED: undefined micro fields; GREEN: `go test -race ./...` 721 passed, `go vet`/`gofmt` clean. CPU (60 s demo, 80x24): idle calm 0.8–0.9% vs effects 2.8%; playing noisy (calm 2.7–4.95%, effects 5.8%). Defaults tuned from 0.3–1.5 s / 80 ms to stay under ~3% idle.
- G2 review: commit `49850af`, RDD medium, 673 lines, consent granted, lineage `review-606c1f312372b324`, reliability lens, APPROVED, acknowledged (burned). Suggestions (not scheduled): wake for the next alert; guard the idle-interval test loop; progress-bar/geometric glyphs count as text.

## Round 3 (user feedback, 2026-09-30)

- [x] G3 — Scrambled cells keep the original cell's color/style (no cyan/yellow glitch style); micro-glitches more frequent (new one every ~0.2–0.8 s) with fewer animation frames per glitch to limit CPU; measure CPU. Branch `feat/glitch-tune`. Route: delegated.
- G3 done (route: delegated writer). `setCells` swaps only printable runes via `ansi.DecodeSequence`, so scrambled cells keep their original SGR (bursts keep their neon noise by design); micro-glitch gap 0.2–0.8 s, never overlapping the previous one's resolve; idle glitches take 3 frames (start, midpoint, end). RED: 4 failing tests (style skeleton, frequency, frame count, tick). GREEN: `go test -race ./...` 724 passed, `go vet`/`gofmt` clean. CPU (60 s demo, 80x24): idle 1.80% effects vs 0.83% calm; playing 3.70% vs 2.74%.
- G3 review: commit `f8657d2`, lineage `review-9689659166c023ed`, reliability lens, APPROVED, acknowledged (burned). Advisory (not scheduled): the idle mid-glitch wait isn't clamped to the base tick (only matters for base ticks between 100 and 150 ms, which don't exist today); no test that a burst due during an idle micro-glitch starts on time.
