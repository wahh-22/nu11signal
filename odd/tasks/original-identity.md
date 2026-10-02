# Original identity (no Cyberpunk references)

## Objective
Keep the neon look but drop every reference to Cyberpunk 2077 or the word "cyberpunk", to stay clearly original (user request, trademark/copyright caution).

## Decisions
- Theme NIGHT CITY is renamed REDSHIFT (user choice). Config files saying "NIGHT CITY" keep working (alias to REDSHIFT).
- Replace references to the game: place names (Night City, Kabuki, Badlands, Northside, Watson, Pacifica, Japantown, Heywood, Afterlife bar...), band/song names (Samurai, Chippin' In, Never Fade Away...), terms (Netrunner, NC-GRID, 2077-era dates), with original names. Generic words (neon, chrome, signal, night drive) may stay.
- Published release notes and git history stay as they are.

## Tasks
- [x] O1 (nu11signal, delegated writer): rename the theme (code, config alias, goldens, README art, logo files and ids), remove "cyberpunk" wording (README, docs, code comments, cask/formula templates and caveats), replace the game references in the demo library, test fixtures, header strings and goldens; regenerate goldens and README art.
- [ ] O2 (nu11signal-web, delegated writer): theme id night-city -> redshift (with a saved-theme migration), logo files, copy without "cyberpunk", screens re-copied from the app.
- [~] O3 (parent): GitHub repo descriptions; tap cask/formula desc at the next release.

## Checks
- app: gofmt, go vet, go test -race ./..., make test-scripts, readmeart byte-stable; `git grep -i` for the banned terms returns only the config alias, its test and history docs (odd/).
- web: build, check:links, check:brand, astro check; grep for banned terms; screenshots.

## Progress
- O1 done (writer delegated). Final commits: `c308691` (wording, Homebrew templates, demo/fixture names outside internal/radio), `3413b75` (internal/radio: REDSHIFT with NIGHT CITY config alias, fixture names, NU-GRID, goldens; tools/readmeart), `361f81f` (REDSHIFT logo with renamed ids, regenerated screens, README image paths). Names: Fuseway, Hollow Wire, Long Wire Hymns, Kestrel Bay, Tidewater, Lowmarket, Kitamachi, Saltflat, the Copper Lantern, The Signal Thieves, NU-GRID; 2070s dates shifted back 56 years. `Seed: 2077` test seeds kept (they shape every golden; not user-visible). Real names left only in internal test fixtures (queen, daft punk, Digital Love), not in any public render.
- Checks: gofmt, vet, `go test -race ./...`, `make test-scripts` 58, readmeart byte-stable, banned-term grep leaves only the alias and its test.
- Reviews: `c308691` approved (`review-b133074a0633d308`; warnings: docs name REDSHIFT before code — resolved by `3413b75`; demo artist id rename). The radio slice could not be reviewed natively: alone it fails the README asset tests (finding R3-readmeart-assets-missing, a slicing artifact; lineage `review-8c3dbee797efe5c7` correction applied as the assets commit, recovery authorized by the user as successor `review-oi-redshift-r2`), and every test-green slice (code + goldens + generated SVGs) exceeds the reviewer context budget (`lens_context_budget_exceeded`). An earlier attempt `review-3d16f3cf679a9fdb` stopped with corrupted_or_unverifiable_authority. User chose to continue without a receipt for that part under ordinary policy (full tests + CI).
- O3: GitHub descriptions updated for both repos (no cyberpunk topics existed); the tap's cask/formula desc changes with the next release (`make cask`).
