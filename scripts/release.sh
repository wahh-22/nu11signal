#!/usr/bin/env bash
# Builds a Nu11Signal release: signed, notarized macOS (arm64 + x86_64) and
# Linux (amd64, arm64) archives.
#
# Usage: scripts/release.sh [--dry-run] [--force] [--linux-only] VERSION
#        (VERSION: x.y.z or x.y.z-pre)
#
# Produces dist/vVERSION/ with nu11signal-VERSION/{bin/nu11signal,
# libexec/Nu11SignalHelper.app, LICENSE, README.md},
# nu11signal-VERSION-macos-universal.tar.gz (that tree), and its .sha256; and
# nu11signal-VERSION-linux-{amd64,arm64}.tar.gz with their .sha256 files, each
# holding nu11signal-VERSION/{bin/nu11signal, LICENSE, README.md}. The Linux
# binaries are cross-compiled with CGO_ENABLED=0 and the same version ldflags;
# they are checked to be ELF executables for their architecture and stamped
# with VERSION, but never run (cross-arch).
#
# --linux-only (make release-linux) builds only the Linux archives and needs
# no Apple credentials, signing, or Xcode tools. With --force it replaces the
# Linux archives of an existing dist/vVERSION and keeps everything else in it
# (the macOS artifacts are copied into the new directory).
#
# Everything is built in dist/.staging-vVERSION.XXXXXX (same filesystem) and
# promoted only after notarization and the final checks pass, with a single
# rename of the staging directory to dist/vVERSION. A failed or interrupted
# run removes the staging directory and leaves dist/ untouched.
#
# An existing dist/vVERSION is never overwritten unless --force is given. Then
# it is first renamed to dist/vVERSION.replaced-<timestamp> (kept) and the
# staging directory renamed into place: two adjacent renames, because macOS
# has no atomic directory swap from the shell. If the second rename fails, or
# the run is interrupted between them, the backup is renamed back on exit;
# only a hard kill (SIGKILL, power loss) in that instant can leave
# dist/vVERSION missing with the previous release in the backup. A later run
# then refuses to build until that backup is restored or deleted, and backups
# kept next to an existing dist/vVERSION are listed as a warning.
#
# Requires (see docs/releasing.md; with --linux-only, only the last item):
#   - a "Developer ID Application" certificate for the team in the keychain;
#   - the Developer ID provisioning profile at $NU11SIGNAL_PROFILE
#     (default signing/Nu11Signal_DeveloperID.provisionprofile);
#   - a notarytool keychain profile $NU11SIGNAL_NOTARY_PROFILE (default soulking-notary);
#   - no uncommitted changes to tracked files.
#
# --dry-run (or DRY_RUN=1) reports missing requirements without aborting,
# builds and assembles everything with ad-hoc signatures, and skips
# notarization, stapling, and archiving the macOS layout (the Linux archives
# are built and checked as in a release). It writes to
# build/release-dry-run/vVERSION/ (never dist/), replacing an earlier dry run;
# it is not distributable.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DRY_RUN="${DRY_RUN:-0}"
FORCE=0
LINUX_ONLY=0
VERSION=""
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --force) FORCE=1 ;;
    --linux-only) LINUX_ONLY=1 ;;
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

TEAM_ID="${NU11SIGNAL_TEAM_ID:-W6GZP998GQ}"
BUNDLE_ID="${NU11SIGNAL_BUNDLE_ID:-dev.wahh.soulking.player}"
IDENTITY="${NU11SIGNAL_SIGN_IDENTITY:-Developer ID Application}"
PROFILE="${NU11SIGNAL_PROFILE:-signing/Nu11Signal_DeveloperID.provisionprofile}"
NOTARY_PROFILE="${NU11SIGNAL_NOTARY_PROFILE:-soulking-notary}"
CLI_IDENTIFIER="${NU11SIGNAL_CLI_IDENTIFIER:-dev.wahh.nu11signal.cli}"

step() { echo "==> $*"; }
die() {
  echo "error: $*" >&2
  exit 1
}

# --- Preflight -------------------------------------------------------------

[[ -n "$VERSION" ]] || die "usage: scripts/release.sh [--dry-run] [--force] [--linux-only] VERSION"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] ||
  die "VERSION must be semver (x.y.z or x.y.z-pre), got: $VERSION"

tools=(go shasum tar git od)
[[ "$LINUX_ONLY" == 1 ]] || tools+=(swift xcrun codesign lipo ditto plutil security)
for tool in "${tools[@]}"; do
  command -v "$tool" >/dev/null 2>&1 || die "required tool not found: $tool"
done

NAME="nu11signal-$VERSION"
ARCHIVE_NAME="$NAME-macos-universal.tar.gz"
LINUX_ARCHS=(amd64 arm64)
if [[ "$LINUX_ONLY" == 1 ]]; then
  MAKE_TARGET=release-linux
else
  MAKE_TARGET=release
fi
if [[ "$DRY_RUN" == 1 ]]; then
  OUT_DIR="$ROOT/build/release-dry-run"
else
  OUT_DIR="$ROOT/dist"
fi
DEST="$OUT_DIR/v$VERSION"

if [[ "$DRY_RUN" != 1 && "$FORCE" != 1 ]] && [[ -e "$DEST" || -L "$DEST" ]]; then
  die "$DEST already exists; pass --force (make $MAKE_TARGET VERSION=$VERSION FORCE=1) to replace it"
fi

# Backups of DEST left by earlier --force runs. A missing DEST next to one
# means a run was killed between its two renames: refuse to build over that.
if [[ "$DRY_RUN" != 1 ]]; then
  shopt -s nullglob
  backups=("$DEST".replaced-*)
  shopt -u nullglob
  if ((${#backups[@]} > 0)); then
    if [[ ! -e "$DEST" && ! -L "$DEST" ]]; then
      die "$DEST is missing, but backup(s) of it exist (an earlier --force run was interrupted between its renames):
$(printf '       %s\n' "${backups[@]}")
       Restore the latest one (mv ${backups[${#backups[@]} - 1]} $DEST) or delete them, then rerun."
    fi
    echo "warning: earlier backup(s) of $DEST from previous --force runs are kept; delete them once they are not needed:" >&2
    printf '    %s\n' "${backups[@]}" >&2
  fi
fi

step "Preflight for nu11signal $VERSION$([[ "$LINUX_ONLY" == 1 ]] && echo ", Linux only")$([[ "$DRY_RUN" == 1 ]] && echo " (dry run)")"
missing=()

# check_apple: the signing and notarization setup (not needed for --linux-only).
check_apple() {
  if security find-identity -v -p codesigning | grep -F "\"$IDENTITY" | grep -qF "($TEAM_ID)"; then
    echo "    ok: signing identity \"$IDENTITY\" for team $TEAM_ID"
  else
    missing+=("signing identity \"$IDENTITY\" for team $TEAM_ID is not in the keychain.
        Create a Developer ID Application certificate (docs/releasing.md, step 1);
        check with: security find-identity -v -p codesigning")
  fi

  if [[ -f "$PROFILE" ]]; then
    echo "    ok: provisioning profile $PROFILE"
  else
    missing+=("Developer ID provisioning profile not found: $PROFILE
        Create it for $TEAM_ID.$BUNDLE_ID and save it there (docs/releasing.md, step 2)")
  fi

  if xcrun notarytool history --keychain-profile "$NOTARY_PROFILE" >/dev/null 2>&1; then
    echo "    ok: notarytool keychain profile $NOTARY_PROFILE"
  else
    missing+=("notarytool keychain profile \"$NOTARY_PROFILE\" is missing or its credentials are rejected.
        Run: xcrun notarytool store-credentials $NOTARY_PROFILE --apple-id <id> --team-id $TEAM_ID
        (docs/releasing.md, step 3)")
  fi
}
[[ "$LINUX_ONLY" == 1 ]] || check_apple

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
  echo "    dry run: continuing$([[ "$LINUX_ONLY" == 1 ]] || echo " with ad-hoc signatures")" >&2
fi

if [[ "$DRY_RUN" == 1 ]]; then
  IDENTITY="-" # ad hoc: never pass a dry-run build off as a release
fi

# --- Build -----------------------------------------------------------------

mkdir -p "$OUT_DIR"
# The staging directory lives inside OUT_DIR so that promoting it is a
# same-filesystem rename. Nothing in OUT_DIR changes before promotion.
STAGE_ROOT="$(mktemp -d "$OUT_DIR/.staging-v$VERSION.XXXXXX")"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/nu11signal-release.XXXXXX")"
BACKUP=""
cleanup() {
  # Interrupted between the two --force renames: put the previous release back.
  if [[ -n "$BACKUP" && -e "$BACKUP" && ! -e "$DEST" ]]; then
    mv "$BACKUP" "$DEST" && echo "restored the previous $DEST" >&2
  fi
  rm -rf "$WORK_DIR" ${STAGE_ROOT:+"$STAGE_ROOT"}
}
trap cleanup EXIT
STAGE="$STAGE_ROOT/$NAME"
ARCHIVE="$STAGE_ROOT/$ARCHIVE_NAME"
[[ "$LINUX_ONLY" == 1 ]] || mkdir -p "$STAGE/bin" "$STAGE/libexec"

# promote: renames the staging directory to DEST in one step. An existing
# DEST is removed first in a dry run, and renamed to a timestamped backup with
# --force: everything is prepared before that first rename, the second rename
# follows it directly, and BACKUP is set before the first one so that cleanup
# restores the previous release if the run stops between them.
promote() {
  chmod 755 "$STAGE_ROOT" # mktemp -d creates it 0700
  if [[ -e "$DEST" || -L "$DEST" ]]; then
    if [[ "$DRY_RUN" == 1 ]]; then
      rm -rf "$DEST"
    elif [[ "$FORCE" == 1 ]]; then
      local backup
      backup="$DEST.replaced-$(date +%Y%m%dT%H%M%S)"
      [[ ! -e "$backup" && ! -L "$backup" ]] || die "$backup already exists; retry in a second"
      BACKUP="$backup"
      mv "$DEST" "$BACKUP"
      mv "$STAGE_ROOT" "$DEST" || die "could not rename $STAGE_ROOT to $DEST"
      STAGE_ROOT=""
      echo "    previous release kept at $BACKUP"
      BACKUP=""
      return
    else
      die "$DEST appeared during the build; pass --force to replace it"
    fi
  fi
  # mv would move the staging directory *into* an existing DEST; it was
  # checked (or removed) just above.
  mv "$STAGE_ROOT" "$DEST" || die "could not rename $STAGE_ROOT to $DEST"
  STAGE_ROOT=""
}

# elf_machine FILE: the machine of a 64-bit little-endian ELF executable
# (x86_64, aarch64, machine-<hex>), or not-elf.
elf_machine() {
  local hdr
  hdr="$(od -An -tx1 -N20 "$1" | tr -d ' \n')"
  [[ "${hdr:0:8}" == 7f454c46 && "${hdr:8:4}" == 0201 && "${hdr:32:4}" == 0200 ]] || {
    echo not-elf
    return
  }
  case "${hdr:36:4}" in
    3e00) echo x86_64 ;;
    b700) echo aarch64 ;;
    *) echo "machine-${hdr:36:4}" ;;
  esac
}

# check_linux_binary FILE GOARCH: FILE is an executable ELF for GOARCH and
# carries VERSION (stamped with -X main.version). It cannot be run here.
check_linux_binary() {
  local file="$1" want got
  case "$2" in
    amd64) want=x86_64 ;;
    arm64) want=aarch64 ;;
    *) die "unsupported Linux architecture: $2" ;;
  esac
  [[ -f "$file" && -x "$file" ]] || die "$file is missing or not executable"
  got="$(elf_machine "$file")"
  [[ "$got" == "$want" ]] || die "$file is not an $want ELF executable (got: $got)"
  LC_ALL=C grep -qaF -- "$VERSION" "$file" || die "$file is not stamped with version $VERSION"
}

# build_linux_archives: cross-compiles nu11signal for each Linux architecture
# and writes nu11signal-VERSION-linux-ARCH.tar.gz and its .sha256 to the
# staging directory.
build_linux_archives() {
  local arch tree archive
  for arch in "${LINUX_ARCHS[@]}"; do
    step "Building nu11signal $VERSION for linux/$arch"
    tree="$WORK_DIR/linux-$arch"
    archive="$NAME-linux-$arch.tar.gz"
    mkdir -p "$tree/$NAME/bin"
    GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=$VERSION" \
      -o "$tree/$NAME/bin/nu11signal" ./cmd/nu11signal
    check_linux_binary "$tree/$NAME/bin/nu11signal" "$arch"
    cp LICENSE README.md "$tree/$NAME/"
    COPYFILE_DISABLE=1 tar -C "$tree" -czf "$STAGE_ROOT/$archive" "$NAME"
    (cd "$STAGE_ROOT" && shasum -a 256 "$archive" >"$archive.sha256")
  done
}

# verify_linux_archives: each staged Linux archive matches its checksum, holds
# exactly the expected entries, and unpacks to an ELF for its architecture.
verify_linux_archives() {
  local arch archive check entries
  local want="$NAME $NAME/LICENSE $NAME/README.md $NAME/bin $NAME/bin/nu11signal "
  step "Verifying the Linux archives"
  for arch in "${LINUX_ARCHS[@]}"; do
    archive="$NAME-linux-$arch.tar.gz"
    (cd "$STAGE_ROOT" && shasum -a 256 -c "$archive.sha256" >/dev/null) ||
      die "$archive does not match $archive.sha256"
    entries="$(tar -tzf "$STAGE_ROOT/$archive" | sed 's:/$::' | LC_ALL=C sort | tr '\n' ' ')"
    [[ "$entries" == "$want" ]] || die "unexpected entries in $archive: $entries"
    check="$WORK_DIR/check-linux-$arch"
    mkdir -p "$check"
    tar -C "$check" -xzf "$STAGE_ROOT/$archive"
    check_linux_binary "$check/$NAME/bin/nu11signal" "$arch"
    echo "    ok: $archive ($(elf_machine "$check/$NAME/bin/nu11signal") ELF, version $VERSION; --version not run (cross-arch))"
  done
}

# keep_previous_entries: with --linux-only, copies everything in an existing
# DEST except its Linux archives into the staging directory, so replacing DEST
# keeps the macOS artifacts (the previous DEST itself becomes the backup).
keep_previous_entries() {
  local entry base arch skip
  [[ -d "$DEST" ]] || return 0
  shopt -s nullglob dotglob
  for entry in "$DEST"/*; do
    base="${entry##*/}"
    skip=0
    for arch in "${LINUX_ARCHS[@]}"; do
      [[ "$base" == "$NAME-linux-$arch.tar.gz" || "$base" == "$NAME-linux-$arch.tar.gz.sha256" ]] && skip=1
    done
    if [[ "$skip" == 0 ]]; then
      cp -pR "$entry" "$STAGE_ROOT/" || die "could not copy $entry into the staging directory"
      echo "    kept from the previous $DEST: $base"
    fi
  done
  shopt -u nullglob dotglob
}

if [[ "$LINUX_ONLY" == 1 ]]; then
  build_linux_archives
  verify_linux_archives
  keep_previous_entries
  step "Promoting the Linux archives to $DEST"
  promote
  if [[ "$DRY_RUN" == 1 ]]; then
    echo "Assembled (dry run, not distributable): $DEST"
    exit 0
  fi
  step "Linux release ready"
  for arch in "${LINUX_ARCHS[@]}"; do cat "$DEST/$NAME-linux-$arch.tar.gz.sha256"; done
  echo "Next: upload the Linux archives and their .sha256 files to the v$VERSION GitHub release"
  exit 0
fi

step "Building universal nu11signal $VERSION"
for arch in arm64 amd64; do
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o "$WORK_DIR/nu11signal-$arch" ./cmd/nu11signal
done
lipo -create -output "$STAGE/bin/nu11signal" "$WORK_DIR/nu11signal-arm64" "$WORK_DIR/nu11signal-amd64"

step "Signing nu11signal with \"$IDENTITY\""
if [[ "$IDENTITY" == "-" ]]; then
  codesign --force --options runtime --timestamp=none --identifier "$CLI_IDENTIFIER" \
    --sign - "$STAGE/bin/nu11signal"
else
  codesign --force --options runtime --timestamp --identifier "$CLI_IDENTIFIER" \
    --sign "$IDENTITY" "$STAGE/bin/nu11signal"
fi
codesign --verify --strict --verbose=2 "$STAGE/bin/nu11signal"

step "Building universal Nu11SignalHelper.app"
NU11SIGNAL_SIGN_MODE=release \
  NU11SIGNAL_SIGN_IDENTITY="$IDENTITY" \
  NU11SIGNAL_PROFILE="$PROFILE" \
  NU11SIGNAL_BUILD_DIR="$STAGE/libexec" \
  NU11SIGNAL_VERSION="$VERSION" \
  ./helper/build.sh

cp LICENSE README.md "$STAGE/"

build_linux_archives

if [[ "$DRY_RUN" == 1 ]]; then
  step "Dry run: skipping notarization, stapling, and archiving the macOS layout"
  lipo -archs "$STAGE/bin/nu11signal"
  lipo -archs "$STAGE/libexec/Nu11SignalHelper.app/Contents/MacOS/nu11signal-helper"
  verify_linux_archives
  promote
  echo "Assembled (ad hoc, not distributable): $DEST/$NAME"
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
xcrun stapler staple "$STAGE/libexec/Nu11SignalHelper.app"
xcrun stapler validate "$STAGE/libexec/Nu11SignalHelper.app"

# --- Archive ---------------------------------------------------------------

step "Archiving $ARCHIVE_NAME"
# COPYFILE_DISABLE keeps AppleDouble (._*) files out of the archive.
COPYFILE_DISABLE=1 tar -C "$STAGE_ROOT" -czf "$ARCHIVE" "$NAME"
(cd "$STAGE_ROOT" && shasum -a 256 "$ARCHIVE_NAME" >"$ARCHIVE_NAME.sha256")

# --- Final checks ------------------------------------------------------------

step "Verifying the archived contents"
CHECK_DIR="$WORK_DIR/check"
mkdir -p "$CHECK_DIR"
tar -C "$CHECK_DIR" -xzf "$ARCHIVE"
spctl --assess --type execute -vv "$CHECK_DIR/$NAME/libexec/Nu11SignalHelper.app"
codesign --verify --strict --deep --verbose=2 "$CHECK_DIR/$NAME/libexec/Nu11SignalHelper.app"
codesign --verify --strict --verbose=2 "$CHECK_DIR/$NAME/bin/nu11signal"
[[ "$("$CHECK_DIR/$NAME/bin/nu11signal" --version)" == "$VERSION" ]] ||
  die "archived nu11signal --version does not print $VERSION"
verify_linux_archives

step "Promoting the release to $DEST"
promote

step "Release ready"
cat "$DEST/$ARCHIVE_NAME.sha256"
for arch in "${LINUX_ARCHS[@]}"; do cat "$DEST/$NAME-linux-$arch.tar.gz.sha256"; done
echo "Next: upload the archives and their .sha256 files in $DEST to the v$VERSION GitHub release,"
echo "      then run: make cask VERSION=$VERSION (renders the cask and the formula)"
