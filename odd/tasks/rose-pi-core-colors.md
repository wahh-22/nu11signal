# Pi core colors for Rose themes

## Objective
Keep ROSE and NEON ROSE at exactly eight unique colors each while choosing their principal colors by semantic role from the corresponding Gentleman-Cute and Gentleman-Sexy Pi themes.

## Scope and decisions
- Preserve the existing names, NIGHT CITY/BLUE/MATRIX palettes, default, terminal background behavior, and the eight-color count across every radio role.
- Select the Pi core roles `bg`, `text`, `selectedBg`, `muted`, `accent`, `activePink`, `warning`, and `success`. Cute success is mint `#B4E7C7`; Sexy success is pearl `#D2CBD0`. Both share source-neutral values; their accents stay soft versus vivid.
- Reuse those eight colors for nu11signal-only roles. Pi's distinct border, champagne heading, error, and Sexy violet cannot all be retained within eight colors; do not call the result an exact copy of Pi.
- No push, PR, or merge is requested for this correction.

## Tasks
- [ ] T1 (in progress): Update both theme palettes, tests, SETTINGS snapshots/artwork and user documentation to reflect Pi's principal eight colors. Route: delegated `gentle-ai-worker` (multiple nontrivial files). Acceptance: each source Pi core role maps to the corresponding radio color where available; exactly eight unique colors per theme; old three themes unchanged; names and persistence unchanged. Test-first: observed RED/GREEN for semantic roles and eight-count; then `make test`, `make vet`, `make fmt-check`, SVG freshness and diff checks. Work-unit commit: pending. Native review: pending.

## Progress
- Forecast: approximately 100–200 authored changed lines in one work-unit commit; delivery strategy `ask-on-risk` if scope exceeds approximately 400 lines.
- Current state: T1 implementation complete, pending independent verification and commit. RED observed for mismatched muted/success/extra colors; focused tests GREEN, `go test ./internal/radio`, `make test`, `make vet`, `make fmt-check`, and `go test ./tools/readmeart` passed. No golden/SVG bytes changed. ROSE success is Pi mint; NEON ROSE success is Pi pearl; frames/rain use Pi muted. Next: independent verification, work-unit commit, and native review if due.
- Source evidence: `internal/radio/theme.go` defines `pinkTheme`; `internal/radio/theme_test.go` enumerates all roles in `themeRoles`; installed Pi JSON maps its semantic roles.
