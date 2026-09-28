#!/usr/bin/env bash
# Builds a signed, notarized soul-king release for macOS (arm64 + x86_64).
#
# Usage: scripts/release.sh [--dry-run] VERSION      (VERSION: x.y.z or x.y.z-pre)
#
# Produces dist/soul-king-VERSION-macos-universal.tar.gz and its .sha256,
# containing soul-king-VERSION/{bin/soul-king, libexec/SoulKingHelper.app,
# LICENSE, README.md}.
#
# Requires (see README.md, "Releasing"):
#   - a "Developer ID Application" certificate for the team in the keychain;
#   - the Developer ID provisioning profile at $SOULKING_PROFILE
#     (default signing/SoulKing_Player_DeveloperID.provisionprofile);
#   - a notarytool keychain profile $SOULKING_NOTARY_PROFILE (default soulking-notary);
#   - no uncommitted changes to tracked files.
#
# --dry-run (or DRY_RUN=1) reports missing requirements without aborting,
# builds and assembles everything with ad-hoc signatures, and skips
# notarization, stapling, and archiving. Its output is not distributable.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DRY_RUN="${DRY_RUN:-0}"
VERSION=""
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    -h | --help)
      sed -n '2,20s/^# \{0,1\}//p' "$0"
      exit 0
      ;;
    -*)
      echo "error: unknown option: $arg" >&2
      exit 2
      ;;
    *)
      if [[ -n "$VERSION" ]]; then
        echo "error: more than one VERSION given" >&2
        exit 2
      fi
      VERSION="$arg"
      ;;
  esac
done

TEAM_ID="${SOULKING_TEAM_ID:-W6GZP998GQ}"
IDENTITY="${SOULKING_SIGN_IDENTITY:-Developer ID Application}"
PROFILE="${SOULKING_PROFILE:-signing/SoulKing_Player_DeveloperID.provisionprofile}"
NOTARY_PROFILE="${SOULKING_NOTARY_PROFILE:-soulking-notary}"
CLI_IDENTIFIER="${SOULKING_CLI_IDENTIFIER:-dev.wahh.soulking.cli}"

step() { echo "==> $*"; }
die() {
  echo "error: $*" >&2
  exit 1
}

# --- Preflight -------------------------------------------------------------

[[ -n "$VERSION" ]] || die "usage: scripts/release.sh [--dry-run] VERSION"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] ||
  die "VERSION must be semver (x.y.z or x.y.z-pre), got: $VERSION"

for tool in go swift xcrun codesign lipo ditto plutil shasum tar git security; do
  command -v "$tool" >/dev/null 2>&1 || die "required tool not found: $tool"
done

step "Preflight for soul-king $VERSION$([[ "$DRY_RUN" == 1 ]] && echo " (dry run)")"
missing=()

if security find-identity -v -p codesigning | grep -F "\"$IDENTITY" | grep -qF "($TEAM_ID)"; then
  echo "    ok: signing identity \"$IDENTITY\" for team $TEAM_ID"
else
  missing+=("signing identity \"$IDENTITY\" for team $TEAM_ID is not in the keychain.
      Create a Developer ID Application certificate (README.md, Releasing, step 1);
      check with: security find-identity -v -p codesigning")
fi

if [[ -f "$PROFILE" ]]; then
  echo "    ok: provisioning profile $PROFILE"
else
  missing+=("Developer ID provisioning profile not found: $PROFILE
      Create it for $TEAM_ID.dev.wahh.soulking.player and save it there (README.md, Releasing, step 2)")
fi

if xcrun notarytool history --keychain-profile "$NOTARY_PROFILE" >/dev/null 2>&1; then
  echo "    ok: notarytool keychain profile $NOTARY_PROFILE"
else
  missing+=("notarytool keychain profile \"$NOTARY_PROFILE\" is missing or its credentials are rejected.
      Run: xcrun notarytool store-credentials $NOTARY_PROFILE --apple-id <id> --team-id $TEAM_ID
      (README.md, Releasing, step 3)")
fi

if [[ -z "$(git status --porcelain --untracked-files=no)" ]]; then
  echo "    ok: no uncommitted changes to tracked files ($(git rev-parse --short HEAD))"
else
  missing+=("the working tree has uncommitted changes to tracked files; commit or stash them (git status)")
fi

if ((${#missing[@]} > 0)); then
  for m in "${missing[@]}"; do
    echo "  missing: $m" >&2
  done
  if [[ "$DRY_RUN" != 1 ]]; then
    die "preflight failed: ${#missing[@]} requirement(s) missing (see above); nothing was built"
  fi
  echo "    dry run: continuing with ad-hoc signatures" >&2
fi

if [[ "$DRY_RUN" == 1 ]]; then
  IDENTITY="-" # ad hoc: never pass a dry-run build off as a release
fi

# --- Build -----------------------------------------------------------------

NAME="soul-king-$VERSION"
DIST="$ROOT/dist"
STAGE="$DIST/$NAME"
ARCHIVE="$DIST/$NAME-macos-universal.tar.gz"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/soulking-release.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT

rm -rf "$STAGE" "$ARCHIVE" "$ARCHIVE.sha256"
mkdir -p "$STAGE/bin" "$STAGE/libexec"

step "Building universal soul-king $VERSION"
for arch in arm64 amd64; do
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o "$WORK_DIR/soul-king-$arch" ./cmd/soul-king
done
lipo -create -output "$STAGE/bin/soul-king" "$WORK_DIR/soul-king-arm64" "$WORK_DIR/soul-king-amd64"

step "Signing soul-king with \"$IDENTITY\""
if [[ "$IDENTITY" == "-" ]]; then
  codesign --force --options runtime --timestamp=none --identifier "$CLI_IDENTIFIER" \
    --sign - "$STAGE/bin/soul-king"
else
  codesign --force --options runtime --timestamp --identifier "$CLI_IDENTIFIER" \
    --sign "$IDENTITY" "$STAGE/bin/soul-king"
fi
codesign --verify --strict --verbose=2 "$STAGE/bin/soul-king"

step "Building universal SoulKingHelper.app"
SOULKING_SIGN_MODE=release \
  SOULKING_SIGN_IDENTITY="$IDENTITY" \
  SOULKING_PROFILE="$PROFILE" \
  SOULKING_BUILD_DIR="$STAGE/libexec" \
  SOULKING_VERSION="$VERSION" \
  ./helper/build.sh

cp LICENSE README.md "$STAGE/"

if [[ "$DRY_RUN" == 1 ]]; then
  step "Dry run: skipping notarization, stapling, and archiving"
  lipo -archs "$STAGE/bin/soul-king"
  lipo -archs "$STAGE/libexec/SoulKingHelper.app/Contents/MacOS/soulking-helper"
  echo "Assembled (ad hoc, not distributable): $STAGE"
  exit 0
fi

# --- Notarize --------------------------------------------------------------

step "Submitting to the notary service (this can take several minutes)"
ZIP="$WORK_DIR/$NAME.zip"
ditto -c -k --keepParent "$STAGE" "$ZIP"
RESULT="$WORK_DIR/notary.json"
xcrun notarytool submit "$ZIP" --keychain-profile "$NOTARY_PROFILE" --wait \
  --output-format json >"$RESULT" || true
STATUS="$(plutil -extract status raw -o - "$RESULT" 2>/dev/null || echo unknown)"
SUBMISSION="$(plutil -extract id raw -o - "$RESULT" 2>/dev/null || echo unknown)"
if [[ "$STATUS" != "Accepted" ]]; then
  cat "$RESULT" >&2 || true
  echo "error: notarization status: $STATUS (submission $SUBMISSION)" >&2
  echo "       See why: xcrun notarytool log $SUBMISSION --keychain-profile $NOTARY_PROFILE" >&2
  exit 1
fi
echo "    notarized: submission $SUBMISSION"

step "Stapling the helper app"
xcrun stapler staple "$STAGE/libexec/SoulKingHelper.app"
xcrun stapler validate "$STAGE/libexec/SoulKingHelper.app"

# --- Archive ---------------------------------------------------------------

step "Archiving $ARCHIVE"
# COPYFILE_DISABLE keeps AppleDouble (._*) files out of the archive.
COPYFILE_DISABLE=1 tar -C "$DIST" -czf "$ARCHIVE" "$NAME"
(cd "$DIST" && shasum -a 256 "$(basename "$ARCHIVE")" >"$(basename "$ARCHIVE").sha256")

# --- Final checks ------------------------------------------------------------

step "Verifying the archived contents"
CHECK_DIR="$WORK_DIR/check"
mkdir -p "$CHECK_DIR"
tar -C "$CHECK_DIR" -xzf "$ARCHIVE"
spctl --assess --type execute -vv "$CHECK_DIR/$NAME/libexec/SoulKingHelper.app"
codesign --verify --strict --deep --verbose=2 "$CHECK_DIR/$NAME/libexec/SoulKingHelper.app"
codesign --verify --strict --verbose=2 "$CHECK_DIR/$NAME/bin/soul-king"
[[ "$("$CHECK_DIR/$NAME/bin/soul-king" --version)" == "$VERSION" ]] ||
  die "archived soul-king --version does not print $VERSION"

step "Release ready"
cat "$ARCHIVE.sha256"
echo "Next: upload $(basename "$ARCHIVE") to the v$VERSION GitHub release and put the sha256 in the cask."
