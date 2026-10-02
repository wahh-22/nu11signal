# Rose and Neon Rose themes

## Objective
Add ROSE and NEON ROSE to nu11signal's SETTINGS themes, inspired by the installed Pi `Gentleman-Cute.json` and `Gentleman-Sexy.json` palettes without copying their theme names.

## Scope and constraints
- Preserve NIGHT CITY, BLUE, and MATRIX appearances, ordering, and default.
- Each new theme must use exactly eight distinct colors across all radio roles, like NIGHT CITY and BLUE; preserve the source palettes' soft-versus-vivid pink character and leave the terminal's background unchanged.
- Add tested theme selection and rendering, update the two SETTINGS golden snapshots, and document the available themes. Do not modify the source Pi theme files.
- Work only on this feature's files and preserve unrelated work in the shared repository.

## Tasks
- [x] T1: Implement initial Gentleman palettes in SETTINGS, add focused regression tests and user documentation. Route: delegated `gentle-ai-worker`. Verification: observed RED/GREEN, full checks passed, scoped review approved for the earlier candidate. Original work-unit commit: `8d90572ff9ba20a491b53ce98316bc195d062df1`; clean-branch cherry-pick: `21aa6d9`. Superseded by T2 on user feedback.
- [x] T2: Rename the two themes ROSE and NEON ROSE, constrain each to exactly eight unique colors across all radio roles, update selection/persistence tests, SETTINGS golden snapshots, generated artwork, README and usage docs. Route: delegated `gentle-ai-worker` (multiple nontrivial files). Acceptance: five themes in order; default and other themes unchanged; each pink palette has eight unique colors and retains its soft/vivid distinction. Verification: test-first observed RED/GREEN using themeRoles and settings tests, then `make test`, `make vet`, `make fmt-check`, generated-art freshness, clean diff, native review `review-2ad8d9c83a64c197` approved and acknowledged (one informational warning R3-001, not a correction). Work-unit commit: `f95742afdea6bd806a26f3966018bb4ef177d38c`.
- [ ] T3 (in progress): Revise approved issue #64 and PR #65 to reflect ROSE/NEON ROSE and eight-color palettes, push the corrected branch, wait for required CI, then merge only if checks and mergeability pass. Route: inline GitHub delivery; no source changes. Acceptance: issue and PR agree with code, exactly one `type:feature` label, checks green, merged state verified. Commit: not applicable (delivery-only).

## Delivery and progress
- Forecast: approximately 150–250 authored changed lines, one cohesive work-unit commit; strategy `ask-on-risk` if the feature exceeds approximately 400 authored changed lines.
- Current state: T1 historically complete, T2 done: implementation verified, committed and natively reviewed. ROSE and NEON ROSE each use exactly eight unique colors across `themeRoles`; obsolete display names are rejected. Test-first RED/GREEN observed; `go test ./internal/radio` passed (888 tests), `make test`, `make vet`, `make fmt-check`, SVG freshness and diff checks passed. Only SETTINGS SVG changed among regenerated assets. PR #65 for approved issue #64 remains draft pending revised remote title/body and CI. Native review approved T2 and acknowledged authority; R3-001 is an informational later follow-up, not a blocker. Next: update issue/PR and merge only after checks pass.
- Source evidence: `internal/radio/theme.go` has the existing role vocabulary; the installed Pi JSON files have the source palettes.
