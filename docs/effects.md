# Signal effects and rain

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

The glitch bursts, the boot and shutdown splashes, content intros, the data rain visualizer, and the idle emblem.

## Signal effects

The screen now and then loses the signal: every 10–22 s a short glitch
burst of 0.6–1 s tears one to three rows 1–2 cells sideways, corrupts a
few cells (4–10, twice as many on NO SIGNAL) and, on about one frame in
three, runs a static bar across a row, all changing every frame; about one burst in four also flashes a bold
red `NO SIGNAL` framed in red static for its whole length. A real message
on the status line is never glitched. The effects are drawn over the
frame, so clicks and keys work during a burst.

At launch nu11signal boots: for about 1.5 s the body shows the null
emblem (compact, or its text alone, on a small terminal) with
`BOOTING NU11SIGNAL...`, spaced out and bright, under it, glitching from the first frame (torn
emblem rows, noise cells, now and then a static bar, at about 15 fps), then
the normal UI scrambles in, whether Apple Music has linked yet or not
(`LINKING` stays in the header until it does). Any key or click skips the
boot (the key does nothing else, but `q` and `ctrl+c` ask to quit); refused
access shows the access error at once. With the effects off the boot splash
shows still.

New content scrambles in: when text appears that was not on screen
(another tab or page, a list or search results arriving, the ADD TO
PLAYLIST picker or NEW PLAYLIST editor opening, the KEYS or SETTINGS
overlay opening or closing, a new artist, album or feed in NOW PLAYING),
about half of its new characters (a seeded pick) show
light glyphs, uppercase letters, digits and a few thin symbols, each
keeping its color. They hold for a moment, then resolve left to right
within about 0.9 s, quickly at first and settling gently at the end; the
glyphs drift every ~140 ms, each cell at its own moment, instead of jumping
all at once. Only text that changed intros: the clock, progress, volume and
rain never do, moving the
cursor (in a list or in SETTINGS) or scrolling a list does not, and the SEARCH input and the playlist
name never scramble while you type (live results intro once as they
arrive). Intros follow the same switch as the effects.

`x` turns them off or on; `nu11signal --calm` (or `NU11SIGNAL_CALM=1`)
starts with them off. They pause while the SEARCH input or a NEW PLAYLIST
name takes the keys (intros keep running, off the input line), and on the
tiny layout. They add no timer: the animation tick sleeps until the next
burst is due; a burst runs at about 15 fps and an intro at 20 fps. The terminal is redrawn at most 20 times a second, not Bubble
Tea's default 60, to keep the process's wakeups (and battery use) low.

## Rain

The spectrum area at the bottom of NOW PLAYING draws data rain: hex digits
and half-width katakana fall down every other column, a bright head over a
trail that dims, in the spectrum's colors (yellow, bold red, red, dim red).
In app volume mode (see [Spectrum](audio.md#spectrum)) it plays the music:

- Each column follows the band under it through a curve that keeps quiet
  bands completely dry and stands the loud ones out: the louder the band,
  the more often drops start there, the faster and longer they fall, and
  the brighter their heads burn, up to yellow. Silence is dry.
- A hit (a band jumping over its recent average, the bass counting more)
  bursts: a bass hit sends a wave of new drops across every sounding
  column, a higher one under its own band, as many as the hit is strong,
  and every head flashes brighter for a few frames. A steady passage never
  flashes.
- The waveform's loudness scales how often drops start.

Without readings (system volume, `--demo`, macOS before 15) it drizzles
slowly and dimly instead. When paused the rain holds still and the
animation tick slows down to once a second.

While no music plays (paused, stopped, nothing loaded) the spectrum area
shows the null emblem with `N U 1 1 / S I G N A L` beside it, centered,
in place of the rain: the large emblem where it fits (the expanded player
included), else the compact one, else the emblem without its text, else
nothing, on blank cells. With the effects on the emblem glitches all the
time, softly: every frame zero to two block cells flicker over it and, on
about one frame in six, one emblem row tears a cell sideways. The tick
runs at about 6.7 fps (150 ms) while the emblem is shown with the effects
on, instead of once a second; with the effects off the emblem is still and
the tick stays at once a second. The emblem and the glitch come from the
seed and the clock, and the signal effects still run over them.

When music starts or stops the area does not cut: over 0.7 s the emblem
breaks up into the rain, or the rain settles into the emblem. Each cell
turns at its own seeded moment, biased so the emblem leaves from the
area's edges inward and comes back from its middle outward; the cells at
the moving edge glitch, block noise over the emblem and bright rain
glyphs elsewhere. The rain keeps falling and the emblem keeps its glitch
underneath, the tick runs at the burst's pace (about 15 fps) until the
switch ends, and a flip halfway turns it around from where it stands.
With the effects off, during the boot, on the player's first report and
in the compact and tiny layouts (no rain area) it cuts as before. Playing
again brings the rain back, its drops where they were.

The other visualizers (bars, oscilloscope, synthwave, random) and the `v`
key are gone. A `nu11signal/config.json` under `os.UserConfigDir()`
(`~/Library/Application Support/nu11signal/config.json` on macOS) is still
read once at startup (it also holds the theme, see [Settings](usage.md#settings)),
but its `"visualizer"` value,
whatever it names, is accepted and ignored without a notice; a file that is
not valid JSON says so once on the status line.
