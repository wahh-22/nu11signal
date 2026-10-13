# Hermetic Android release tests. Run through scripts/test/run.sh.
setup_android_release() {
  unset VERSION FORCE DEBUG NU11SIGNAL_TV_KEYSTORE NU11SIGNAL_TV_KEY_ALIAS \
    NU11SIGNAL_TV_KEYSTORE_PASS NU11SIGNAL_TV_KEY_PASS NU11SIGNAL_TV_CERT_SHA256
  cp "$REPO/scripts/android-release.sh" "$T/root/scripts/"
  mkdir -p "$T/root/android" "$T/sdk/build-tools/35.0.0"
  export ANDROID_HOME="$T/sdk"
  export NU11SIGNAL_TV_KEYSTORE="$T/home/release.keystore"
  : >"$NU11SIGNAL_TV_KEYSTORE"
  cat >"$T/root/android/build.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$DEBUG" == 0 && "$NU11SIGNAL_TV_KEY_ALIAS" == nu11signal-tv ]]
[[ -n "$NU11SIGNAL_TV_KEYSTORE_PASS" ]]
echo "android-build VERSION=$VERSION DEBUG=$DEBUG" >>"$STUB_LOG"
mkdir -p "$(dirname "$0")/build"
printf 'fake APK\n' >"$(dirname "$0")/build/nu11signal-tv-$VERSION.apk"
EOF
  cat >"$T/bin/security" <<'EOF'
#!/usr/bin/env bash
[[ "$*" == 'find-generic-password -a nu11signal-tv -s nu11signal-tv-keystore -w' ]] || exit 1
echo 'security keychain-lookup' >>"$STUB_LOG"
[[ "${STUB_KEYCHAIN_FAIL:-0}" != 1 ]] || exit 1
printf 'synthetic-test-credential\n'
EOF
  cat >"$ANDROID_HOME/build-tools/35.0.0/apksigner" <<'EOF'
#!/usr/bin/env bash
[[ "$1 $2" == 'verify --print-certs' ]] || exit 1
[[ "${STUB_VERIFY_FAIL:-0}" != 1 ]] || exit 1
echo "Signer #1 certificate SHA-256 digest: ${STUB_CERT:-448148925a1ecad2f51f3f5007343535d22d41716b4f0b9c4d88daaed0494320}"
[[ "${STUB_MULTISIGNER:-0}" != 1 ]] || echo "Signer #2 certificate SHA-256 digest: aaaa"
EOF
  cat >"$ANDROID_HOME/build-tools/35.0.0/aapt2" <<'EOF'
#!/usr/bin/env bash
[[ "$1 $2" == 'dump badging' ]] || exit 1
[[ "${STUB_BADGING_FAIL:-0}" != 1 ]] || exit 1
apk_version="$(basename "$3" .apk)"; apk_version="${apk_version#nu11signal-tv-}"
echo "package: name='dev.wahh.nu11signal.tv' versionCode='1' versionName='${STUB_VERSION_NAME:-$apk_version}'"
[[ "${STUB_DEBUGGABLE:-0}" != 1 ]] || echo application-debuggable
EOF
  chmod +x "$T/bin/security" "$ANDROID_HOME/build-tools/35.0.0/"{apksigner,aapt2}
}
android_release() { run bash "$T/root/scripts/android-release.sh"; }

test_android_release_requires_version() {
  setup_android_release
  android_release
  assert_failed
  assert_output_contains 'VERSION=x.y.z'
  [[ ! -e "$T/root/android/build" ]] || fail 'built without VERSION'
}

test_android_release_rejects_bad_version() {
  setup_android_release
  local v
  for v in dev 0.1 01.2.3 1.2.3-01 1.2.3-rc..1 '../1.2.3'; do
    VERSION="$v" android_release
    assert_failed
    assert_output_contains 'VERSION must be semver'
  done
}

test_android_release_missing_keystore() {
  setup_android_release
  NU11SIGNAL_TV_KEYSTORE="$T/home/missing.keystore" VERSION=0.0.1 android_release
  assert_failed
  assert_output_contains 'release keystore missing'
  assert_output_contains 'restore it from your backup'
  [[ "$(stub_calls security)" == 0 ]] || fail 'looked up password before checking key'
}

test_android_release_missing_keychain_item() {
  setup_android_release
  STUB_KEYCHAIN_FAIL=1 VERSION=0.0.1 android_release
  assert_failed
  assert_output_contains 'account nu11signal-tv, service nu11signal-tv-keystore'
  [[ ! -e "$T/root/dist" ]] || fail 'created dist on failure'
}

test_android_release_fingerprint_mismatch() {
  setup_android_release
  STUB_CERT=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa VERSION=0.0.1 android_release
  assert_failed
  assert_output_contains 'fingerprint mismatch'
  [[ ! -e "$T/root/dist" ]] || fail 'published mismatched APK'
}

test_android_release_rejects_multiple_signers() {
  setup_android_release
  STUB_MULTISIGNER=1 VERSION=0.0.1 android_release
  assert_failed
  assert_output_contains 'expected exactly one release signer'
}

test_android_release_rejects_debuggable() {
  setup_android_release
  STUB_DEBUGGABLE=1 VERSION=0.0.1 android_release
  assert_failed
  assert_output_contains 'APK is debuggable'
  [[ ! -e "$T/root/dist" ]] || fail 'published debuggable APK'
}

test_android_release_rejects_stale_manifest_version() {
  setup_android_release
  STUB_VERSION_NAME=dev VERSION=0.0.1 android_release
  assert_failed
  assert_output_contains 'APK version is not 0.0.1'
  [[ ! -e "$T/root/dist" ]] || fail 'published APK with the wrong version'
}

test_android_release_verification_failure() {
  setup_android_release
  STUB_VERIFY_FAIL=1 VERSION=0.0.1 android_release
  assert_failed
  assert_output_contains 'signature verification failed'
  STUB_BADGING_FAIL=1 VERSION=0.0.1 android_release
  assert_failed
  assert_output_contains 'badging verification failed'
}

test_android_release_refuses_overwrite() {
  setup_android_release
  mkdir -p "$T/root/dist/v0.0.1"
  printf 'old APK\n' >"$T/root/dist/v0.0.1/nu11signal-tv-0.0.1.apk"
  VERSION=0.0.1 android_release
  assert_failed
  assert_output_contains 'FORCE=1'
  assert_file_contains "$T/root/dist/v0.0.1/nu11signal-tv-0.0.1.apk" 'old APK'
  [[ "$(stub_calls android-build)" == 0 ]] || fail 'built before overwrite refusal'
}

test_android_release_dry_run_ignores_existing_dist() {
  setup_android_release
  mkdir -p "$T/root/dist/v0.0.1"
  printf 'old APK\n' >"$T/root/dist/v0.0.1/nu11signal-tv-0.0.1.apk"
  DEBUG=1 VERSION=0.0.1 DRY_RUN=1 android_release
  assert_status 0
  assert_output_contains 'signer fingerprint check passed'
  assert_output_contains 'APK is not debuggable'
  assert_output_contains "versionCode='1'"
  assert_file_contains "$T/root/dist/v0.0.1/nu11signal-tv-0.0.1.apk" 'old APK'
  [[ -f "$T/root/android/build/nu11signal-tv-0.0.1.apk" ]] || fail 'no dry run APK'
  assert_output_lacks 'synthetic-test-credential'
  ! grep -RqF 'synthetic-test-credential' "$T/root" "$T/stub.log" "$T/run.log" || fail 'credential leaked'
}

test_android_release_ci_password_and_force_preserve_desktop() {
  setup_android_release
  mkdir -p "$T/root/dist/v0.0.1"
  printf 'desktop\n' >"$T/root/dist/v0.0.1/desktop.tar.gz"
  printf 'old APK\n' >"$T/root/dist/v0.0.1/nu11signal-tv-0.0.1.apk"
  NU11SIGNAL_TV_KEYSTORE_PASS=synthetic-test-credential FORCE=1 VERSION=0.0.1 android_release
  assert_status 0
  [[ "$(stub_calls security)" == 0 ]] || fail 'CI still used keychain'
  assert_file_contains "$T/root/dist/v0.0.1/desktop.tar.gz" desktop
  assert_file_contains "$T/root/dist/v0.0.1/nu11signal-tv-0.0.1.apk" 'fake APK'
  (cd "$T/root/dist/v0.0.1" && shasum -a 256 -c nu11signal-tv-0.0.1.apk.sha256 >/dev/null) || fail 'checksum mismatch'
  assert_output_lacks 'synthetic-test-credential'
}

test_android_release_accepts_semver_and_cert_override() {
  setup_android_release
  STUB_CERT=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA \
    NU11SIGNAL_TV_CERT_SHA256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    VERSION=1.2.3-rc.1+build.7 DRY_RUN=1 android_release
  assert_status 0
  [[ ! -e "$T/root/dist" ]] || fail 'dry run created dist'
}
