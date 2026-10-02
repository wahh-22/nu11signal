#!/usr/bin/env bash
# Updates the Nu11Signal Homebrew cask (macOS) and formula (macOS and Linux)
# in a local checkout of the tap.
#
# Usage: scripts/bump-cask.sh VERSION [--push]      (VERSION: x.y.z or x.y.z-pre)
#
# Renders packaging/homebrew/nu11signal.rb.template into Casks/nu11signal.rb
# with VERSION and the sha256 from
# dist/vVERSION/nu11signal-VERSION-macos-universal.tar.gz.sha256, and
# packaging/homebrew/nu11signal-formula.rb.template into Formula/nu11signal.rb
# with VERSION, the same macOS sha256, and the sha256 of
# nu11signal-VERSION-linux-{amd64,arm64}.tar.gz (all written by
# scripts/release.sh). The formula installs on macOS and Linux: it shares its
# name with the cask, so `brew install` without --cask resolves to it on
# macOS. It checks both with `ruby -c`, the cask with
# `brew audit --cask --strict`, and the formula with
# `brew audit --formula --strict` and `brew style` (the audit does not run
# every style cop). Then it shows the git diff.
# Nothing is committed unless --push is given: then the change is committed
# as "chore: bump nu11signal to VERSION" and pushed. An unchanged render
# creates no commit.
#
# The tap checkout is $NU11SIGNAL_TAP_DIR (default: ../homebrew-tap next to
# this repository). It is cloned from $NU11SIGNAL_TAP_REPO (default
# https://github.com/wahh-22/homebrew-tap.git) when missing, fast-forwarded
# when present and behind its upstream, and refused when it has diverged from
# its upstream or when any path other than Casks/nu11signal.rb and
# Formula/nu11signal.rb has uncommitted changes or untracked files (a cask or
# formula rendered by an earlier run without --push is fine).
#
# Resuming a failed push: when the checkout is ahead of its upstream with
# "chore: bump nu11signal to VERSION" commits that touch only the cask and the
# formula, and both match the render, --push pushes them instead of committing
# again; without --push the unpushed bump is reported. Any other local commit,
# or an unpushed bump that no longer matches the render, is refused.
#
# Upload the release archives to GitHub before pushing: the cask and formula
# URLs point at the v$VERSION release assets.
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
FORMULA_TEMPLATE="$ROOT/packaging/homebrew/nu11signal-formula.rb.template"
TAP_REPO="${NU11SIGNAL_TAP_REPO:-https://github.com/wahh-22/homebrew-tap.git}"
TAP_DIR="${NU11SIGNAL_TAP_DIR:-$ROOT/../homebrew-tap}"
CASK_REL="Casks/nu11signal.rb"
FORMULA_REL="Formula/nu11signal.rb"

# --- Checksums -------------------------------------------------------------

MACOS_ARCHIVE="nu11signal-$VERSION-macos-universal.tar.gz"
LINUX_ARCHIVES=("nu11signal-$VERSION-linux-amd64.tar.gz" "nu11signal-$VERSION-linux-arm64.tar.gz")

# check_checksum_files: every .sha256 file release.sh writes for VERSION
# exists; otherwise names each missing one and how to build it.
check_checksum_files() {
  local archive missing=() macos_missing=0 dist="$ROOT/dist/v$VERSION"
  for archive in "$MACOS_ARCHIVE" "${LINUX_ARCHIVES[@]}"; do
    if [[ ! -f "$dist/$archive.sha256" ]]; then
      missing+=("checksum not found: $dist/$archive.sha256")
      [[ "$archive" == "$MACOS_ARCHIVE" ]] && macos_missing=1
    fi
  done
  ((${#missing[@]} > 0)) || return 0
  printf 'error: %s\n' "${missing[@]}" >&2
  if [[ ! -e "$dist" ]]; then
    echo "       Build the release first: make release VERSION=$VERSION (macOS and Linux archives)" >&2
  elif [[ "$macos_missing" == 1 ]]; then
    echo "       Rebuild the release: make release VERSION=$VERSION FORCE=1 (macOS and Linux archives;" >&2
    echo "       make release-linux builds only the Linux ones)" >&2
  else
    echo "       Rebuild the release: make release VERSION=$VERSION FORCE=1 (macOS and Linux archives;" >&2
    echo "       make release-linux VERSION=$VERSION FORCE=1 would build the Linux ones but replace" >&2
    echo "       $dist without its macOS archive, which comes from another build)" >&2
  fi
  exit 1
}

# read_checksum ARCHIVE: prints the sha256 of ARCHIVE from
# dist/vVERSION/ARCHIVE.sha256, checking that the file names ARCHIVE.
read_checksum() {
  local archive="$1" file="$ROOT/dist/v$VERSION/$1.sha256" sha name
  read -r sha name <"$file" || true
  [[ "$sha" =~ ^[0-9a-f]{64}$ ]] || die "no sha256 digest in $file"
  [[ "$name" == "$archive" ]] || die "$file is for ${name:-an unnamed file}, not $archive"
  echo "    sha256 $sha ($archive)" >&2
  echo "$sha"
}

check_checksum_files
SHA256="$(read_checksum "$MACOS_ARCHIVE")"
SHA256_LINUX_AMD64="$(read_checksum "${LINUX_ARCHIVES[0]}")"
SHA256_LINUX_ARM64="$(read_checksum "${LINUX_ARCHIVES[1]}")"

# --- Tap checkout ----------------------------------------------------------

# commits N: "1 commit" or "N commits".
commits() { if (($1 == 1)); then echo "1 commit"; else echo "$1 commits"; fi; }

# set_upstream: UPSTREAM is the tracking branch of the tap checkout's branch.
UPSTREAM=""
set_upstream() {
  UPSTREAM="$(git -C "$TAP_DIR" rev-parse --abbrev-ref --symbolic-full-name '@{upstream}' 2>/dev/null)" ||
    die "the current branch of $TAP_DIR has no upstream; check it out on a tracking branch"
}

if [[ -d "$TAP_DIR/.git" ]]; then
  TAP_DIR="$(cd "$TAP_DIR" && pwd)"
  step "Updating the tap checkout $TAP_DIR"
  # Only the cask and the formula may differ (an earlier run without --push
  # leaves them rendered; they are regenerated below). Every other status
  # entry, including untracked files anywhere and renames, is refused.
  dirty="$(git -C "$TAP_DIR" status --porcelain=v1 --untracked-files=all |
    grep -vxE "([ MA][ MA]|\?\?) (${CASK_REL//./\\.}|${FORMULA_REL//./\\.})" || true)"
  [[ -z "$dirty" ]] ||
    die "the tap checkout $TAP_DIR has uncommitted changes other than $CASK_REL and $FORMULA_REL; commit or discard them first:
$dirty"
  set_upstream
  git -C "$TAP_DIR" fetch --quiet ||
    die "could not fetch $UPSTREAM into $TAP_DIR; check the network and the remote, then rerun"
  # Compare with the upstream before touching the checkout: only a checkout
  # that is behind (and not ahead) is fast-forwarded.
  read -r ahead behind <<<"$(git -C "$TAP_DIR" rev-list --left-right --count "HEAD...$UPSTREAM")"
  if ((ahead > 0 && behind > 0)); then
    die "the tap checkout $TAP_DIR has diverged from $UPSTREAM:
       the checkout has $(commits "$ahead") not on $UPSTREAM, and $UPSTREAM has $(commits "$behind") not in the checkout.
       Local commits:
$(git -C "$TAP_DIR" log --format='      %h %s' "$UPSTREAM..HEAD")
       If they are an earlier unpushed bump, drop them (git -C $TAP_DIR reset --hard $UPSTREAM) and rerun;
       to keep them, rebase them first (git -C $TAP_DIR pull --rebase), then rerun."
  elif ((behind > 0)); then
    echo "    behind $UPSTREAM by $(commits "$behind"); fast-forwarding"
    git -C "$TAP_DIR" merge --ff-only --quiet "$UPSTREAM" ||
      die "could not fast-forward $TAP_DIR to $UPSTREAM; resolve it by hand, then rerun"
  elif ((ahead > 0)); then
    echo "    ahead of $UPSTREAM by $(commits "$ahead") (checked below)"
  else
    echo "    up to date with $UPSTREAM"
  fi
elif [[ -e "$TAP_DIR" ]]; then
  die "$TAP_DIR exists but is not a git checkout; set NU11SIGNAL_TAP_DIR"
else
  step "Cloning $TAP_REPO into $TAP_DIR"
  git clone --quiet "$TAP_REPO" "$TAP_DIR"
  TAP_DIR="$(cd "$TAP_DIR" && pwd)"
  set_upstream
fi

# Commits left by an earlier --push whose push failed. Only bump commits for
# this VERSION that touch nothing but the cask and the formula can be resumed.
UNPUSHED="$(git -C "$TAP_DIR" log --format='%h %s' "$UPSTREAM..HEAD")"
if [[ -n "$UNPUSHED" ]]; then
  BUMP_SUBJECT="chore: bump nu11signal to $VERSION"
  DROP_HINT="git -C $TAP_DIR reset --hard $UPSTREAM"
  others=""
  while IFS= read -r commit; do
    [[ "${commit#* }" == "$BUMP_SUBJECT" ]] || others+="      $commit"$'\n'
  done <<<"$UNPUSHED"
  [[ -z "$others" ]] ||
    die "$TAP_DIR has unpushed commits on $UPSTREAM that are not bump commits for $VERSION (\"$BUMP_SUBJECT\"):
$others       Push them yourself, or drop them ($DROP_HINT), then rerun."
  # Net change of all unpushed commits against the upstream: it must be
  # limited to the cask and the formula.
  touched="$(git -C "$TAP_DIR" diff --name-only "$UPSTREAM" HEAD)"
  if [[ -z "$touched" ]]; then
    die "the unpushed bump commits for $VERSION in $TAP_DIR make no net change against $UPSTREAM
       (they cancel each other out); drop them ($DROP_HINT) and rerun"
  fi
  extra="$(grep -vxF -e "$CASK_REL" -e "$FORMULA_REL" <<<"$touched" || true)"
  [[ -z "$extra" ]] ||
    die "the unpushed bump commits for $VERSION in $TAP_DIR change files other than $CASK_REL and $FORMULA_REL:
$(sed 's/^/      /' <<<"$extra")
       A bump commit may change only the cask and the formula; drop them ($DROP_HINT), or fix them by hand, then rerun."
  echo "    unpushed bump commit(s) on $UPSTREAM:"
  printf '%s\n' "$UNPUSHED" | sed 's/^/      /'
fi

# --- Render ------------------------------------------------------------------

# render TEMPLATE OUT: drops the template's leading comment block, then fills
# in __VERSION__, __SHA256__, __SHA256_LINUX_AMD64__, and __SHA256_LINUX_ARM64__.
render() {
  mkdir -p "$(dirname "$2")"
  awk -v version="$VERSION" -v sha="$SHA256" -v amd64="$SHA256_LINUX_AMD64" -v arm64="$SHA256_LINUX_ARM64" '
    !started && /^#/ { next }
    {
      started = 1
      gsub(/__VERSION__/, version); gsub(/__SHA256__/, sha)
      gsub(/__SHA256_LINUX_AMD64__/, amd64); gsub(/__SHA256_LINUX_ARM64__/, arm64)
      print
    }
  ' "$1" >"$2"
  if grep -q '__[A-Z0-9_]*__' "$2"; then
    die "unfilled placeholder left in $2"
  fi
}

step "Rendering $CASK_REL and $FORMULA_REL for $VERSION"
render "$TEMPLATE" "$TAP_DIR/$CASK_REL"
render "$FORMULA_TEMPLATE" "$TAP_DIR/$FORMULA_REL"

# --- Checks ------------------------------------------------------------------

step "Checking Ruby syntax"
ruby -c "$TAP_DIR/$CASK_REL" >/dev/null
ruby -c "$TAP_DIR/$FORMULA_REL" >/dev/null

# brew audit only takes cask and formula names, and reads a tap's working
# tree, so the checkout is linked in as a temporary tap for the audit.
step "Auditing with brew audit --cask --strict"
TAPS="$(brew --repository)/Library/Taps"
AUDIT_USER="nu11signal-bump-$$"
AUDIT_TAP="$TAPS/$AUDIT_USER/homebrew-local"
cleanup() { rm -rf "${TAPS:?}/$AUDIT_USER"; }
trap cleanup EXIT
mkdir -p "$TAPS/$AUDIT_USER"
ln -s "$TAP_DIR" "$AUDIT_TAP"
HOMEBREW_NO_AUTO_UPDATE=1 brew audit --cask --strict "$AUDIT_USER/local/nu11signal"
step "Auditing with brew audit --formula --strict and brew style"
HOMEBREW_NO_AUTO_UPDATE=1 brew audit --formula --strict "$AUDIT_USER/local/nu11signal"
HOMEBREW_NO_AUTO_UPDATE=1 brew style "$AUDIT_USER/local/nu11signal"
cleanup
trap - EXIT

push_tap() {
  git -C "$TAP_DIR" -c credential.helper= -c 'credential.helper=!gh auth git-credential' push --quiet
}

step "Changes in $TAP_DIR"
git -C "$TAP_DIR" add -N "$CASK_REL" "$FORMULA_REL"
if git -C "$TAP_DIR" diff --quiet HEAD -- "$CASK_REL" "$FORMULA_REL"; then
  if [[ -z "$UNPUSHED" ]]; then
    echo "    $CASK_REL and $FORMULA_REL are already at $VERSION; nothing to commit"
    exit 0
  fi
  # The committed cask and formula match the render: resume the push, never
  # recommit.
  if [[ "$PUSH" != 1 ]]; then
    echo "An unpushed bump commit for $VERSION exists and matches the render; nothing to commit."
    echo "Not pushed. Rerun with --push (make cask VERSION=$VERSION PUSH=1) to push it."
    exit 0
  fi
  step "Pushing the earlier bump commit(s)"
  push_tap
  echo "Pushed: brew update && brew upgrade --cask nu11signal (macOS) or brew upgrade nu11signal (Linux)"
  exit 0
fi
git -C "$TAP_DIR" --no-pager diff HEAD -- "$CASK_REL" "$FORMULA_REL"

if [[ -n "$UNPUSHED" ]]; then
  msg="the unpushed bump commit for $VERSION in $TAP_DIR does not match the render (diff above);
       inspect it, then drop it ($DROP_HINT) and rerun"
  [[ "$PUSH" == 1 ]] && die "$msg"
  echo "warning: $msg" >&2
fi

if [[ "$PUSH" != 1 ]]; then
  echo "Not pushed. Review, then rerun with --push (make cask VERSION=$VERSION PUSH=1)."
  exit 0
fi

# --- Publish -----------------------------------------------------------------

step "Committing and pushing"
git -C "$TAP_DIR" add -- "$CASK_REL" "$FORMULA_REL"
git -C "$TAP_DIR" commit --quiet -m "chore: bump nu11signal to $VERSION"
push_tap
echo "Pushed: brew update && brew upgrade --cask nu11signal (macOS) or brew upgrade nu11signal (Linux)"
