# Pi core colors for Rose themes

## Objective
Keep ROSE and NEON ROSE at exactly eight unique colors each while choosing their principal colors by semantic role from the corresponding Gentleman-Cute and Gentleman-Sexy Pi themes.

## Scope and decisions
- Preserve the existing names, NIGHT CITY/BLUE/MATRIX palettes, default, terminal background behavior, and the eight-color count across every radio role.
- Select the Pi core roles `bg`, `text`, `selectedBg`, `muted`, `accent`, `activePink`, `warning`, and `success`. Cute success is mint `#B4E7C7`; Sexy success is pearl `#D2CBD0`. Both share source-neutral values; their accents stay soft versus vivid.
- Reuse those eight colors for nu11signal-only roles. Pi's distinct border, champagne heading, error, and Sexy violet cannot all be retained within eight colors; do not call the result an exact copy of Pi.
- No push, PR, or merge is requested for this correction.

## Tasks
- [x] T1: Update both theme palettes, tests and user documentation to reflect Pi's principal eight colors; confirm SETTINGS snapshots/artwork remain current. Route: delegated `gentle-ai-worker` (multiple nontrivial files). Acceptance: each source Pi core role maps to the corresponding radio color where available; exactly eight unique colors per theme; old three themes unchanged; names and persistence unchanged. Verification: observed RED/GREEN for semantic roles and eight-count; `go test ./internal/radio` (891 passed), `make test`, `make vet`, `make fmt-check`, SVG freshness and diff checks passed; independent verifier confirmed source JSON values and eight-color count. Work-unit commit: `df4055330b52d5284a79ca7e8ecb2d94723cb701`. Native review `review-face2f9f6c38bc08` approved and acknowledged.

## Progress
- Forecast: approximately 100–200 authored changed lines in one work-unit commit; delivery strategy `ask-on-risk` if scope exceeds approximately 400 lines.
- Current state: T1 complete in local branch `feat/rose-pi-core-colors`; original worktree `main` and Spotify worktree untouched. RED observed for mismatched muted/success/extra colors; focused tests GREEN, full checks and independent verification passed. No golden/SVG bytes changed because SETTINGS uses NIGHT CITY. ROSE success is Pi mint; NEON ROSE success is Pi pearl; frames/rain use Pi muted. Native review approved and acknowledged. Terminal visual legibility was not manually checked; dark dim/selection may be faint. No push, PR or merge requested. Next: user decides delivery.
- Source evidence: `internal/radio/theme.go` defines `pinkTheme`; `internal/radio/theme_test.go` enumerates all roles in `themeRoles`; installed Pi JSON maps its semantic roles.
