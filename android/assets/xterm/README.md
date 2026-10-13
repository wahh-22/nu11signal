# Vendored xterm assets

MIT-licensed UMD/global builds, usable with plain script tags. All three upstream
licenses are included. Run `android/vendor-xterm.sh` to refresh these exact
versions; builds use only the checked-in assets.

| Package | Version | Source | SHA-512 (base64) |
| --- | --- | --- | --- |
| @xterm/xterm | 6.0.0 | https://registry.npmjs.org/@xterm/xterm/-/xterm-6.0.0.tgz | TQwDdQGtwwDt+2cgKDLn0IRaSxYu1tSUjgKarSDkUM0ZNiSRXFpjxEsvc/Zgc5kq5omJ+V0a8/kIM2WD3sMOYg== |
| @xterm/addon-fit | 0.11.0 | https://registry.npmjs.org/@xterm/addon-fit/-/addon-fit-0.11.0.tgz | jYcgT6xtVYhnhgxh3QgYDnnNMYTcf8ElbxxFzX0IZo+vabQqSPAjC3c1wJrKB5E19VwQei89QCiZZP86DCPF7g== |

| @xterm/addon-webgl | 0.19.0 | https://registry.npmjs.org/@xterm/addon-webgl/-/addon-webgl-0.19.0.tgz | b3fMOsyLVuCeNJWxolACEUED0vm7qC0cy4wRvf3oURSzDTYVQiGPhTnhWZwIHdvC48Y+oLhvYXnY4XDXPoJo6A== |

WebGL 0.19.0 and xterm 6.0.0 were released from the same upstream commit
`f447274f430fd22513f6adbf9862d19524471c04` (verified archive package metadata).
The addon's upstream README requires xterm v4+.

Only xterm.js, xterm.css, addon-fit.js, addon-webgl.js, and the three licenses are copied from
the verified archives. The vendoring script retains downloaded archives in
the ignored build directory for inspection.
