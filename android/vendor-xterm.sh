#!/usr/bin/env bash
# Vendors verified MIT xterm distributions; APK builds never access npm.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$ROOT/build" "$ROOT/assets/xterm"
TMP="$(mktemp -d "$ROOT/build/vendor.XXXXXXXX")"
fetch() {
  local name="$1" url="$2" integrity="$3" actual
  curl --fail --location --silent --show-error "$url" -o "$TMP/$name.tgz"
  actual="$(openssl dgst -sha512 -binary "$TMP/$name.tgz" | openssl base64 -A)"
  [[ "$actual" == "$integrity" ]] || { echo "Integrity mismatch: $name" >&2; exit 1; }
  echo "Verified sha512: $name"
  mkdir -p "$TMP/$name"
  tar -xzf "$TMP/$name.tgz" -C "$TMP/$name"
}
fetch xterm https://registry.npmjs.org/@xterm/xterm/-/xterm-6.0.0.tgz TQwDdQGtwwDt+2cgKDLn0IRaSxYu1tSUjgKarSDkUM0ZNiSRXFpjxEsvc/Zgc5kq5omJ+V0a8/kIM2WD3sMOYg==
fetch fit https://registry.npmjs.org/@xterm/addon-fit/-/addon-fit-0.11.0.tgz jYcgT6xtVYhnhgxh3QgYDnnNMYTcf8ElbxxFzX0IZo+vabQqSPAjC3c1wJrKB5E19VwQei89QCiZZP86DCPF7g==
# These are the UMD/global builds, not the ES module distributions.
cp "$TMP/xterm/package/lib/xterm.js" "$ROOT/assets/xterm/xterm.js"
cp "$TMP/xterm/package/css/xterm.css" "$ROOT/assets/xterm/xterm.css"
cp "$TMP/fit/package/lib/addon-fit.js" "$ROOT/assets/xterm/addon-fit.js"
cp "$TMP/xterm/package/LICENSE" "$ROOT/assets/xterm/LICENSE-xterm"
cp "$TMP/fit/package/LICENSE" "$ROOT/assets/xterm/LICENSE-addon-fit"
printf 'Vendored assets: %s\n' "$ROOT/assets/xterm"
# Keep verified archives under the ignored build directory for inspection.
