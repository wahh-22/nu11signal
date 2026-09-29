# Helpers for the release script tests. Sourced by run.sh into a fresh bash
# process per test (errexit, nounset, and pipefail are on).
#
# Every test runs in its own sandbox $T:
#   $T/root   a copy of the scripts, the cask template, LICENSE, and README.md,
#             so the scripts' ROOT (and ROOT/dist) is the sandbox, never the repo;
#   $T/bin    per-test tools (bash, and git: real or stubbed), first on PATH;
#   scripts/test/stubs  brew, ruby, go, swift, lipo, codesign, security,
#             xcrun, spctl, and a logging mv; every call is appended to $STUB_LOG.
# PATH is otherwise limited to the system directories, so no real Homebrew,
# Go, or notary tool can be reached.

T="$(mktemp -d "${TMPDIR:-/tmp}/nu11signal-script-test.XXXXXX")"
T="$(cd "$T" && pwd -P)"
STUB_LOG="$T/stub.log"
STUB_BREW_REPO="$T/brew"
export STUB_LOG STUB_BREW_REPO

REAL_GIT="$(command -v git)"
mkdir -p "$T/bin" "$T/home" "$T/root/scripts" "$T/root/packaging/homebrew" "$STUB_BREW_REPO"
: >"$STUB_LOG"
ln -s "$BASH" "$T/bin/bash"
cp "$REPO/scripts/release.sh" "$REPO/scripts/bump-cask.sh" "$T/root/scripts/"
cp "$REPO/packaging/homebrew/nu11signal.rb.template" "$T/root/packaging/homebrew/"
cp "$REPO/LICENSE" "$REPO/README.md" "$T/root/"

export HOME="$T/home"
export PATH="$T/bin:$TEST_DIR/stubs:/usr/bin:/bin:/usr/sbin:/sbin"
export GIT_CONFIG_GLOBAL="$T/gitconfig" GIT_CONFIG_NOSYSTEM=1
cat >"$GIT_CONFIG_GLOBAL" <<'EOF'
[user]
	name = Script Test
	email = script-test@example.invalid
[init]
	defaultBranch = main
[advice]
	detachedHead = false
EOF
unset NU11SIGNAL_TAP_DIR NU11SIGNAL_TAP_REPO NU11SIGNAL_PROFILE NU11SIGNAL_SIGN_IDENTITY \
  NU11SIGNAL_NOTARY_PROFILE NU11SIGNAL_TEAM_ID DRY_RUN

teardown() {
  if [[ "${KEEP_TEST_TMP:-0}" == 1 ]]; then
    echo "kept sandbox: $T"
  else
    rm -rf "$T"
  fi
}
trap teardown EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# run CMD...: runs CMD with errexit off; sets $status and $output (stdout+stderr).
run() {
  set +e
  output="$("$@" 2>&1)"
  status=$?
  set -e
  printf '%s\n' "$output" >>"$T/run.log"
}

assert_status() {
  [[ "$status" == "$1" ]] || fail "expected exit $1, got $status; output:
$output"
}

assert_failed() {
  [[ "$status" != 0 ]] || fail "expected a failure, got exit 0; output:
$output"
}

assert_output_contains() {
  [[ "$output" == *"$1"* ]] || fail "output does not contain '$1'; output:
$output"
}

assert_output_lacks() {
  [[ "$output" != *"$1"* ]] || fail "output unexpectedly contains '$1'; output:
$output"
}

assert_file_contains() {
  grep -qF -- "$2" "$1" || fail "$1 does not contain '$2'"
}

# tree_hash DIR: a digest of every path, mode, and file content under DIR.
tree_hash() {
  (
    cd "$1" &&
      find . -print0 | LC_ALL=C sort -z | while IFS= read -r -d '' p; do
        if [[ -f "$p" && ! -L "$p" ]]; then
          printf '%s %s %s\n' "$p" "$(stat -f %Lp "$p")" "$(shasum -a 256 <"$p" | cut -d' ' -f1)"
        else
          printf '%s %s\n' "$p" "$(stat -f %Lp "$p")"
        fi
      done
  ) | shasum -a 256 | cut -d' ' -f1
}

stub_calls() {
  grep -c "^$1 " "$STUB_LOG" || true
}

# --- bump-cask.sh -------------------------------------------------------------

SHA_A="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
SHA_B="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

# render_cask VERSION SHA: what bump-cask.sh is expected to write.
render_cask() {
  awk -v version="$1" -v sha="$2" '
    !started && /^#/ { next }
    { started = 1; gsub(/__VERSION__/, version); gsub(/__SHA256__/, sha); print }
  ' "$T/root/packaging/homebrew/nu11signal.rb.template"
}

# write_checksum VERSION SHA: the .sha256 file release.sh writes for VERSION.
write_checksum() {
  local archive="nu11signal-$1-macos-universal.tar.gz"
  mkdir -p "$T/root/dist/v$1"
  printf '%s  %s\n' "$2" "$archive" >"$T/root/dist/v$1/$archive.sha256"
}

# setup_tap: a bare "origin" whose Casks/nu11signal.rb is at 0.2.0 ($SHA_A),
# with the tap checkout at $TAP (not cloned yet) and real git on PATH.
setup_tap() {
  ln -s "$REAL_GIT" "$T/bin/git"
  ORIGIN="$T/origin.git"
  TAP="$T/tap"
  export NU11SIGNAL_TAP_REPO="$ORIGIN" NU11SIGNAL_TAP_DIR="$TAP"
  git init --quiet --bare "$ORIGIN"
  git clone --quiet "$ORIGIN" "$T/seed" 2>/dev/null
  mkdir -p "$T/seed/Casks"
  render_cask 0.2.0 "$SHA_A" >"$T/seed/Casks/nu11signal.rb"
  printf '# tap\n' >"$T/seed/README.md"
  git -C "$T/seed" add -A
  git -C "$T/seed" commit --quiet -m "Add nu11signal 0.2.0"
  git -C "$T/seed" push --quiet origin HEAD:main 2>/dev/null
}

clone_tap() {
  git clone --quiet "$ORIGIN" "$TAP"
}

origin_commits() {
  git --git-dir="$ORIGIN" rev-list --count main
}

bump() {
  run "$T/root/scripts/bump-cask.sh" "$@"
}

# --- release.sh ---------------------------------------------------------------

# setup_release: a stubbed toolchain, a fake helper/build.sh, and a profile.
setup_release() {
  ln -s "$TEST_DIR/stubs/git-release" "$T/bin/git"
  mkdir -p "$T/root/helper" "$T/root/signing"
  : >"$T/root/signing/Nu11Signal_DeveloperID.provisionprofile"
  cat >"$T/root/helper/build.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "helper/build.sh $NU11SIGNAL_BUILD_DIR" >>"$STUB_LOG"
app="$NU11SIGNAL_BUILD_DIR/Nu11SignalHelper.app/Contents/MacOS"
mkdir -p "$app"
printf '#!/bin/sh\necho helper\n' >"$app/nu11signal-helper"
chmod +x "$app/nu11signal-helper"
EOF
  chmod +x "$T/root/helper/build.sh"
  DIST="$T/root/dist"
}

release() {
  run "$T/root/scripts/release.sh" "$@"
}

# staging_left DIR: prints any staging directory left in DIR.
staging_left() {
  find "$1" -maxdepth 1 -name '.staging-*' 2>/dev/null
}
