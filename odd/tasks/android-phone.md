# Android phone and tablet
Make the Android TV app usable on Android phones and tablets, from the same APK.
Branch feat/android-phone from main 1eb8142; delivery as PRs and a later release.

## Specs
1. S1 — User: "ok, vamos hacer la version para android." Clarified: "App para celulares y tablets Android".
2. S2 — User chose "Una sola app": the same APK adapts to the device; one updater, one signature, the current package name.
3. S3 — User chose "Horizontal y vertical": the app works in both orientations on a phone.
4. S4 — User chose "Tocar la interfaz + teclado del teléfono": a tap is a click, a swipe is the wheel, and the phone keyboard opens for search.
5. S5 — User chose "Tengo un celular Android y lo conecto": verification on the user's real phone over ADB.

## Tasks
- [ ] M1 — S1, S3: Terminal sizing by device: on a touch device the grid follows a readable cell size (not the TV's 161x44), recomputed on rotation; the TV keeps 161x44. Route: inline, test where testable + phone check. Commit: pending.
- [ ] M2 — S3: Portrait layout in the TUI: when the screen is tall and narrow, stack the now-playing panel (with the spectrum/emblem) over the list, filling the height, with the footer hints; landscape and desktop layouts unchanged. Route: inline, test-first with goldens. Commit: pending.
- [ ] M3 — S4: Touch input: tap → mouse click, vertical swipe → wheel, in the terminal; keyboard opens for search as today; Back gesture behaves like the TV's Back. Route: inline + phone check. Commit: pending.
- [ ] M4 — S1, S4: Apple Music sign-in on a phone uses direct touch on the page (no D-pad pointer); guide text for touch; update check reachable without a Menu key. Route: inline + phone check. Commit: pending.
- [ ] M5 — S1–S5: Docs, independent verification, reviews, PR, CI, merge; release after the user's phone test. Route: verifier + parent delivery. Commit: pending.

## Log
- L1: User: "ok, vamos hacer la version para android." Clarification answered: phones and tablets; one APK; landscape and portrait; tap/swipe/keyboard; test on the user's phone.
- L2: The manifest already has the LAUNCHER category and does not require a touchscreen or leanback, so the APK installs on phones. terminal.html fits a fixed 161x44 grid to the screen (unreadable on a phone). The TUI already handles mouse clicks and the wheel (internal/radio/mouse.go). MainActivity handles D-pad keys and an emulated sign-in pointer.
- L3: Rendered with the fake player: at 110x40 (landscape phone) the full layout is complete and fine; at 46x80 (portrait) the compact single-column layout uses 10 rows and leaves the rest empty, without spectrum, emblem or hints (fullMinWidth 60). Portrait needs a new tall layout (M2).
