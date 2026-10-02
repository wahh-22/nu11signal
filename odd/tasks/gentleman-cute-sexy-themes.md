# Gentleman Cute and Sexy themes

## Objective
Add GENTLEMAN CUTE and GENTLEMAN SEXY to nu11signal's SETTINGS themes, deriving their semantic colors from the installed Pi theme files `Gentleman-Cute.json` and `Gentleman-Sexy.json` in the Gentle Shell package.

## Scope and constraints
- Preserve NIGHT CITY, BLUE, and MATRIX appearances, ordering, and default.
- Map Pi's semantic palette to existing radio roles rather than assuming an eight-color one-to-one recolor; leave the terminal's background unchanged.
- Add tested theme selection and rendering, update the two SETTINGS golden snapshots, and document the available themes. Do not modify the source Pi theme files.
- Work only on this feature's files and preserve unrelated work in the shared repository.

## Tasks
- [ ] T1 (in progress): Implement both semantic palettes in SETTINGS, add focused regression tests and user documentation. Route: delegated `gentle-ai-worker` (multiple nontrivial files and preparation reads). Acceptance: five ordered themes, case-insensitive lookup, distinct Cute/Sexy role values sourced from Pi JSON, no changes to legacy themes, tests pass. Verification: test-first observed RED then GREEN with `go test ./internal/radio`, followed by `go test ./...`, `go vet ./...`, and a formatting/diff check. Commit: pending. Review assessment: pending.

## Delivery and progress
- Forecast: approximately 150–250 authored changed lines, one cohesive work-unit commit; strategy `ask-on-risk` if the feature exceeds approximately 400 authored changed lines.
- Current state: T1 verified, pending commit. RED observed for missing themes and for Sexy's source-specific number color; focused tests GREEN, `go test ./internal/radio` passed (885 tests), `go test ./...` passed (1190 tests across 12 packages), `go vet ./...` passed, gofmt clean, `git diff --check` passed. Independent verifier found the Sexy number mismatch; corrected and independently rechecked against both Pi JSON palettes (focused test: 3 passed). Two SETTINGS snapshots and the SETTINGS SVG regenerated; no other generated asset changed bytes. Native assessment of staged changes: medium, under budget; independent verification applied due to unknown review outcome and small writer profile. Next: work-unit commit, record identity, native review routing.
- Source evidence: `internal/radio/theme.go` has the existing role vocabulary; the installed Pi JSON files have the source palettes.
