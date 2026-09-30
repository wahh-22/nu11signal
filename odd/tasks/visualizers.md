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
