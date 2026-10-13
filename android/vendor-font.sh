#!/usr/bin/env bash
# Vendor the pinned SIL OFL Kode Mono font; APK builds use local assets only.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
COMMIT=909667d11fe41a8384294d064950c48c53904d27
SOURCE="https://raw.githubusercontent.com/google/fonts/$COMMIT/ofl/kodemono"
SHA256=bd7b11b37524e03e270c7a1726f08cc52cb17e8ebc3f664f9d323dfd9277b311
mkdir -p "$ROOT/build" "$ROOT/assets/fonts"
TMP="$(mktemp -d "$ROOT/build/font.XXXXXXXX")"
curl --fail --location --silent --show-error "$SOURCE/KodeMono%5BROND%2Cwdth%2Cwght%5D.ttf" -o "$TMP/KodeMono.ttf"
actual="$(openssl dgst -sha256 "$TMP/KodeMono.ttf" | awk '{print $NF}')"
[[ "$actual" == "$SHA256" ]] || { echo 'Integrity mismatch: Kode Mono' >&2; exit 1; }
echo 'Verified sha256: Kode Mono'
curl --fail --location --silent --show-error "$SOURCE/OFL.txt" -o "$TMP/OFL.txt"
cp "$TMP/KodeMono.ttf" "$TMP/OFL.txt" "$ROOT/assets/fonts/"
printf 'Vendored font: %s\n' "$ROOT/assets/fonts"
# Retain downloads under the ignored build directory for inspection.
