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
