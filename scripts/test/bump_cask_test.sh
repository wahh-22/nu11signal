# Tests for scripts/bump-cask.sh. Run through scripts/test/run.sh.

test_bump_rejects_non_semver_version() {
  bump 0.2
  assert_failed
  assert_output_contains "VERSION must be semver"
  [[ "$(stub_calls brew)" == 0 ]] || fail "brew was called before validation"
}

test_bump_requires_the_versioned_checksum() {
  setup_tap
  bump 0.2.1
  assert_failed
  assert_output_contains "checksum not found: $T/root/dist/v0.2.1/nu11signal-0.2.1-macos-universal.tar.gz.sha256"
  [[ ! -e "$TAP" ]] || fail "the tap was cloned without a checksum"
}

test_bump_rejects_a_checksum_for_another_archive() {
  setup_tap
  mkdir -p "$T/root/dist/v0.2.1"
  printf '%s  nu11signal-0.2.0-macos-universal.tar.gz\n' "$SHA_B" \
    >"$T/root/dist/v0.2.1/nu11signal-0.2.1-macos-universal.tar.gz.sha256"
  bump 0.2.1
  assert_failed
  assert_output_contains "not nu11signal-0.2.1-macos-universal.tar.gz"
}

test_bump_renders_and_audits_without_committing() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1
  assert_status 0
  assert_output_contains "Not pushed"
  diff <(render_cask 0.2.1 "$SHA_B") "$TAP/Casks/nu11signal.rb" || fail "unexpected render"
  [[ "$(stub_calls 'brew audit')" == 1 ]] || fail "brew audit was not run once"
  [[ "$(git -C "$TAP" rev-list --count HEAD)" == 1 ]] || fail "a commit was created"
  [[ "$(origin_commits)" == 1 ]] || fail "origin changed"
  [[ -z "$(ls "$STUB_BREW_REPO/Library/Taps")" ]] || fail "the temporary audit tap was left behind"
}

test_bump_refuses_a_tap_with_an_untracked_file_elsewhere() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf 'x\n' >"$TAP/notes.txt"
  bump 0.2.1
  assert_failed
  assert_output_contains "uncommitted changes"
  assert_output_contains "notes.txt"
}

test_bump_refuses_an_untracked_file_next_to_the_cask() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf 'x\n' >"$TAP/Casks/nu11signal.rb.orig"
  bump 0.2.1
  assert_failed
  assert_output_contains "Casks/nu11signal.rb.orig"
}

test_bump_refuses_a_modified_tracked_file() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf 'changed\n' >>"$TAP/README.md"
  bump 0.2.1
  assert_failed
  assert_output_contains "README.md"
}

test_bump_allows_a_cask_left_by_an_earlier_run() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf '# stale render\n' >>"$TAP/Casks/nu11signal.rb"
  bump 0.2.1
  assert_status 0
  assert_output_contains "Not pushed"
  diff <(render_cask 0.2.1 "$SHA_B") "$TAP/Casks/nu11signal.rb" || fail "unexpected render"
}

test_bump_push_commits_and_pushes_once() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1 --push
  assert_status 0
  assert_output_contains "Pushed"
  [[ "$(origin_commits)" == 2 ]] || fail "expected one new commit on origin"
  [[ "$(git --git-dir="$ORIGIN" log -1 --format=%s main)" == "chore: bump nu11signal to 0.2.1" ]] ||
    fail "unexpected commit subject"
  diff <(render_cask 0.2.1 "$SHA_B") <(git --git-dir="$ORIGIN" show main:Casks/nu11signal.rb) ||
    fail "origin cask differs from the render"
}

test_bump_push_with_an_unchanged_render_creates_no_commit() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1 --push
  assert_status 0
  bump 0.2.1 --push
  assert_status 0
  assert_output_contains "nothing to commit"
  [[ "$(origin_commits)" == 2 ]] || fail "a duplicate commit reached origin"
  [[ "$(git -C "$TAP" rev-list --count HEAD)" == 2 ]] || fail "a duplicate local commit was created"
}

# leave_unpushed_bump VERSION SHA: the state after `--push` committed but the
# push itself failed.
leave_unpushed_bump() {
  bump "$1"
  assert_status 0
  git -C "$TAP" commit --quiet -am "chore: bump nu11signal to $1"
}

test_bump_reports_an_unpushed_bump_without_push() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  bump 0.2.1
  assert_status 0
  assert_output_contains "unpushed"
  assert_output_contains "chore: bump nu11signal to 0.2.1"
  assert_output_contains "--push"
  [[ "$(origin_commits)" == 1 ]] || fail "pushed without --push"
}

test_bump_push_resumes_an_unpushed_bump() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  local bump_commit
  bump_commit="$(git -C "$TAP" rev-parse HEAD)"
  bump 0.2.1 --push
  assert_status 0
  assert_output_contains "Pushed"
  [[ "$(git --git-dir="$ORIGIN" rev-parse main)" == "$bump_commit" ]] ||
    fail "origin is not at the earlier bump commit (duplicate or missing push)"
  [[ "$(git -C "$TAP" rev-list --count HEAD)" == 2 ]] || fail "a duplicate local commit was created"
  [[ "$(stub_calls 'brew audit')" == 2 ]] || fail "the resumed push was not audited"
}

test_bump_push_refuses_an_unpushed_bump_that_differs_from_the_render() {
  setup_tap
  write_checksum 0.2.1 "$SHA_A"
  leave_unpushed_bump 0.2.1
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "does not match"
  [[ "$(origin_commits)" == 1 ]] || fail "a mismatched bump was pushed"
}

test_bump_refuses_unrelated_unpushed_commits() {
  setup_tap
  clone_tap
  write_checksum 0.2.1 "$SHA_B"
  printf 'local\n' >>"$TAP/README.md"
  git -C "$TAP" commit --quiet -am "docs: local change"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "docs: local change"
  [[ "$(origin_commits)" == 1 ]] || fail "unrelated commits were pushed"
}

test_bump_warns_about_an_unpushed_bump_that_differs_from_the_render_without_push() {
  setup_tap
  write_checksum 0.2.1 "$SHA_A"
  leave_unpushed_bump 0.2.1
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1
  assert_status 0
  assert_output_contains "does not match the render"
  assert_output_contains "reset --hard origin/main"
  [[ "$(origin_commits)" == 1 ]] || fail "pushed without --push"
}

test_bump_refuses_an_unpushed_bump_for_another_version() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  write_checksum 0.2.2 "$SHA_B"
  bump 0.2.2 --push
  assert_failed
  assert_output_contains "chore: bump nu11signal to 0.2.1"
  assert_output_contains "not bump commits for 0.2.2"
  [[ "$(origin_commits)" == 1 ]] || fail "unrelated commits were pushed"
}

test_bump_refuses_an_unpushed_bump_that_touches_other_files() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  bump 0.2.1
  assert_status 0
  printf 'local\n' >>"$TAP/README.md"
  git -C "$TAP" commit --quiet -am "chore: bump nu11signal to 0.2.1"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "change files other than Casks/nu11signal.rb"
  assert_output_contains "README.md"
  assert_output_lacks "      Casks/nu11signal.rb"
  [[ "$(origin_commits)" == 1 ]] || fail "a bump touching other files was pushed"
}

test_bump_explains_unpushed_bumps_with_no_net_change() {
  setup_tap
  write_checksum 0.2.1 "$SHA_B"
  leave_unpushed_bump 0.2.1
  render_cask 0.2.0 "$SHA_A" >"$TAP/Casks/nu11signal.rb"
  git -C "$TAP" commit --quiet -am "chore: bump nu11signal to 0.2.1"
  bump 0.2.1 --push
  assert_failed
  assert_output_contains "no net change"
  assert_output_contains "reset --hard origin/main"
  [[ "$(origin_commits)" == 1 ]] || fail "empty bump commits were pushed"
}
