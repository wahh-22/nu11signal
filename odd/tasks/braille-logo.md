# Braille logo in the terminal

## Objective
Replace the in-app null emblem (Braille slashed zero beside "N U 1 1 / S I G N A L" and thin bars) with a Braille rendering faithful to the official vector logo: the masked head with headphones and 4x4 LED eyes, the wordmark NU11SIGNAL written together, and five thick slanted bars centered under it.

## Design (approved by the user from Braille previews)
- Large (52 x 8 cells): head 16 x 8 cells drawn geometrically on a 32 x 32 dot grid; LEDs one dot each at pitch 2, slanted toward the center; wordmark in a 6 x 8 dot bold font (pitch 7) on rows 3-4; bars on rows 5-6.
- Compact (24 x 6 cells): head 12 x 6 cells (3 x 3 LEDs), NU11SIGNAL as plain text on row 2, `⡾⡾⡾⡾⡾` bars on row 3.
- Colors follow the logo per theme: primary (frame, headphones, top LED row, SIGNAL, bars) and secondary (NU11, other LED rows). NIGHT CITY #FF5F57/#5EF6FF, BLUE #347AFF/#5CE1FF, MATRIX #00C832/#00FF41, ROSE #F095C8/#FFB1DD, NEON ROSE #F43888/#FF4F9A.
- Generator kept outside the repo (scratchpad); the art is committed as data.

## Tasks
- [x] T1 (delegated writer: emblem, theme, splash/idle layout, goldens, readmeart, docs): swap the emblem art and its coloring.

## Checks
- `go vet ./...`, `gofmt -l .`, `go test ./...` (goldens regenerated and reviewed), `go run ./tools/readmeart` leaves no stale assets.
- Visual check of boot splash, idle emblem (expanded and 80x24) and `--version` output.

## Progress
- T1 done (writer delegated; parent narrowed `TestSwapDissolvesTheEmblemIntoTheRain`'s mix window to 0.3-0.6 because the 80x24 logo's last cells leave by about 0.66 with seed 2077). Commits `0329228` (code, tests, goldens, docs) and `7911ff9` (regenerated emblem and boot SVGs). Checks: gofmt clean, `go vet ./...`, `go test -race ./...` all pass, readmeart assets byte-stable, `--version` prints the compact logo. Reviews: `0329228` medium slice_budget_reached, granted, approved (`review-8618f3527d7e7390`; warning R3-stale-readme-assets resolved by `7911ff9`; suggestions on the splash glitch test, the narrowed swap window, --version dropping the version line silently); `7911ff9` granted, approved (`review-b3881cc9cb3ebe07`).
