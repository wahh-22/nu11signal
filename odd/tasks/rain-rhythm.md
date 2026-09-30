# Rain rhythm

## Objective
Make the rain visualizer read clearly as following the music, especially on flat, heavily compressed tracks where the bars still show the rhythm but the rain does not.

## Problem / why
- Drops fix speed and length at spawn and live for seconds: the rain shows the past, not the current beat.
- Compressed music moves in a narrow level range (e.g. 0.5–0.6), which the fixed energy curve maps to nearly constant rain.
- Hits only lift head color one or two steps and need a large rise over a 35% moving average, which flat music rarely produces.

## Scope
`internal/radio/viz_rain.go` and its tests (`viz_test.go`, goldens if affected). No helper, config or other visualizer changes.

## Constraints
Deterministic (seed + frames), no extra timers, negligible CPU, paused rain still holds, drizzle without readings unchanged, silence stays dry. Conventional commits.

## Tasks
- [x] R1 — Adaptive per-band gain (running min/max per band, level normalized into the full scale, with a minimum span so noise is not amplified); live drops take their column's current speed each frame (smoothed); adaptive beat pulse on total spectral flux (threshold relative to recent flux) that briefly boosts the whole rain's speed and brightness for a couple of frames. Route: delegated writer (one non-trivial file + tests, needs preparation reading).

## Acceptance criteria
- A flat track (levels oscillating 0.50–0.60 with a periodic beat) produces clearly different rain density/speed at peaks vs troughs and a pulse on each beat.
- Silence stays dry; no readings still drizzles; paused holds.
- `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean.

## Delivery
Forecast ~300 authored lines; strategy ask-on-risk. Branch `feat/rain-rhythm` stacked on `feat/intro-smooth`.

## Progress
- Created 2026-09-30.
- R1 done (route: delegated writer), commit `0a214c3`. rainGain: per-band floor/ceiling (instant attack, 0.05 release), stretch around range middle by 0.5/max(span,0.15) (≤3.3×, never <1), eased below knee 0.3, untouched ≤ rainFloor. Live speed: falling drops ease 50%/step toward current column speed (dry columns keep pace). Beat: bass-weighted mean positive flux > 1.5× recent avg + 0.02, refractory 3 steps; pulse 1 → ×0.45/step, extra fall rainPulseFall×pulse, heads lifted via max(flash, 0.6×pulse). RED: flat track peak/trough speed 0.87/0.85 (want ≥1.4×); pulse tests did not compile. GREEN: 0.94/0.57 (1.65×), beat on every beat and none between; `go test -race ./...` pass, vet/gofmt clean; golden viz_rain_80x24 regenerated.
- Review: assess medium, 359 lines, under_budget → pending in slice (boundary c650bbb). Next: user tries it by ear; tune constants if needed.
