# Linux release

## Objective
Ship nu11signal for Linux in the same release as macOS (user choice 2026-10-02: package Linux first, then one v0.4.0 for both).

## Design
- Linux archives built by cross-compiling with CGO_ENABLED=0 for linux/amd64 and linux/arm64 (no Swift helper; the app runs local files only on Linux): `nu11signal-<v>-linux-<arch>.tar.gz` holding `nu11signal-<v>/bin/nu11signal`, LICENSE, README.md, plus `.sha256` files, under `dist/v<v>/` next to the macOS artifacts; version stamped with -ldflags like macOS.
- `make release VERSION=x.y.z` builds macOS (signed/notarized, unchanged) and Linux archives; a `make release-linux` target builds only Linux (no Apple credentials needed); dry-run covers both.
- Homebrew on Linux: casks are macOS-only, so the tap gains `Formula/nu11signal.rb` (Linux only, per-arch url + sha256, installs bin/nu11signal) rendered from a template by the same bump script that renders the cask; `brew install wahh-22/tap/nu11signal` then works on Linux while macOS keeps the cask.
- GitHub release gets all artifacts; docs (install.md, releasing.md, README) explain Linux install (brew on Linux or the tarball).

## Tasks
- [ ] R1 — Linux archives in the release tooling + formula template + bump script support + hermetic script tests + docs. Route: delegated writer.

## Progress
- Created 2026-10-02 on branch `feat/linux-release` (worktree) from main 76f73ec.
