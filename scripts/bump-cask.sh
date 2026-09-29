#!/usr/bin/env bash
# Updates the Nu11Signal Homebrew cask in a local checkout of the tap.
#
# Usage: scripts/bump-cask.sh VERSION [--push]      (VERSION: x.y.z or x.y.z-pre)
#
# Renders packaging/homebrew/nu11signal.rb.template into Casks/nu11signal.rb
# with VERSION and the sha256 from
# dist/nu11signal-VERSION-macos-universal.tar.gz.sha256 (written by
# scripts/release.sh), checks it with `ruby -c` and
# `brew audit --cask --strict`, and shows the git diff. Nothing is committed
# unless --push is given: then the change is committed as
# "chore: bump nu11signal to VERSION" and pushed.
#
# The tap checkout is $NU11SIGNAL_TAP_DIR (default: ../homebrew-tap next to
# this repository). It is cloned from $NU11SIGNAL_TAP_REPO (default
# https://github.com/wahh-22/homebrew-tap.git) when missing, fast-forwarded
# when present, and refused when it has uncommitted changes other than a cask
# rendered by an earlier run without --push.
#
# Upload the release archive to GitHub before pushing: the cask URL points at
# the v$VERSION release asset.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

step() { echo "==> $*"; }
die() {
  echo "error: $*" >&2
  exit 1
}

PUSH=0
VERSION=""
for arg in "$@"; do
  case "$arg" in
    --push) PUSH=1 ;;
    -h | --help)
      sed -n '2,/^[^#]/s/^# \{0,1\}//p' "$0"
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

[[ -n "$VERSION" ]] || die "usage: scripts/bump-cask.sh VERSION [--push]"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] ||
  die "VERSION must be semver (x.y.z or x.y.z-pre), got: $VERSION"

for tool in git ruby brew; do
  command -v "$tool" >/dev/null 2>&1 || die "required tool not found: $tool"
done

TEMPLATE="$ROOT/packaging/homebrew/nu11signal.rb.template"
ARCHIVE_NAME="nu11signal-$VERSION-macos-universal.tar.gz"
SHA_FILE="$ROOT/dist/$ARCHIVE_NAME.sha256"
TAP_REPO="${NU11SIGNAL_TAP_REPO:-https://github.com/wahh-22/homebrew-tap.git}"
TAP_DIR="${NU11SIGNAL_TAP_DIR:-$ROOT/../homebrew-tap}"
CASK_REL="Casks/nu11signal.rb"

# --- Checksum --------------------------------------------------------------

[[ -f "$SHA_FILE" ]] ||
  die "checksum not found: $SHA_FILE (build the release first: make release VERSION=$VERSION)"
read -r SHA256 SHA_NAME <"$SHA_FILE" || true
[[ "$SHA256" =~ ^[0-9a-f]{64}$ ]] || die "no sha256 digest in $SHA_FILE"
[[ "$SHA_NAME" == "$ARCHIVE_NAME" ]] ||
  die "$SHA_FILE is for ${SHA_NAME:-an unnamed file}, not $ARCHIVE_NAME"
echo "    sha256 $SHA256 ($ARCHIVE_NAME)"

# --- Tap checkout ----------------------------------------------------------

if [[ -d "$TAP_DIR/.git" ]]; then
  TAP_DIR="$(cd "$TAP_DIR" && pwd)"
  step "Updating the tap checkout $TAP_DIR"
  # A cask left by an earlier run without --push is regenerated below.
  [[ -z "$(git -C "$TAP_DIR" status --porcelain -- . ":(exclude)$CASK_REL")" ]] ||
    die "the tap checkout $TAP_DIR has uncommitted changes; commit or discard them first"
  git -C "$TAP_DIR" pull --ff-only --quiet
elif [[ -e "$TAP_DIR" ]]; then
  die "$TAP_DIR exists but is not a git checkout; set NU11SIGNAL_TAP_DIR"
else
  step "Cloning $TAP_REPO into $TAP_DIR"
  git clone --quiet "$TAP_REPO" "$TAP_DIR"
  TAP_DIR="$(cd "$TAP_DIR" && pwd)"
fi

# --- Render ------------------------------------------------------------------

step "Rendering $CASK_REL for $VERSION"
mkdir -p "$TAP_DIR/Casks"
# Drop the template's leading comment block (up to and including the first
# line that is not a comment), then fill in the placeholders.
awk -v version="$VERSION" -v sha="$SHA256" '
  !started && /^#/ { next }
  { started = 1; gsub(/__VERSION__/, version); gsub(/__SHA256__/, sha); print }
' "$TEMPLATE" >"$TAP_DIR/$CASK_REL"
if grep -q '__[A-Z0-9]*__' "$TAP_DIR/$CASK_REL"; then
  die "unfilled placeholder left in $TAP_DIR/$CASK_REL"
fi

# --- Checks ------------------------------------------------------------------

step "Checking Ruby syntax"
ruby -c "$TAP_DIR/$CASK_REL" >/dev/null

# brew audit only takes cask names, and reads a tap's working tree, so the
# checkout is linked in as a temporary tap for the audit.
step "Auditing with brew audit --cask --strict"
TAPS="$(brew --repository)/Library/Taps"
AUDIT_USER="nu11signal-bump-$$"
AUDIT_TAP="$TAPS/$AUDIT_USER/homebrew-local"
cleanup() { rm -rf "${TAPS:?}/$AUDIT_USER"; }
trap cleanup EXIT
mkdir -p "$TAPS/$AUDIT_USER"
ln -s "$TAP_DIR" "$AUDIT_TAP"
HOMEBREW_NO_AUTO_UPDATE=1 brew audit --cask --strict "$AUDIT_USER/local/nu11signal"
cleanup
trap - EXIT

step "Changes in $TAP_DIR"
git -C "$TAP_DIR" add -N "$CASK_REL"
if git -C "$TAP_DIR" diff --quiet -- "$CASK_REL"; then
  echo "    $CASK_REL is already at $VERSION ($SHA256); nothing to commit"
  exit 0
fi
git -C "$TAP_DIR" --no-pager diff -- "$CASK_REL"

if [[ "$PUSH" != 1 ]]; then
  echo "Not pushed. Review, then rerun with --push (make cask VERSION=$VERSION PUSH=1)."
  exit 0
fi

# --- Publish -----------------------------------------------------------------

step "Committing and pushing"
git -C "$TAP_DIR" add -- "$CASK_REL"
git -C "$TAP_DIR" commit --quiet -m "chore: bump nu11signal to $VERSION"
git -C "$TAP_DIR" -c credential.helper= -c 'credential.helper=!gh auth git-credential' push --quiet
echo "Pushed: brew update && brew upgrade --cask nu11signal"
