# Vector logos

## Objective
Adopt the five user-supplied vector logos (masked head with headphones, 4x4 LED eyes, NU11SIGNAL in Kode Mono Bold outlines, five slanted bars) as the project's logo: one per color theme (night-city, blue, matrix, rose, neon-rose).

## Scope
- nu11signal: store the logos under `docs/assets/brand/logo/`, use the Night City logo as the README banner, retire the generated Braille banner.
- nu11signal-web: the hero shows the logo of the active theme (NIGHT CITY, BLUE, MATRIX), switching with the theme; favicon and social card from the new logo.
- Out of scope: the in-terminal Braille emblem (a terminal cannot draw SVG); rose themes on the website (ask).

## Constraints
- Logos are used as supplied; only metadata text (title/desc) is translated to English.
- Website: GitHub Pages from main; deliver through a PR.

## Tasks
- [x] T1 (nu11signal, delegated writer: 2+ files incl. tool and test): logos in repo, README banner, readmeart stops generating the banner.
- [ ] T2 (nu11signal-web, delegated writer: 4+ files): theme-matched hero logo, favicon, og.png.

## Checks
- T1: `go test ./tools/readmeart/`, `go test ./...`, README renders (structural readback).
- T2: `npm run build`, visual check of the built hero in each theme.

## Progress
- T1 done, commit `cdf8690` (writer delegated; parent removed the dead banner builders in tools/readmeart/emblem.go). Checks: xmllint on the 5 logos clean; `go test ./tools/readmeart/` pass, regenerated assets not stale; `go test ./...` 1196 passed. Review: medium, slice_budget_reached, consent granted, 1 lens, lineage `review-22e544b08ff604d3` approved and acknowledged; suggestion R3 (no test guards the README logo path), not scheduled.
