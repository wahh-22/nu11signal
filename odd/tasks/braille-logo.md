# Braille logo in the terminal

## Objective
Replace the in-app null emblem (Braille slashed zero beside "N U 1 1 / S I G N A L" and thin bars) with a Braille rendering faithful to the official vector logo: the masked head with headphones and 4x4 LED eyes, the wordmark NU11SIGNAL written together, and five thick slanted bars centered under it.

## Design (approved by the user from Braille previews)
- Large (52 x 8 cells): head 16 x 8 cells drawn geometrically on a 32 x 32 dot grid; LEDs one dot each at pitch 2, slanted toward the center; wordmark in a 6 x 8 dot bold font (pitch 7) on rows 3-4; bars on rows 5-6.
- Compact (24 x 6 cells): head 12 x 6 cells (3 x 3 LEDs), NU11SIGNAL as plain text on row 2, `⡾⡾⡾⡾⡾` bars on row 3.
- Colors follow the logo per theme: primary (frame, headphones, top LED row, SIGNAL, bars) and secondary (NU11, other LED rows). NIGHT CITY #FF5F57/#5EF6FF, BLUE #347AFF/#5CE1FF, MATRIX #00C832/#00FF41, ROSE #F095C8/#FFB1DD, NEON ROSE #F43888/#FF4F9A.
- Generator kept outside the repo (scratchpad); the art is committed as data.

## Tasks
- [ ] T1 (delegated writer: emblem, theme, splash/idle layout, goldens, readmeart, docs): swap the emblem art and its coloring.

## Checks
- `go vet ./...`, `gofmt -l .`, `go test ./...` (goldens regenerated and reviewed), `go run ./tools/readmeart` leaves no stale assets.
- Visual check of boot splash, idle emblem (expanded and 80x24) and `--version` output.

## Progress
