# Vendored xterm assets

MIT-licensed UMD/global builds, usable with plain script tags. Both upstream
licenses are included. Run `android/vendor-xterm.sh` to refresh these exact
versions; builds use only the checked-in assets.

| Package | Version | Source | SHA-512 (base64) |
| --- | --- | --- | --- |
| @xterm/xterm | 6.0.0 | https://registry.npmjs.org/@xterm/xterm/-/xterm-6.0.0.tgz | TQwDdQGtwwDt+2cgKDLn0IRaSxYu1tSUjgKarSDkUM0ZNiSRXFpjxEsvc/Zgc5kq5omJ+V0a8/kIM2WD3sMOYg== |
| @xterm/addon-fit | 0.11.0 | https://registry.npmjs.org/@xterm/addon-fit/-/addon-fit-0.11.0.tgz | jYcgT6xtVYhnhgxh3QgYDnnNMYTcf8ElbxxFzX0IZo+vabQqSPAjC3c1wJrKB5E19VwQei89QCiZZP86DCPF7g== |

Only xterm.js, xterm.css, addon-fit.js, and the two licenses are copied from
the verified archives. The vendoring script retains downloaded archives in
the ignored build directory for inspection.
