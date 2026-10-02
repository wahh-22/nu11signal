# Original identity (no Cyberpunk references)

## Objective
Keep the neon look but drop every reference to Cyberpunk 2077 or the word "cyberpunk", to stay clearly original (user request, trademark/copyright caution).

## Decisions
- Theme NIGHT CITY is renamed REDSHIFT (user choice). Config files saying "NIGHT CITY" keep working (alias to REDSHIFT).
- Replace references to the game: place names (Night City, Kabuki, Badlands, Northside, Watson, Pacifica, Japantown, Heywood, Afterlife bar...), band/song names (Samurai, Chippin' In, Never Fade Away...), terms (Netrunner, NC-GRID, 2077-era dates), with original names. Generic words (neon, chrome, signal, night drive) may stay.
- Published release notes and git history stay as they are.

## Tasks
- [ ] O1 (nu11signal, delegated writer): rename the theme (code, config alias, goldens, README art, logo files and ids), remove "cyberpunk" wording (README, docs, code comments, cask/formula templates and caveats), replace the game references in the demo library, test fixtures, header strings and goldens; regenerate goldens and README art.
- [ ] O2 (nu11signal-web, delegated writer): theme id night-city -> redshift (with a saved-theme migration), logo files, copy without "cyberpunk", screens re-copied from the app.
- [ ] O3 (parent): GitHub repo descriptions; tap cask/formula desc at the next release.

## Checks
- app: gofmt, go vet, go test -race ./..., make test-scripts, readmeart byte-stable; `git grep -i` for the banned terms returns only the config alias, its test and history docs (odd/).
- web: build, check:links, check:brand, astro check; grep for banned terms; screenshots.

## Progress
