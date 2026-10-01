# Install

[← Back to the README](../README.md) · [Documentation index](../README.md#documentation)

Install a signed, notarized build and set up the font the UI is designed with. To build from source instead, see [Building from source](building.md).

## Download

Signed, notarized builds (macOS 14 or later, Apple Silicon and Intel) are
published on [GitHub Releases](https://github.com/wahh-22/nu11signal/releases).
No Apple Developer account is needed to run them.

With [Homebrew](https://brew.sh):

```sh
brew install --cask wahh-22/tap/nu11signal
```

Or manually from a release archive:

```sh
tar -xzf nu11signal-<version>-macos-universal.tar.gz
nu11signal-<version>/bin/nu11signal          # keep bin/ and libexec/ together
nu11signal-<version>/bin/nu11signal --version
```

Symlink `bin/nu11signal` onto your `PATH` if you like; the helper is found
through the symlink. The cask lives in
[wahh-22/homebrew-tap](https://github.com/wahh-22/homebrew-tap) and is generated
from `packaging/homebrew/nu11signal.rb.template`. The first launch asks for
Apple Music access.

## Font

The UI is designed with [Kode Mono](https://fonts.google.com/specimen/Kode+Mono),
which the cask installs (`depends_on cask: "font-kode-mono"`, in releases
after v0.2.1; by hand:
`brew install --cask font-kode-mono`). A terminal UI cannot choose its font:
the terminal draws every character with the font it is set to, so Kode Mono
shows once your terminal uses it, for example `font-family = Kode Mono` in
Ghostty's config or the profile font in Terminal.app or iTerm2. Any
monospaced font works; the frames, blocks and rain glyphs look the same in
all of them.
