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
