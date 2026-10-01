# Quiet chrome

## Objective
Drop the NIGHT CITY RADIO subtitle, push the key hint footer into the background, and research a more cyberpunk font.

## Tasks
- [x] C1 — Header shows the wordmark alone (`◢◤ NU11SIGNAL`), window title `NU11SIGNAL`; footer hints drawn all in the muted color (dim was unreadable). Route: inline (view.go + tests). RED: TestTheHeaderIsTheWordmarkAlone, TestTheKeyHintsSitInTheBackground failed; GREEN: `go test -race ./...` pass, vet/gofmt clean; goldens (incl. ANSI baselines) regenerated.
- [ ] C2 — Font: a TUI cannot pick its font (the terminal does: Ghostty, IosevkaTerm NF 14). Present cyberpunk monospace options to the user to set in Ghostty; decision pending.

## Progress
- Created 2026-10-01 on branch `feat/chrome-quiet` from main 4452172.
- C1 review: commit `4185ab0`, medium, 141 lines, consent granted, consolidated lens, lineage `review-02301bc81a078fb2`, APPROVED, acknowledged (burned). Advisory: keyCap may be orphaned; header test covers only the wide variant.
- C2: font comparison published at https://claude.ai/artifact/Rs4T6S9nA2g4YeafVvqh72 (Kode Mono recommended); awaiting the user's pick.
