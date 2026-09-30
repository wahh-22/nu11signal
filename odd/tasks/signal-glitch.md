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

## Round 4 (user feedback, 2026-09-30)

Scattered micro-glitches are hard to perceive. The user wants text alterations on practically the whole screen at once (many words), less often, alternating with the full-screen burst (midway between bursts).

- [x] G4 — Replace micro-glitches with a text "wave": ~70–80% of visible words scrambled at once (original colors kept), ~0.6–1 s, words resolving at staggered times; scheduled midway between full-screen bursts; keep bursts, NO SIGNAL flash, alerts, title-change glitch and exclusions; measure CPU. Branch `feat/glitch-wave`. Route: delegated.
- G4 done (route: delegated writer). Micro-glitches removed; text wave scrambles ceil(75%) of visible words (seeded ranking across all lines, colors kept via `setCells`) for 0.6–1.0 s, all scrambled for the first 30%, then staggered resolve (60% random / 40% column); 10 fps during the wave. Cadence: wave halfway between consecutive bursts (bursts every 20–45 s → wave 10–22.5 s after each burst; something every ~10–23 s). RED: vet `nextWave` undefined; GREEN: `go test -race ./...` 725 passed, `go vet`/`gofmt` clean; effects goldens regenerated + new `text_wave_80x24`. CPU (60 s demo): idle 1.06% effects vs 0.89% calm; playing 3.78% vs 3.55%.
- G4 review: commit `150c826`, RDD medium, 762 lines, consent granted, lineage `review-dcf301df5a198a02`, reliability lens, APPROVED, acknowledged (burned). Suggestions (not scheduled): idle-interval test can skip silently on an alert; no test for a late tick where the burst wins over a pending wave.

## Round 5 (user feedback, 2026-09-30)

Effects feel too fast, like the app is failing. Make them smoother and a bit longer so they read as an intentional effect.

- [x] G5 — Smooth wave (1.6–2.4 s, staggered ramp-in and eased staggered resolve, slower glyph flicker ~160 ms) and gentler bursts (longer, fewer/shorter tears, sparser noise, slower flicker, rarer and softer NO SIGNAL); cadence unchanged; measure CPU. Branch `feat/glitch-smooth`. Route: delegated.
- G5 done (route: delegated writer). Wave 1.6–2.4 s: column-biased ramp-in before 35%, hold to 50%, eased resolve 0.5+0.4·u^1.6, letters turn left to right over ~10% of the wave, per-cell glyph phase on a 160 ms period (shimmer). Bursts 0.5–0.9 s: 1–2 rows shifted 1 cell settling back, 3–6 noise cells moving every 125 ms, static bar 1 in 4 (≤250 ms), NO SIGNAL 1 in 6 with dim→full→dim; idle burst tick 125 ms. Cadence unchanged. RED: 8 failing tests with a stub; GREEN: `go test -race ./...` 734 passed, vet/gofmt clean. CPU (demo, 60 s ×2): idle effects 0.58–0.60% vs base 0.40–0.49%; playing within noise; wakeups unchanged.
- G5 review: commit `6de4d14`, RDD medium, 674 lines, consent granted, lineage `review-02ce5717b6d18931`, reliability lens, APPROVED, acknowledged (burned). Suggestion (not scheduled): while playing, the last burst frame can linger up to one playing tick past burstEnd.

## Round 6 (user feedback, 2026-09-30)

- Real EQ bars jump on the first play (decorative → empty → real).
- The NO SIGNAL bursts (with and without the sign) were the best; G5 made them too rare and dim — restore the G4 burst look (2–4 tears, more noise, static bar, red NO SIGNAL 1 in 4).
- The text wave is too strong (whole words): scramble ~65% of the letters instead.

- [x] G6 — EQ start without the decorative jump (app mode starts from zero, decorative only after 1.5 s without levels); restore G4 bursts (keep the smooth text wave); wave scrambles ~65% of eligible letters. Branch `feat/glitch-eq-tune`. Route: delegated.
- G6 done (route: delegated writer). EQ: in app mode bars hold heights up to 1.5 s waiting for the first reading, then decorative with an eased handover; decorative→real glides over 4 frames; play start clears the kept reading and drains the one-slot channel; empty readings ignored. Bursts: G4 look restored (0.2–0.6 s, 2–4 tears of 1–3 cells, 6–14 noise ×3 on NO SIGNAL, static bar ~half the frames, full red framed NO SIGNAL 1 in 4, 66 ms tick also while playing). Wave: exact ceil(0.65 × text cells) scrambled (~64% at the hold), G5 timing kept. RED: compile + 7 runtime failures; GREEN: `go test -race ./...` 765 passed, vet/gofmt clean. CPU not re-measured (bursts ≤0.6 s every 20–45 s).
- G6 review: commit `8fad41c`, RDD high, 893 lines, consent granted, lineage `review-26ce3e4de9c0c5d1`, 4 lenses, APPROVED, acknowledged (burned). Follow-ups (not scheduled): name the burst tuning literals again; name the per-letter hash salt; phrase the EQ handover comment via eqHandover; burst tick ignores base d < 66 ms; tears can land on the same row twice.

## Round 7 (user decision, 2026-09-30)

- [x] G7 — Final values: bursts 0.6–1 s; text wave 1–2 s scrambling 40% of the letters (route: inline, constants + pinned test values + goldens + README). `go test -race ./...` ok, vet/gofmt clean; goldens `no_signal_80x24` and `text_wave_80x24` regenerated.
- G7 review: commit `cca31a4`, lineage `review-c63af848460e44c4`, reliability lens, APPROVED with no findings, acknowledged (burned).

## Round 8 (user feedback, 2026-09-30)

With the content intro in place the text wave is redundant.

- [x] G8 — Smoother content intro (~650 ms, glyph change ~90 ms, eased left-to-right resolve); remove the text wave entirely; bursts more frequent (every 10–22 s), keeping 0.6–1 s and NO SIGNAL 1 in 4. Branch `feat/intro-smooth`. Route: delegated.
- G8 done (route: delegated writer). Intro 650 ms: 20% hold, ease-out left-to-right resolve (front covers 1-(1-t)² of the columns, 10% jitter), per-cell 90 ms glyph phase, 50 ms tick. Text wave removed entirely (golden deleted); burst salts pinned by `TestBurstSaltsStayPut`; bursts every 10–22 s (0.6–1 s, NO SIGNAL 1 in 4). RED: 5 failing tests; GREEN: `go test -race ./...` ok, vet/gofmt clean; burst goldens changed only in clock/progress text.
- G8 review: commit `10c2985`, RDD medium, 925 lines, consent granted, lineage `review-b440520d68ac7046`, reliability lens, APPROVED with no findings, acknowledged (burned).

## Round 9 (user feedback, 2026-09-30)

- [x] G9 — Softer intro (~50% of new cells, alphanumeric glyphs instead of heavy blocks, ~900 ms, ~140 ms glyph period); keep only the rain visualizer (remove bars/oscilloscope/synthwave, `v` key and random; config `visualizer` accepted but ignored) and make rain strongly music-driven (per-band density/speed/length, onset/bass hits spawn bursts and flash, silence = dry); slightly gentler bursts (1–3 tears, 1–2 cells, less noise). Branch `feat/intro-smooth`. Route: delegated.
- G9 done (route: delegated writer). Intro 900 ms, 50% of new cells (seeded), light glyphs A–Z 0–9 + - = : / < >, 140 ms per-cell glyph phase. Only rain remains (bars/scope/synthwave, `v`, random removed; config `visualizer` ignored silently). Rain: energy ((l−0.08)/0.92)^1.7, dry at silence; spawn/speed/trail/head tied to energy; waveform loudness gain 0.7–1.3; onsets vs a 35% moving average (bass ×1.5) spawn a drop wave + flash (0.55/frame decay, only if stronger than the remaining flash). Bursts: 1–3 tears of 1–2 cells, 4–10 noise (×2 on NO SIGNAL), static bar ~1 in 3 frames. RED: compile + burst/intro assertions; GREEN: `go test -race ./...` 854 passed, vet/gofmt clean; 17 goldens regenerated (NOW PLAYING now rain). CPU demo playing ~3.8–4.1% (within noise of the previous build), idle 0.44%.
