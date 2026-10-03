# Post-0.7 polish

## Objective
Close the follow-ups listed after v0.7.1 (owner request: items 2, 3, 4).

## Tasks
- [ ] Q1 (nu11signal, delegated writer): fix the flaky `TestCloseFadesAndIsIdempotent` (internal/playback/local) — find the race (last samples 0.0012 instead of 0 under -race) and make the test or the fade deterministic.
- [ ] Q2 (nu11signal-web, delegated writer): the hero's latest-version badge follows the theme colors (replace the fixed-color shields.io image).
- [ ] Q3 (both, same writers): recent review suggestions —
  - release.sh: version-token adjacency (a stamped version touching other token characters fails falsely), regex escaping of the version, a test for a truncated ELF; bump-cask duplicated hint and hidden precondition.
  - internal/update cache: `checked_at` is written but never read (decide: drop or use, keep old files readable).
  - docs: note that older versions do not know BLUESHIFT (a downgrade falls back to their default).
  - web: ThemeArt's per-theme asset list and fallback tokens hardcoded; the saved-theme migration untested; og-card/check-brand hardcoded logo paths.

## Scope note
Older backlog advisories in other odd/tasks files stay out of scope.

## Checks
- app: gofmt, vet, `go test -race ./...` (run the local package with -count=50 to prove the flake is gone), `make test-scripts`.
- web: build, check:links, check:brand, astro check, screenshots of the badge in the three themes.

## Progress
- Q1 + Q3 (app) implemented by the delegated writer (route: delegated, 2+ non-trivial files), uncommitted.
  - Q1 root cause: `render` signalled `faded` in the same call that rendered the fade's last frames (at 8000 Hz the 200 ms fade is 1600 frames = 25 pulls of 64, so it ends exactly at a chunk end), so a Close that returned before the test's next pull left the fade's tail (~0.001) as the final samples. Fix: `faded` is signalled only by the first render that begins with the fade done (silence), so the fade's last frames reach the device before Close closes it. Could not reproduce by `-count=300` / `-cpu 1,2,8`; RED via new deterministic `TestCloseSignalsFadedOnlyAfterSilence` (peak 0.0082 in the signalling render). GREEN: `-count=200 -race` passes.
  - release.sh: `has_version_stamp` matches VERSION literally (tr + grep -F, no regex) and rejects only boundaries that extend a version (digit/`.` before; digit, pre-release letters, or `.`/`-`/`+` + alnum after). `-trimpath` drops ldflags from build info, so no marker exists. Tests: adjacency (RED then GREEN), prerelease/longer stamps, literal 0x5y0, truncated ELF.
  - bump-cask.sh: one shared rebuild hint line; archives named by arch; `read_checksum` checks its own file-exists precondition (normally reported earlier by `check_checksum_files`).
  - update cache: `checked_at` no longer written; old files still load (test); older versions treat a missing timestamp as stale and ask.
  - docs: usage.md BLUESHIFT downgrade note; releasing.md stamp rule.
  - Checks: gofmt empty, vet ok, `go test -race ./...` ok, `make test-scripts` 62 passed, `bash -n` ok.
