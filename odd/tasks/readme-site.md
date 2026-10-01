# README and website

## Objective
A professional open-source README (in the style of github.com/Gentleman-Programming/gentle-shell: centered brand header, badges, link row, tagline, screenshots, feature sections, docs links) with the null emblem; and a public website for nu11signal in its own repository.

## Tasks
- [x] R1 — README redesign: centered SVG emblem banner (generated from the emblem grid in theme colors), name, tagline, shields badges (release, macOS 14+, license MIT, stars, last commit), link row (Website · Install · Docs · Releases), terminal screenshots rendered to SVG from the ANSI goldens (NIGHT CITY, BLUE, boot), feature sections separated by `---` with a "Docs →" link each, short Install/Quick start, then links to detailed docs. Move the long reference sections (build from source, releasing, keys, mouse, protocol, helper lookup, troubleshooting, repository notes…) into `docs/*.md` without losing content. A small generator (`go run ./tools/readmeart`) re-renders the SVGs. Route: delegated writer.
- [ ] W1 — Website in a new public repo (name/owner pending user confirmation), fast static framework (Astro), own branding with the emblem, deployed later. Route: delegated writer after confirmation.

## Progress
- Created 2026-10-01 on branch `feat/boot-splash` (B1–B5 reviewed, not yet merged).
- R1 implemented (delegated writer, uncommitted, pending parent commit/review): README rebuilt (banner, badges, link row without Website until the site exists, pitch, 8 `---` feature sections with SVG screens and Docs links, Get started, Documentation index, Contributing, License, back to top). Reference content moved verbatim into `docs/` (install, usage, effects, audio, architecture, building, releasing, troubleshooting, contributing) with intra-doc anchors rewritten. `go run ./tools/readmeart` renders `docs/assets/brand/nu11signal-{banner,emblem}.svg` from `radio.EmblemArt()` and 8 screens in `docs/assets/screens/` from the ANSI goldens; `go test ./tools/readmeart` checks well-formed XML, 36 emblem rects, and that the committed SVGs are not stale.
- R1 checks: generator stable across reruns; `go build ./...`, `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean; `make test-scripts` 34 passed; 73 relative links in README/docs resolve.
- R1 follow-ups: `scripts/release.sh` and `Makefile` comments still point to "README.md, Releasing" (now `docs/releasing.md`); the published v0.2.1 cask does not depend on font-kode-mono yet (docs say "releases after v0.2.1").
- R1 parent follow-up: scripts/release.sh setup messages and Makefile header now point to docs/releasing.md / docs/building.md; `make test-scripts` 34 passed.
- R1 reviews: the single README commit exceeded the reviewer context budget (`lens_context_budget_exceeded`), so it was split: `a5edfab` code (readmeart + release pointers) high, 581 lines, APPROVED (`review-e75b99cd22545341`; advisories: 256-color SGR not parsed, parseLine comment, stale-asset gate ties goldens to SVGs); `69a30b7` + `beb4c3f` generated SVG assets, medium, 1139 lines, APPROVED (`review-a7366d13cd573c32`); `dfc8ac4` README + docs text assessed passive (no review needed).
