# Gentleman Cute and Sexy themes

## Objective
Add GENTLEMAN CUTE and GENTLEMAN SEXY to nu11signal's SETTINGS themes, deriving their semantic colors from the installed Pi theme files `Gentleman-Cute.json` and `Gentleman-Sexy.json` in the Gentle Shell package.

## Scope and constraints
- Preserve NIGHT CITY, BLUE, and MATRIX appearances, ordering, and default.
- Map Pi's semantic palette to existing radio roles rather than assuming an eight-color one-to-one recolor; leave the terminal's background unchanged.
- Add tested theme selection and rendering, update the two SETTINGS golden snapshots, and document the available themes. Do not modify the source Pi theme files.
- Work only on this feature's files and preserve unrelated work in the shared repository.

## Tasks
- [x] T1: Implement both semantic palettes in SETTINGS, add focused regression tests and user documentation. Route: delegated `gentle-ai-worker` (multiple nontrivial files and preparation reads). Acceptance: five ordered themes, case-insensitive lookup, distinct Cute/Sexy role values sourced from Pi JSON, no changes to legacy themes, tests pass. Verification: observed RED then GREEN for missing themes and Sexy's numeric role; `go test ./internal/radio` (885 passed), `go test ./...` (1190 passed), `go vet ./...` passed, gofmt and `git diff --check` clean; independent verifier confirmed numeric correction. Work-unit commit: `8d90572ff9ba20a491b53ce98316bc195d062df1`. Native assessment: medium, under budget; defer review to a focused PR slice.

## Delivery and progress
- Forecast: approximately 150–250 authored changed lines, one cohesive work-unit commit; strategy `ask-on-risk` if the feature exceeds approximately 400 authored changed lines.
- Current state: T1 done. The two SETTINGS snapshots and SETTINGS SVG were regenerated; no other generated asset changed bytes. Work-unit commit `8d90572ff9ba20a491b53ce98316bc195d062df1` contains the behavior, tests, documentation and assets. Native committed-range assessment from `4103d72335a3976ca5bb920f81dc0b591f6ea656`: medium, review not yet due (`under_budget`); independent verification passed. Native inspect cannot start review (`managed_assets_outdated`) and projects unrelated prior branch changes, so no review was started and the future PR slice must be scoped correctly. No push or PR created. Next: user decides delivery; review remains pending.
- Source evidence: `internal/radio/theme.go` has the existing role vocabulary; the installed Pi JSON files have the source palettes.
