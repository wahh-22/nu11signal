# Feature: visualizers

Locator: `odd/tasks/visualizers.md` · Engram mirror: `odd/visualizers/tasks` · Branch: `feat/visualizers`

## Objective

User request (2026-09-30): remove the status-line alerts; offer several cyberpunk visualizers besides the bars, selectable in a JSON config file, with a random option that changes per song.

## Decisions

- Visualizers (user choice): bars (current), oscilloscope (braille waveform), neon waterfall (scrolling spectrogram cyan→magenta→yellow), data rain (hex/katakana columns reacting per band), synthwave horizon (perspective grid with spectrum mountains).
- Config: `os.UserConfigDir()/nu11signal/config.json` with `{"visualizer": "bars"|"oscilloscope"|"waterfall"|"rain"|"synthwave"|"random"}`; `random` picks per song (seeded by song id); a key cycles visualizers live; invalid/missing config → bars.
- Real data from the helper levels event (bands + a new 64-point waveform); decorative fallback per visualizer without real data.

## Tasks

- [x] A0 — Remove the status-line alerts. Route: delegated.
- [x] X1 — Helper/Go: add a downsampled waveform (64 points, pre-gain, −1..1) to the `levels` event; port/fake/demo/tests. Route: delegated.
- [x] X2 — TUI: visualizer abstraction + 5 visualizers + config loader + random per song + cycle key + decorative fallbacks + tests/goldens + README; CPU check. Route: delegated.

## Progress

- A0 done (route: delegated writer). Alert code removed from glitch.go/view.go (salts pinned so burst/wave patterns keep their values), 3 alert tests removed and 3 adjusted, README updated, `no_signal_80x24` golden status row only. `go test -race ./...` 762 passed, vet/gofmt clean.
  - A0 review: commit `5e81575`, RDD medium, 209 lines, consent granted, lineage `review-d79bffa62ada3f19`, reliability lens, APPROVED, acknowledged (burned). Suggestions (not scheduled): guard the pinned salts with a test; a long-run test that the status line stays SYS NOMINAL with effects on.
- X1+X2 done (route: delegated writer; one commit — model.go/levels.go mix both). X1: helper adds a 64-point peak-preserving waveform (−100..100) to `levels`; Go `playback.Spectrum{Bands, Wave}`, `LevelSource.Levels() <-chan Spectrum`. X2: `visualizer` interface (`Name/Step/Render/Idle`, value semantics), five visualizers (bars unchanged, oscilloscope braille with auto-gain, waterfall 12-row cyan→magenta→yellow, rain hex/half-width katakana, synthwave sun+mountains+grid), `internal/config` File source (`visualizer`, `random` per song via FNV of song id avoiding the previous pick), `v` cycles, text wave skips braille. RED seen for Swift/Go wiring/config/cmd; visualizer tests written after the code (no RED). GREEN: Go 1005, Swift 198; existing goldens unchanged; 5 new viz goldens. CPU demo playing: bars 2.5%, scope 1.9%, waterfall 2.2%, rain 2.4%, synthwave 2.5%. Open: 12-row cap leaves blank space in the expanded player; compact layout has no visualizer; scope auto-gain untuned on real music.
  - X review: commit `a7dfb82`, RDD high, 2230 lines, consent granted, lineage `review-ed7912b6a3651372`, 4 lenses, APPROVED, acknowledged (burned). Follow-ups (not scheduled): vizSize duplicates nowPlaying geometry with unnamed numbers; unused vizInput.Frame; wrong doc refs (loudInput, config.go); synthwave freeze test has no slack; test vizInput freshness; test the real helper emits wave.

## Round 2 (user feedback, 2026-09-30)

- [x] X3 — Content intro transition: new content (tab/view switch, page load, lists arriving, picker/editor) scrambles in and resolves left to right in ~400 ms like the song-title change, only on changed content (not the visualizer, clock or progress); respects `x`/`--calm`. Remove the waterfall visualizer. Oscilloscope/rain/synthwave clearly driven by the music and restyled with the bars' palette/style; synthwave redesigned (horizon, solid spectrum mountain range above, dim reflection over a bass-pulsing perspective grid below). Route: delegated.
- X3 done (route: delegated writer). Intro: frame diff after every non-tick/non-resize message (before/after laid out at the same instant), only the list panel and NOW PLAYING artist/album/feed rows; a row is new if its text is on no previous row of that region (selection marks ignored, shared-start rows matched); SEARCH input and NEW PLAYLIST name rows never compared; 400 ms, 30% hold then left-to-right resolve, `setCells`, 50 ms tick only while running; own `introOn()` switch (effects on, not tiny/auth). Waterfall removed (old config → bars + notice). Palette unified to the bars' inks (`barInk`, `levelInk`); rain dry under 0.08 with strong band response; scope auto-gain fast attack/slow release ≤4x; synthwave redesigned (horizon, solid range, dim reflection over a bass-pulsing grid, no sun). RED: intro build failure + palette/scope/rain/synthwave assertions; GREEN: `go test -race ./...` 987 passed, vet/gofmt clean. CPU demo: bars 2.0%, synthwave 2.8–3.0%, idle 0.55%. Open: no explicit tiny-layout intro test; scope auto-gain untuned on real music.
  - X3 review: commit `4e1c70c`, RDD high, 1267 lines, consent granted, lineage `review-c27b6a7d5c460f51`, 4 lenses, APPROVED, acknowledged (burned). Advisories → X3f.
- [x] X3f — X3 review follow-ups: find the NEW PLAYLIST name row from its real zone (never scramble it while typing); restore the oscilloscope gain clamp (silence must not amplify noise); name the NOW PLAYING intro rows from the layout instead of 3/4/7; sameRow in runes with named thresholds; carry running intro cells correctly on cursor/scroll; skip the double layout for messages that cannot add content where safe; tests for tiny/auth intro off; small test/readability fixes. Route: delegated.
- X3f done (route: delegated writer). `zoneNameInput` so the name row is skipped wherever drawn; scope gain floor restored (≤4x); NOW PLAYING rows as named constants shared by nowPlaying and the intro; `sameRow` in runes with named thresholds; `intro.carried` keeps only unchanged cells; `introQuiet` skips tick/resize/volume/setVolume/seek/loop messages; tiny/auth intro-off tests; `rainRamp`/`trailInk`; slices.Contains; intro timing fixed. RED: 6 observed failures (name zone, sameRow runes, moved-text scramble, quiet messages, 50x hiss gain, rain ramp build); GREEN: `go test -race ./...` ok, vet/gofmt clean.
