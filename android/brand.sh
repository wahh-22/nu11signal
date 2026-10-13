#!/usr/bin/env bash
# Renders the Android TV launcher icon, home-screen banner, dialog mark and
# notification icon from the project's Blueshift vector logo (the default
# theme). Run after the logo changes; the PNGs are committed so APK builds need
# no SVG tooling. Requires rsvg-convert (Homebrew: librsvg) and perl.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
LOGO="$ROOT/../docs/assets/brand/logo/nu11signal-blueshift.svg"
INK='#05070F' # Blueshift ink, the app's default terminal background
# Head-and-headphones region of the 1650x520 logo, as a square viewBox.
HEAD='2 -4 528 528'
command -v rsvg-convert >/dev/null || { echo 'rsvg-convert is required (brew install librsvg)' >&2; exit 1; }
[[ -f "$LOGO" ]] || { echo "Missing logo: $LOGO" >&2; exit 1; }
mkdir -p "$ROOT/build"
TMP="$(mktemp -d "$ROOT/build/brand.XXXXXXXX")"
# The logo body without its root element, reused inside nested <svg> viewports.
perl -0777 -ne 's/^.*?<svg\b[^>]*>//s; s#</svg>\s*$##s; print' "$LOGO" >"$TMP/body.svg"

# compose NAME WIDTH HEIGHT BACKGROUND X Y W H VIEWBOX [FILTER]
compose() {
  local name="$1" width="$2" height="$3" bg="$4" x="$5" y="$6" w="$7" h="$8" box="$9" filter="${10:-}"
  {
    printf '<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s">\n' \
      "$width" "$height" "$width" "$height"
    # Bright logo parts become opaque white; the dark shell fades out.
    printf '<defs><filter id="mono" color-interpolation-filters="sRGB"><feColorMatrix type="matrix" '
    printf 'values="0 0 0 0 1 0 0 0 0 1 0 0 0 0 1 0.64 2.15 0.22 0 -0.05"/></filter></defs>\n'
    [[ "$bg" == none ]] || printf '<rect width="100%%" height="100%%" fill="%s"/>\n' "$bg"
    printf '<g%s><svg x="%s" y="%s" width="%s" height="%s" viewBox="%s">\n' \
      "${filter:+ filter=\"url(#$filter)\"}" "$x" "$y" "$w" "$h" "$box"
    cat "$TMP/body.svg"
    printf '</svg></g></svg>\n'
  } >"$TMP/$name.svg"
}

# render NAME BASE_DP: one PNG per density bucket from a BASE_DP-sized design.
render() {
  local name="$1" base="$2" bucket scale
  for bucket in mdpi:1 hdpi:1.5 xhdpi:2 xxhdpi:3 xxxhdpi:4; do
    scale="${bucket#*:}"
    mkdir -p "$ROOT/res/drawable-${bucket%%:*}"
    rsvg-convert -w "$(awk -v b="$base" -v s="$scale" 'BEGIN { printf "%d", b * s }')" \
      "$TMP/$name.svg" -o "$ROOT/res/drawable-${bucket%%:*}/$name.png"
  done
}

# Adaptive icon foreground: 108dp canvas, head inside the 66dp safe zone.
compose icon_foreground 108 108 none 21 21 66 66 "$HEAD"
render icon_foreground 108
# Dialog mark: the head alone, 48dp.
compose logo_mark 48 48 none 2 2 44 44 "$HEAD"
render logo_mark 48
# Status/notification icon: white silhouette of the head, 24dp.
compose notification 24 24 none 1 1 22 22 "$HEAD" mono
render notification 24
# TV home-screen banner: 320x180dp, designed at xhdpi (640x360 px).
compose banner 640 360 "$INK" 40 92 560 176 '0 0 1650 520'
mkdir -p "$ROOT/res/drawable-xhdpi"
rsvg-convert -w 640 -h 360 "$TMP/banner.svg" -o "$ROOT/res/drawable-xhdpi/banner.png"
printf 'Rendered Android brand assets from %s\n' "$LOGO"
# Intermediate SVGs remain under the ignored build directory for inspection.
