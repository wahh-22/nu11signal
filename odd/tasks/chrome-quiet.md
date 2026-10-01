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
- [x] C2a — User picked Kode Mono (2026-10-01). A TUI cannot pick its font (the terminal draws the glyphs; no portable escape; Ghostty-only window rejected because not every user has Ghostty). The cask now `depends_on cask: "font-kode-mono"` with a caveat on setting the terminal font; README Install gains a Font section. Route: inline. Checks: `make test-scripts` 34 passed; rendered cask `ruby -c` Syntax OK. Takes effect with the next release.
- [ ] C2b — Block-letter wordmark mockups shown to the user; decision pending.
- [x] C3 — User 2026-10-01: keep the text wordmark (block logo declined), centered, without the ◢◤ before it; the nav bar's leading `▓▒░` becomes `◢◤◢◤`. At 80 columns the status at the right leaves the wordmark centered with a 2-cell gap; narrower it slides left. Route: inline (view.go + chrome_test). RED: TestTheWordmarkIsCenteredAndTheNavLeadsWithSlants failed; GREEN: `go test -race ./...` pass, vet/gofmt clean; goldens and ANSI baselines regenerated.
