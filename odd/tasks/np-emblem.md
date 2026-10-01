# NOW PLAYING emblem

## Objective
Try the null emblem fixed in the free space at the right of the song info in NOW PLAYING (user screenshot 2026-10-01, expanded player in BLUE), always visible, with the soft glitch of the idle emblem.

## Tasks
- [ ] N1 — Draw the compact emblem (with its `N U 1 1 / S I G N A L / ◢◤…` text when it fits, else the emblem alone) right-aligned in the info rows under the title's `[<3]` button (artist, album and the blank row before the progress bar), only where it does not overlap the artist/album text (no truncation of the song info; skip when there is no room). Always on, playing or not; soft glitch on the idle schedule (every 4–7 s, 150–250 ms, block-only, within its own cells); still with effects off. Theme colors. Route: delegated writer.

## Progress
- Created 2026-10-01 on branch `feat/np-emblem` from main e32d638.
