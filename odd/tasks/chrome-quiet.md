# Quiet chrome

## Objective
Drop the NIGHT CITY RADIO subtitle, push the key hint footer into the background, and research a more cyberpunk font.

## Tasks
- [x] C1 — Header shows the wordmark alone (`◢◤ NU11SIGNAL`), window title `NU11SIGNAL`; footer hints drawn all in the muted color (dim was unreadable). Route: inline (view.go + tests). RED: TestTheHeaderIsTheWordmarkAlone, TestTheKeyHintsSitInTheBackground failed; GREEN: `go test -race ./...` pass, vet/gofmt clean; goldens (incl. ANSI baselines) regenerated.
- [ ] C2 — Font: a TUI cannot pick its font (the terminal does: Ghostty, IosevkaTerm NF 14). Present cyberpunk monospace options to the user to set in Ghostty; decision pending.

## Progress
- Created 2026-10-01 on branch `feat/chrome-quiet` from main 4452172.
