# NOW PLAYING emblem

## Objective
Try the null emblem fixed in the free space at the right of the song info in NOW PLAYING (user screenshot 2026-10-01, expanded player in BLUE), always visible, with the soft glitch of the idle emblem.

## Tasks
- [x] N1 — Draw the compact emblem (with its `N U 1 1 / S I G N A L / ◢◤…` text when it fits, else the emblem alone) right-aligned in the info rows under the title's `[<3]` button (artist, album and the blank row before the progress bar), only where it does not overlap the artist/album text (no truncation of the song info; skip when there is no room). Always on, playing or not; soft glitch on the idle schedule (every 4–7 s, 150–250 ms, block-only, within its own cells); still with effects off. Theme colors. Route: delegated writer.

## Progress
- Created 2026-10-01 on branch `feat/np-emblem` from main e32d638.
- N1 implemented (delegated writer, route: delegated direct; 2+ non-trivial files): `internal/radio/npemblem.go` (placement, schedule, draw), `nowPlaying` draws it, `introRegions` stops the artist/album spans at the block, `tickInterval`/`onTick` drive its own `npGlitch` schedule (seed `mix(seed, saltNPGlitch)`, look `saltNPLook`; salts 701/702). RED observed on the new `npemblem_test.go` tests, then GREEN; goldens and README screens regenerated. Side panel at 80x24 shows the emblem alone beside SAMURAI. Commit pending (parent).
- N1 commit `3f1091a` (assets refreshed in a separate commit to stay under the reviewer budget). Review (main..3f1091a): high, 802 lines, consent granted, 4 lenses, lineage `review-c6f8f202a1709549`, APPROVED, acknowledged (burned). Advisories (not scheduled): placement computed twice (draw gate vs npEmblem), duplicated block width, compact test uses the helper only, salt base naming.
