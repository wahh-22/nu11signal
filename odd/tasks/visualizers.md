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
- [ ] X1 — Helper/Go: add a downsampled waveform (64 points, pre-gain, −1..1) to the `levels` event; port/fake/demo/tests. Route: delegated.
- [ ] X2 — TUI: visualizer abstraction + 5 visualizers + config loader + random per song + cycle key + decorative fallbacks + tests/goldens + README; CPU check. Route: delegated.

## Progress

- A0 done (route: delegated writer). Alert code removed from glitch.go/view.go (salts pinned so burst/wave patterns keep their values), 3 alert tests removed and 3 adjusted, README updated, `no_signal_80x24` golden status row only. `go test -race ./...` 762 passed, vet/gofmt clean.
