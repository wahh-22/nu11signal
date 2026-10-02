<a id="top"></a>

<div align="center">
  <img src="docs/assets/brand/nu11signal-banner.svg" width="1200" alt="nu11signal: the null emblem beside N U 1 1 / S I G N A L">
</div>

<h1 align="center">nu11signal</h1>

<p align="center"><strong>A cyberpunk car radio for Apple Music, in your terminal.</strong></p>

<p align="center">
  <a href="https://github.com/wahh-22/nu11signal/releases/latest"><img src="https://img.shields.io/github/v/release/wahh-22/nu11signal?style=for-the-badge&labelColor=0A0A0A&color=FF5F57" alt="Latest release"></a>
  <a href="docs/install.md"><img src="https://img.shields.io/badge/macOS-14%2B-5EF6FF?style=for-the-badge&labelColor=0A0A0A&logo=apple&logoColor=5EF6FF" alt="macOS 14 or later"></a>
  <a href="docs/install.md"><img src="https://img.shields.io/badge/brew-wahh--22%2Ftap-FCEE0A?style=for-the-badge&labelColor=0A0A0A&logo=homebrew&logoColor=FCEE0A" alt="Homebrew cask wahh-22/tap/nu11signal"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wahh-22/nu11signal?style=for-the-badge&labelColor=0A0A0A&color=FF5F57" alt="MIT license"></a>
  <a href="https://github.com/wahh-22/nu11signal/stargazers"><img src="https://img.shields.io/github/stars/wahh-22/nu11signal?style=for-the-badge&labelColor=0A0A0A&color=FF5F57" alt="GitHub stars"></a>
  <a href="https://github.com/wahh-22/nu11signal/commits/main"><img src="https://img.shields.io/github/last-commit/wahh-22/nu11signal?style=for-the-badge&labelColor=0A0A0A&color=9A3B37" alt="Last commit"></a>
</p>

<p align="center">
  <a href="https://github.com/sponsors/wahh-22"><img src="https://img.shields.io/badge/sponsor-%E2%99%A5-FF5F57?style=for-the-badge&labelColor=0A0A0A&logo=githubsponsors&logoColor=FF5F57" alt="Sponsor on GitHub"></a>
  <a href="https://buymeacoffee.com/wahh.dev"><img src="https://img.shields.io/badge/buy%20me%20a%20coffee-%E2%98%95-FCEE0A?style=for-the-badge&labelColor=0A0A0A&logo=buymeacoffee&logoColor=FCEE0A" alt="Buy me a coffee"></a>
</p>

<p align="center">
  <strong>
    <a href="https://nu11signal.wahh.dev/">Website</a>
    &nbsp;·&nbsp;
    <a href="#get-started">Quickstart</a>
    &nbsp;·&nbsp;
    <a href="#documentation">Docs</a>
    &nbsp;·&nbsp;
    <a href="https://github.com/wahh-22/nu11signal/releases">Releases</a>
    &nbsp;·&nbsp;
    <a href="#support-the-signal">Support</a>
  </strong>
</p>

<br>

<p align="center">Your library playlists sit on a pseudo FM dial, beside a now-playing panel, a volume readout, and data rain that plays the music.<br><strong>Search and browse the Apple Music catalog, edit playlists, and love songs without leaving the terminal.</strong></p>

<p align="center"><sub>It plays through a tiny windowless MusicKit helper (about 31 MB RSS measured during playback, near 0% CPU) instead of a browser.</sub></p>

<p align="center">
  <img src="docs/assets/screens/night-city.svg" width="840" alt="nu11signal in the NIGHT CITY theme: the PLAYLISTS dial on the left, NOW PLAYING with transport buttons and data rain on the right">
</p>

> macOS only. Requires an Apple Music subscription. Not affiliated with Apple.

## Support the signal

Nu11Signal is free and open source. If it plays in your terminal and you
want to keep it on air, you can
[sponsor on GitHub](https://github.com/sponsors/wahh-22) or
[buy me a coffee](https://buymeacoffee.com/wahh.dev).

## Features

---

### Apple Music, as a terminal radio

A Bubble Tea UI in Go talks JSON lines to a signed Swift helper that plays
through MusicKit's `ApplicationMusicPlayer`. No browser and no app window: the
helper is windowless and idles near 0% CPU.

**[Docs →](docs/architecture.md)**

---

### Your own music files too

mp3, flac, ogg and wav from `~/Music` (or the folders you name) play beside
Apple Music under a LOCAL section, folders and m3u files as playlists, with
the rain driven by the music itself. On Linux, built from source, they play
on their own.

**[Docs →](docs/usage.md#local-files)**

---

### Browse the catalog like Apple Music

<img width="100%" src="docs/assets/screens/search.svg" alt="SEARCH with live suggestions, matching artists and songs">

Views stack like Apple Music's: PLAYLISTS → SEARCH → RESULTS → ARTIST → ALBUM,
SONG, or PLAYLIST. Live suggestions as you type, recent searches, artist pages
with top songs and discography, album and playlist pages. A song picked from a
list plays with the rest of that list queued around it.

**[Docs →](docs/usage.md#browsing-the-catalog)**

---

### Playlists and favorites

Love a song (`l`, the `<3` mark), add it to a playlist (`a`), or create a new
playlist on the spot, from any song row or the song playing. Every page reads
its songs' favorite state in one call, so the hearts show at once.

**[Docs →](docs/usage.md#editing-the-library)**

---

### Data rain that plays the music

<img width="100%" src="docs/assets/screens/expanded.svg" alt="The expanded player with the data rain under the transport controls">

Hex digits and half-width katakana fall under NOW PLAYING. With app volume on
macOS 15 or later the helper measures the real spectrum: loud bands rain
harder, bass hits send a wave of drops, and silence is dry. Otherwise the rain
drizzles decoratively with the playback state.

**[Docs →](docs/effects.md#rain)**

---

### Signal glitches, intros, and a boot splash

<img width="100%" src="docs/assets/screens/no-signal.svg" alt="A glitch burst tearing rows and flashing NO SIGNAL over the UI">

Every so often the screen loses the signal: torn rows, corrupted cells, static
bars, and now and then a red `NO SIGNAL`. New content scrambles in, and the app
boots and shuts down over the null emblem. `x` toggles the effects;
`nu11signal --calm` starts without them.

<img width="100%" src="docs/assets/screens/boot.svg" alt="The boot splash: the null emblem over BOOTING NU11SIGNAL">

**[Docs →](docs/effects.md#signal-effects)**

---

### A HUD you can drive with keys or the mouse

<img width="100%" src="docs/assets/screens/keys.svg" alt="The KEYS overlay listing every binding by group">

Bracketed HUD buttons for transport, loop, volume, and expand; every control
answers to both the keyboard focus and a click, and the wheel scrolls the
lists. `?` opens KEYS, every binding in one panel, and quitting always asks
first.

**[Docs →](docs/usage.md#keys)** &nbsp;·&nbsp; **[Mouse →](docs/usage.md#mouse)**

---

### Themes

<img width="100%" src="docs/assets/screens/blue.svg" alt="nu11signal in the BLUE theme">

`s` opens SETTINGS: NIGHT CITY (neon red, cyan, and yellow) or BLUE (electric
blue and violet from the gentleman-blue palette). The whole UI recolors at
once, and the choice is saved for the next start.

**[Docs →](docs/usage.md#settings)**

---

### Its own volume

nu11signal's `VOL` is independent of the system volume: the helper taps its
own audio with a Core Audio process tap (macOS 14.2+) and plays it through a
private aggregate device, falling back to the system volume (`SYS`) when it
cannot. Set `NU11SIGNAL_VOLUME_MODE=system` to opt out.

**[Docs →](docs/audio.md#app-volume)**

---

## Get started

Install the signed, notarized build (macOS 14 or later, Apple Silicon and
Intel) with [Homebrew](https://brew.sh):

```sh
brew install --cask wahh-22/tap/nu11signal
nu11signal
```

The first launch asks for Apple Music access. The UI is designed with
[Kode Mono](https://fonts.google.com/specimen/Kode+Mono): the cask installs it
(releases after v0.2.1; by hand, `brew install --cask font-kode-mono`). Set
your terminal's font to it for the intended look; any monospaced font works. Release archives for a manual install are on
[GitHub Releases](https://github.com/wahh-22/nu11signal/releases); see
[Install](docs/install.md).

A release build checks GitHub once a day for a newer release and shows the
upgrade command on its status line; turn it off with
`NU11SIGNAL_NO_UPDATE_CHECK=1` or `"update_check": false` (see
[Update check](docs/usage.md#update-check)).

From a source checkout, try the UI against a simulated player, or build and
sign your own (see [Building from source](docs/building.md)):

```sh
make demo          # try the UI with a simulated player (no Apple Music, no signing)
make build         # signed helper + Go binary (needs the one-time setup)
bin/nu11signal     # play for real
```

## Documentation

| Guide | What it covers |
|-------|----------------|
| [Install](docs/install.md) | Homebrew, release archives, the Kode Mono font |
| [Usage](docs/usage.md) | Browsing the catalog, editing the library, every key, the mouse, settings, the update check |
| [Signal effects and rain](docs/effects.md) | Glitch bursts, boot and shutdown splashes, content intros, the rain visualizer |
| [App volume and spectrum](docs/audio.md) | The Core Audio tap behind `VOL`, and the live spectrum that drives the rain |
| [Architecture](docs/architecture.md) | Go UI and Swift helper, helper lookup, the JSON lines protocol |
| [Building from source](docs/building.md) | Requirements, one-time MusicKit signing setup, make targets |
| [Releasing](docs/releasing.md) | Signed, notarized releases and the Homebrew cask |
| [Troubleshooting](docs/troubleshooting.md) | Common failures and fixes |
| [Contributing](docs/contributing.md) | Development checks, regenerating the README art, repository notes |

## Contributing

Issues and pull requests are welcome. Start with `make demo`, and run
`make test` before opening a pull request; see
[Contributing](docs/contributing.md).

## License

MIT, see [LICENSE](LICENSE). Nu11Signal was formerly named soul-king; v0.1.0
was released under that name.

<p align="right"><a href="#top">Back to top ↑</a></p>
