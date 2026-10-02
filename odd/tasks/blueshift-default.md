# BLUESHIFT as the main theme

## Objective
Rename the BLUE theme to BLUESHIFT (user choice) and make it the app's and the brand's main theme.

## Decisions
- BLUESHIFT is the default theme (no config, unknown name) and comes first in SETTINGS. A config.json saying "BLUE" keeps working (alias), like "NIGHT CITY" -> REDSHIFT.
- Golden baselines stay on REDSHIFT: the test harness pins REDSHIFT explicitly so existing goldens do not churn; a test asserts the default is BLUESHIFT.
- README banner and main screenshot use BLUESHIFT; the logo file is renamed (ids too).
- Website: theme id `blue` -> `blueshift`, default theme, favicon and social card from the BLUESHIFT logo; a saved `blue` migrates.

## Tasks
- [x] B1 (nu11signal, delegated writer): rename, alias, default, SETTINGS order, test pin, README art and logo, docs.
- [ ] B2 (nu11signal-web, delegated writer): theme id, default, logos, favicon, og card, screens, migration, copy.

## Checks
- app: gofmt, vet, go test -race ./..., make test-scripts, readmeart byte-stable, grep for stale "BLUE" theme mentions.
- web: build, check:links, check:brand, astro check, screenshots incl. migration.

## Progress
- B1 done (writer delegated; parent fixed the config.go example and the glitch.go alert color comment). Commit `feat(radio): BLUE becomes BLUESHIFT, the default theme`: alias BLUE -> BLUESHIFT, `defaultTheme` (themes[0]) used at startup and as fallback, test `TestMain` pins REDSHIFT so goldens keep their bytes (only the SETTINGS order lines changed; blue goldens renamed), README banner and main screenshot BLUESHIFT. Checks: gofmt, vet, `go test -race ./...`, `make test-scripts` 58, readmeart byte-stable, logo render byte-identical. Review medium, approved (`review-c1604a889f561619`; suggestion: older versions do not know BLUESHIFT).
