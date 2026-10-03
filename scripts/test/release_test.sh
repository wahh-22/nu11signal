# Tests for scripts/release.sh. Run through scripts/test/run.sh.

# assert_release_layout DIR VERSION: DIR is a complete promoted release
# (macOS and Linux).
assert_release_layout() {
  local dir="$1" v="$2"
  local archive="nu11signal-$v-macos-universal.tar.gz"
  [[ -x "$dir/nu11signal-$v/bin/nu11signal" ]] || fail "missing $dir/nu11signal-$v/bin/nu11signal"
  [[ -x "$dir/nu11signal-$v/libexec/Nu11SignalHelper.app/Contents/MacOS/nu11signal-helper" ]] ||
    fail "missing the helper in $dir"
  [[ -f "$dir/nu11signal-$v/LICENSE" && -f "$dir/nu11signal-$v/README.md" ]] || fail "missing LICENSE/README.md"
  [[ -f "$dir/$archive" ]] || fail "missing $dir/$archive"
  (cd "$dir" && shasum -a 256 -c "$archive.sha256" >/dev/null) || fail "checksum does not verify"
  local linux="nu11signal-$v-linux"
  [[ "$(ls -A "$dir" | LC_ALL=C sort | tr '\n' ' ')" == \
    "nu11signal-$v $linux-amd64.tar.gz $linux-amd64.tar.gz.sha256 $linux-arm64.tar.gz $linux-arm64.tar.gz.sha256 $archive $archive.sha256 " ]] ||
    fail "unexpected entries in $dir: $(ls -A "$dir")"
  assert_linux_archive "$dir" "$v" amd64
  assert_linux_archive "$dir" "$v" arm64
  [[ "$(stat -f %Lp "$dir")" == 755 ]] || fail "$dir mode is $(stat -f %Lp "$dir"), want 755"
}

test_release_rejects_non_semver_version() {
  setup_release
  release 0.2
  assert_failed
  assert_output_contains "VERSION must be semver"
  [[ ! -e "$DIST" ]] || fail "dist/ was created"
}

test_release_promotes_with_a_single_rename() {
  setup_release
  release 0.2.1
  assert_status 0
  assert_release_layout "$DIST/v0.2.1" 0.2.1
  [[ "$(ls -A "$DIST")" == v0.2.1 ]] || fail "unexpected entries in dist/: $(ls -A "$DIST")"
  [[ "$(stub_calls mv)" == 1 ]] || fail "expected exactly one mv, got: $(grep '^mv ' "$STUB_LOG")"
  grep -qE "^mv .*/dist/\.staging-v0\.2\.1\.[^ /]+ $DIST/v0\.2\.1$" "$STUB_LOG" ||
    fail "the one mv is not the staging dir renamed to dist/v0.2.1: $(grep '^mv ' "$STUB_LOG")"
  tar -tzf "$DIST/v0.2.1/nu11signal-0.2.1-macos-universal.tar.gz" | grep -qx 'nu11signal-0.2.1/bin/nu11signal' ||
    fail "archive does not contain nu11signal-0.2.1/bin/nu11signal"
}

test_release_keeps_other_versions_untouched() {
  setup_release
  mkdir -p "$DIST/v0.2.0/nu11signal-0.2.0"
  printf 'published\n' >"$DIST/v0.2.0/nu11signal-0.2.0/marker"
  printf 'legacy\n' >"$DIST/nu11signal-0.2.0-macos-universal.tar.gz"
  release 0.2.1
  assert_status 0
  assert_file_contains "$DIST/v0.2.0/nu11signal-0.2.0/marker" published
  assert_file_contains "$DIST/nu11signal-0.2.0-macos-universal.tar.gz" legacy
  assert_release_layout "$DIST/v0.2.1" 0.2.1
}

test_release_refuses_to_overwrite_without_force() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  local before
  before="$(tree_hash "$DIST")"
  release 0.2.1
  assert_failed
  assert_output_contains "$DIST/v0.2.1 already exists"
  assert_output_contains "--force"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ "$(stub_calls go)" == 0 ]] || fail "built before refusing"
}

test_release_force_replaces_and_keeps_a_backup() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  release --force 0.2.1
  assert_status 0
  assert_release_layout "$DIST/v0.2.1" 0.2.1
  local backups
  backups="$(find "$DIST" -maxdepth 1 -name 'v0.2.1.replaced-*')"
  [[ -n "$backups" && "$(printf '%s\n' "$backups" | wc -l | tr -d ' ')" == 1 ]] ||
    fail "expected one backup, got: $backups"
  assert_file_contains "$backups/marker" published
  assert_output_contains "$backups"
  [[ "$(stub_calls mv)" == 2 ]] || fail "expected two renames, got: $(grep '^mv ' "$STUB_LOG")"
}

test_release_failure_after_staging_leaves_dist_untouched() {
  setup_release
  mkdir -p "$DIST/v0.2.0"
  printf 'published\n' >"$DIST/v0.2.0/marker"
  local before
  before="$(tree_hash "$DIST")"
  STUB_SPCTL_FAIL=1 release 0.2.1
  assert_failed
  [[ "$(stub_calls spctl)" == 1 ]] || fail "did not fail at the final spctl check"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ -z "$(staging_left "$DIST")" ]] || fail "staging left behind: $(staging_left "$DIST")"
  [[ "$(stub_calls mv)" == 0 ]] || fail "something was promoted"
}

test_release_rejected_notarization_leaves_dist_untouched() {
  setup_release
  mkdir -p "$DIST"
  local before
  before="$(tree_hash "$DIST")"
  STUB_NOTARY_STATUS=Invalid release 0.2.1
  assert_failed
  assert_output_contains "notarization status: Invalid"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ -z "$(staging_left "$DIST")" ]] || fail "staging left behind"
}

test_release_dry_run_writes_only_the_dry_run_dir() {
  setup_release
  release --dry-run 0.2.1
  assert_status 0
  local out="$T/root/build/release-dry-run/v0.2.1/nu11signal-0.2.1"
  [[ -x "$out/bin/nu11signal" ]] || fail "missing $out/bin/nu11signal"
  assert_linux_archive "$T/root/build/release-dry-run/v0.2.1" 0.2.1 amd64
  assert_linux_archive "$T/root/build/release-dry-run/v0.2.1" 0.2.1 arm64
  [[ ! -e "$DIST" ]] || fail "a dry run created dist/"
  [[ "$(stub_calls 'xcrun notarytool submit')" == 0 ]] || fail "a dry run notarized"
  release --dry-run 0.2.1
  assert_status 0
  [[ -z "$(staging_left "$T/root/build/release-dry-run")" ]] || fail "staging left behind"
}

test_release_force_restores_the_previous_release_when_promotion_fails() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  local before
  before="$(tree_hash "$DIST")"
  STUB_MV_FAIL_STAGING=1 release --force 0.2.1
  assert_failed
  assert_output_contains "could not rename"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ was not restored: $(ls -A "$DIST")"
}

test_release_force_restores_the_previous_release_when_interrupted_between_renames() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  local before
  before="$(tree_hash "$DIST")"
  STUB_MV_TERM_AFTER_BACKUP=1 release --force 0.2.1
  assert_failed
  assert_output_contains "restored the previous $DIST/v0.2.1"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ was not restored: $(ls -A "$DIST")"
}

test_release_force_reports_a_leftover_backup() {
  setup_release
  mkdir -p "$DIST/v0.2.1" "$DIST/v0.2.1.replaced-20260101T000000"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  release --force 0.2.1
  assert_status 0
  assert_output_contains "earlier backup"
  assert_output_contains "$DIST/v0.2.1.replaced-20260101T000000"
  [[ -d "$DIST/v0.2.1.replaced-20260101T000000" ]] || fail "the earlier backup was removed"
  assert_release_layout "$DIST/v0.2.1" 0.2.1
}

test_release_refuses_when_only_a_backup_is_left() {
  setup_release
  mkdir -p "$DIST/v0.2.1.replaced-20260101T000000"
  printf 'published\n' >"$DIST/v0.2.1.replaced-20260101T000000/marker"
  local before
  before="$(tree_hash "$DIST")"
  release --force 0.2.1
  assert_failed
  assert_output_contains "$DIST/v0.2.1 is missing"
  assert_output_contains "mv $DIST/v0.2.1.replaced-20260101T000000 $DIST/v0.2.1"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ "$(stub_calls go)" == 0 ]] || fail "built before refusing"
}

test_release_rejects_an_unknown_option() {
  setup_release
  release --bogus 0.2.1
  assert_status 2
  assert_output_contains "unknown option: --bogus"
}

test_release_refuses_a_linux_binary_for_the_wrong_arch() {
  setup_release
  mkdir -p "$DIST"
  local before
  before="$(tree_hash "$DIST")"
  STUB_GO_WRONG_ARCH=1 release 0.2.1
  assert_failed
  assert_output_contains "is not an x86_64 ELF executable (got: aarch64)"
  [[ "$(stub_calls 'xcrun notarytool submit')" == 0 ]] || fail "notarized before the Linux check"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ -z "$(staging_left "$DIST")" ]] || fail "staging left behind"
}

test_release_refuses_a_linux_elf_that_is_not_an_executable() {
  setup_release
  mkdir -p "$DIST"
  STUB_GO_ELF_TYPE=1 release --linux-only 0.2.1
  assert_failed
  assert_output_contains "is not an x86_64 ELF executable (got: e_type-1)"
  [[ -z "$(ls -A "$DIST")" ]] || fail "dist/ changed: $(ls -A "$DIST")"
}

test_release_accepts_a_position_independent_linux_executable() {
  setup_release
  STUB_GO_ELF_TYPE=3 release --linux-only 0.2.1
  assert_status 0
  assert_linux_release "$DIST/v0.2.1" 0.2.1
}

test_release_refuses_a_linux_binary_without_the_version_stamp() {
  setup_release
  mkdir -p "$DIST"
  # The bare version alone is not enough: only the stamp is unambiguous.
  STUB_GO_STAMP= release --linux-only 0.2.1
  assert_failed
  assert_output_contains "is not stamped with version 0.2.1"
  [[ -z "$(ls -A "$DIST")" ]] || fail "dist/ changed: $(ls -A "$DIST")"
}

test_release_refuses_a_linux_binary_stamped_with_another_version() {
  setup_release
  mkdir -p "$DIST"
  local stamp
  for stamp in "nu11signal-version:v0.2.1;" "nu11signal-version:0.2.11;" \
    "nu11signal-version:10.2.1;" "nu11signal-version:0.2;" "nu11signal-version:0.2.1-rc1;" \
    "nu11signal-version:0.3.0;" "nu11signal-version:0.2.1" "0.2.1;"; do
    STUB_GO_STAMP="$stamp" release --linux-only 0.2.1
    assert_failed
    assert_output_contains "is not stamped with version 0.2.1"
  done
  # A shorter release version is not found inside a longer stamp.
  STUB_GO_STAMP="nu11signal-version:0.2.1;" release --linux-only 0.2.10
  assert_failed
  assert_output_contains "is not stamped with version 0.2.10"
  [[ -z "$(ls -A "$DIST")" ]] || fail "dist/ changed: $(ls -A "$DIST")"
}

test_release_accepts_a_stamp_next_to_other_strings() {
  setup_release
  # The linker packs strings without separators; the stamp's own prefix and
  # terminator delimit it, whatever touches it.
  STUB_GO_STAMP_PREFIX=runtime1 STUB_GO_STAMP_SUFFIX=0GOROOT release --linux-only 0.2.1
  assert_status 0
  assert_linux_release "$DIST/v0.2.1" 0.2.1
}

test_release_matches_the_stamp_literally() {
  setup_release
  mkdir -p "$DIST"
  # As a regular expression, 0.5.0 would match 0x5y0.
  STUB_GO_STAMP="nu11signal-version:0x5y0;" release --linux-only 0.5.0
  assert_failed
  assert_output_contains "is not stamped with version 0.5.0"
  [[ -z "$(ls -A "$DIST")" ]] || fail "dist/ changed: $(ls -A "$DIST")"
}

test_release_refuses_a_truncated_linux_elf() {
  setup_release
  mkdir -p "$DIST"
  STUB_GO_TRUNCATE=10 release --linux-only 0.2.1
  assert_failed
  assert_output_contains "is not an x86_64 ELF executable (got: not-elf)"
  STUB_GO_TRUNCATE=0 release --linux-only 0.2.1
  assert_failed
  assert_output_contains "is not an x86_64 ELF executable (got: not-elf)"
  [[ -z "$(ls -A "$DIST")" ]] || fail "dist/ changed: $(ls -A "$DIST")"
  [[ -z "$(staging_left "$DIST")" ]] || fail "staging left behind"
}

# --- --linux-only (make release-linux) ----------------------------------------

# assert_linux_release DIR VERSION: DIR holds exactly the Linux artifacts.
assert_linux_release() {
  local dir="$1" v="$2" linux="nu11signal-$2-linux"
  [[ "$(ls -A "$dir" | LC_ALL=C sort | tr '\n' ' ')" == \
    "$linux-amd64.tar.gz $linux-amd64.tar.gz.sha256 $linux-arm64.tar.gz $linux-arm64.tar.gz.sha256 " ]] ||
    fail "unexpected entries in $dir: $(ls -A "$dir")"
  assert_linux_archive "$dir" "$v" amd64
  assert_linux_archive "$dir" "$v" arm64
  [[ "$(stat -f %Lp "$dir")" == 755 ]] || fail "$dir mode is $(stat -f %Lp "$dir"), want 755"
}

test_release_linux_only_needs_no_apple_credentials() {
  setup_release
  rm "$T/root/signing/Nu11Signal_DeveloperID.provisionprofile"
  release --linux-only 0.2.1
  assert_status 0
  assert_linux_release "$DIST/v0.2.1" 0.2.1
  local tool
  for tool in security xcrun codesign lipo swift spctl helper/build.sh; do
    [[ "$(stub_calls "$tool")" == 0 ]] || fail "--linux-only called $tool: $(grep "^$tool " "$STUB_LOG")"
  done
  [[ "$(stub_calls 'go GOOS=darwin')" == 0 ]] || fail "--linux-only built for darwin"
  assert_output_lacks "provisioning profile"
  assert_output_contains "--version not run (cross-arch)"
}

test_release_linux_only_promotes_with_a_single_rename() {
  setup_release
  release --linux-only 0.2.1
  assert_status 0
  [[ "$(ls -A "$DIST")" == v0.2.1 ]] || fail "unexpected entries in dist/: $(ls -A "$DIST")"
  [[ "$(stub_calls mv)" == 1 ]] || fail "expected exactly one mv, got: $(grep '^mv ' "$STUB_LOG")"
  grep -qE "^mv .*/dist/\.staging-v0\.2\.1\.[^ /]+ $DIST/v0\.2\.1$" "$STUB_LOG" ||
    fail "the one mv is not the staging dir renamed to dist/v0.2.1: $(grep '^mv ' "$STUB_LOG")"
}

test_release_linux_only_refuses_a_dirty_tree() {
  setup_release
  mkdir -p "$DIST"
  STUB_GIT_DIRTY=1 release --linux-only 0.2.1
  assert_failed
  assert_output_contains "uncommitted changes"
  [[ -z "$(ls -A "$DIST")" ]] || fail "dist/ changed: $(ls -A "$DIST")"
  [[ "$(stub_calls go)" == 0 ]] || fail "built before refusing"
}

test_release_linux_only_refuses_to_overwrite_without_force() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  local before
  before="$(tree_hash "$DIST")"
  release --linux-only 0.2.1
  assert_failed
  assert_output_contains "$DIST/v0.2.1 already exists"
  assert_output_contains "make release-linux VERSION=0.2.1 FORCE=1"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ "$(stub_calls go)" == 0 ]] || fail "built before refusing"
}

test_release_linux_only_force_drops_the_artifacts_of_the_earlier_build() {
  setup_release
  release 0.2.1
  assert_status 0
  local before_linux
  before_linux="$(cat "$DIST/v0.2.1/nu11signal-0.2.1-linux-amd64.tar.gz.sha256")"
  sleep 1 # a distinct backup timestamp and archive mtime
  release --linux-only --force 0.2.1
  assert_status 0
  # Only this build's Linux archives: never the macOS artifacts of another build.
  assert_linux_release "$DIST/v0.2.1" 0.2.1
  [[ "$(cat "$DIST/v0.2.1/nu11signal-0.2.1-linux-amd64.tar.gz.sha256")" != "$before_linux" ]] ||
    fail "the Linux archives were not rebuilt"
  local backups
  backups="$(find "$DIST" -maxdepth 1 -name 'v0.2.1.replaced-*')"
  [[ -n "$backups" && "$(printf '%s\n' "$backups" | wc -l | tr -d ' ')" == 1 ]] ||
    fail "expected one backup, got: $backups"
  assert_release_layout "$backups" 0.2.1
  assert_output_contains "not carried over from the previous $DIST/v0.2.1"
  assert_output_contains "nu11signal-0.2.1-macos-universal.tar.gz"
  assert_output_contains "make release VERSION=0.2.1 FORCE=1"
  assert_output_lacks "kept from the previous"
}

test_release_linux_only_failure_leaves_dist_untouched() {
  setup_release
  mkdir -p "$DIST/v0.2.0"
  printf 'published\n' >"$DIST/v0.2.0/marker"
  local before
  before="$(tree_hash "$DIST")"
  STUB_GO_WRONG_ARCH=1 release --linux-only 0.2.1
  assert_failed
  assert_output_contains "not an x86_64 ELF"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ changed"
  [[ -z "$(staging_left "$DIST")" ]] || fail "staging left behind: $(staging_left "$DIST")"
  [[ "$(stub_calls mv)" == 0 ]] || fail "something was promoted"
}

test_release_linux_only_force_restores_the_previous_release_when_promotion_fails() {
  setup_release
  mkdir -p "$DIST/v0.2.1"
  printf 'published\n' >"$DIST/v0.2.1/marker"
  local before
  before="$(tree_hash "$DIST")"
  STUB_MV_FAIL_STAGING=1 release --linux-only --force 0.2.1
  assert_failed
  assert_output_contains "could not rename"
  [[ "$(tree_hash "$DIST")" == "$before" ]] || fail "dist/ was not restored: $(ls -A "$DIST")"
}

test_release_linux_only_dry_run_writes_only_the_dry_run_dir() {
  setup_release
  rm "$T/root/signing/Nu11Signal_DeveloperID.provisionprofile"
  STUB_GIT_DIRTY=1 release --linux-only --dry-run 0.2.1
  assert_status 0
  assert_linux_release "$T/root/build/release-dry-run/v0.2.1" 0.2.1
  [[ ! -e "$DIST" ]] || fail "a dry run created dist/"
  release --linux-only --dry-run 0.2.1
  assert_status 0
  [[ -z "$(staging_left "$T/root/build/release-dry-run")" ]] || fail "staging left behind"
}
