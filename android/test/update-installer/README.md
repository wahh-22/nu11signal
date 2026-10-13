# Android update installer boundary

T1 installation/validation half covers S1–S3: a newer official APK can be offered,
validated off-thread and handed to Android for explicit per-install confirmation.
Discovery, download, UI, consent, and permission-return lifecycle belong to the caller.

## Integration API

Package: `dev.wahh.nu11signal.tv`. No dependency on `TvUpdateRelease`.

- `new TvUpdateInstaller(Activity activity)`
- `void validate(File apk, String expectedVersion, long expectedCode) throws IOException`
  runs on a worker. The source must be a regular, non-symlink `.apk` directly inside
  `activity.getFilesDir()/updates`. It cannot be a `verified-*` snapshot. Validation
  copies at most 256 MiB to a new `verified-<UUID>.apk`, closes all writers, seals it
  with mode `0400`, checks metadata, and caches an immutable handle. Provider authorization
  is registered only immediately before installation, not during validation.
- `boolean canInstall()` checks Android's per-source unknown-app permission.
- `void openPermissionSettings()` runs on the foreground main thread only after
  explicit user choice. It clears authorization, then opens this app's source settings.
  Caller handles unavailable Settings activities and permission denial.
- `void install(File apk, String expectedVersion, long expectedCode) throws IOException`
  runs on the foreground main thread only after explicit user choice. The source File
  and release primitives must match the successful validation. Authorization expires
  after five minutes and is consumed on every attempt, including permission denial.
  This method does no file I/O. It selects a unique system installer explicitly and
  grants only temporary read access to the exact snapshot URI. No write, prefix,
  persistable grant, silent-install API, uninstall, or JavaScript bridge is used.
- Additional cancellation hook: `void clearValidation()`. Call on cancellation or
  lifecycle invalidation. It prevents publication by an in-flight worker; also cancel
  the worker with interruption to stop copying promptly.

After a Settings roundtrip, recheck permission and revalidate on a worker. Do not
launch installation automatically from `onResume`. Keep the same adapter instance
for the validation/install pair. No interface change to the requested methods is needed.

Never modify a `verified-*` snapshot. Original downloads can change without changing
validated bytes. Failed/partial/cancelled validations retire and unlink their never-handed-off
snapshot on the worker. Supersession, permission denial, expiration and clearValidation
invalidate handles immediately and schedule background cleanup. Failed installer launches
forget provider authorization, revoke the URI read grant and retire the snapshot. No file
I/O occurs in install or clearValidation. A single validation lock serializes copying;
cleanup has at most one running and one queued sweep.

There is one process-wide pool with at most three snapshots (up to 768 MiB of copied APK
data). Successful handoffs remain authorized and retained for the rest of that process;
neither invalidation nor supersession removes them. Deletion failures retain their slot;
new validation fails closed when capacity is exhausted. The pool is not per Activity.

On the first worker validation in a NEW PROCESS, the pool unlinks only canonical exact
`verified-<UUID>.apk` names from the private updates directory before creating new snapshots.
This is safe because old provider authorization was lost at process death: an old URI
cannot open or reopen its APK. Android/Linux already-open descriptors continue reading the
same inode after unlink. A restart before the installer opens its APK therefore requires
fresh validation and consent, not automatic resumption. Cleanup failure prevents new copies.
The same-process pool never performs this sweep again. Original downloads, unrelated names
and other paths are not deleted. This bounds future retention across process restarts;
legacy accumulated files are cleaned before further copies are admitted.

## Official source evidence

Read-only HTTP inspection of the following AOSP tags was successful:

- [Android 14 PackageManager.java](https://github.com/aosp-mirror/platform_frameworks_base/blob/android-14.0.0_r1/core/java/android/content/pm/PackageManager.java):
  `getPackageArchiveInfo` with signing flags calls `collectCertificates(pkg, false /* skipVerify */)`;
  parse errors return null.
- [Android 9 PackageParser.java](https://github.com/aosp-mirror/platform_frameworks_base/blob/android-9.0.0_r1/core/java/android/content/pm/PackageParser.java):
  `collectCertificates` uses `ApkSignatureVerifier.verify` when `skipVerify` is false;
  the true branch deliberately skips verification for trusted system APKs.
- [Android 14 ApkSignatureVerifier.java](https://github.com/aosp-mirror/platform_frameworks_base/blob/android-14.0.0_r1/core/java/android/util/apk/ApkSignatureVerifier.java):
  normal `verify` passes `verifyFull=true`; unsafe certificate collection passes false.
- [Android 14 InstallPackageHelper.java](https://github.com/aosp-mirror/platform_frameworks_base/blob/android-14.0.0_r1/services/core/java/com/android/server/pm/InstallPackageHelper.java):
  installation collects signing details with `skipVerify=false` unless previously supplied,
  and checks installed signing compatibility.

These sources support verification in the inspected AOSP implementations. They are not
an OEM-wide public-API cryptographic guarantee. The guard compares metadata only; the
system package installer remains the mandatory final APK integrity/signature verifier.
Host tests do not establish APK integrity or prove device confirmation behavior.

## Validation evidence

Run `bash scripts/test-android-update-installer.sh`. Compiler/test outputs are retained
under ignored `android/test/update-installer/build.*` directories, never beside sources.

Observed RED before implementation, exit 2 (stdout empty; stderr):

```text
error: file not found: /Users/wilmerhenao/wahh22/nu11signal/android/src/dev/wahh/nu11signal/tv/TvUpdateGuard.java
Usage: javac <options> <source files>
use --help for a list of possible options
```

This was a missing-boundary compile failure, not an observed behavior assertion failure.
Observed GREEN after implementation, exit 0:

```text
PASS: 30 installer guard assertions
PASS: platform-only installer boundary compiled against /Users/wilmerhenao/Library/Android/sdk/platforms/android-34/android.jar
```

Triangulation added installed-package/unsigned rejection, symlink escape rejection and
signer-array defensive-copy coverage. A subsequent behavior-level RED exposed a missing
private-directory error: exit 1, stdout empty, stderr:

```text
Exception in thread "main" java.lang.NullPointerException: Cannot invoke "java.io.File.getCanonicalFile()" because "<parameter1>" is null
	at dev.wahh.nu11signal.tv.TvUpdateGuard.resolveReadPath(TvUpdateGuard.java:56)
	at dev.wahh.nu11signal.tv.InstallerGuardTest.lambda$main$10(InstallerGuardTest.java:58)
	at dev.wahh.nu11signal.tv.InstallerGuardTest.reject(InstallerGuardTest.java:21)
	at dev.wahh.nu11signal.tv.InstallerGuardTest.main(InstallerGuardTest.java:58)
```

Adding an actionable IOException for a missing private directory produced final GREEN,
exit 0, stdout:

```text
PASS: 35 installer guard assertions
PASS: platform-only installer boundary compiled against /Users/wilmerhenao/Library/Android/sdk/platforms/android-34/android.jar
```

Both successful compiles emitted these expected stderr diagnostics:

```text
Note: /Users/wilmerhenao/wahh22/nu11signal/android/src/dev/wahh/nu11signal/tv/TvUpdateInstaller.java uses or overrides a deprecated API.
Note: Recompile with -Xlint:deprecation for details.
```

`ACTION_INSTALL_PACKAGE` is deliberately retained per the agreed contract. The build uses
Java 11, Android 34 stubs and only public platform APIs available at API 28. No full app
build, dependencies, signing, ADB or device install was run. `git diff --check` passed.

## Review and device gaps

- JVM coverage: wrong package/version/code, debug APK/install, missing/wrong/multiple
  signers, installed signer policy, same-code/downgrade, provider path traversal,
  write modes and symlink escape. It exercises shared public guard contracts.
- Android adapter, provider Binder permissions, temporary URI grants, expiration,
  cancellation, and main-thread enforcement are compile-checked, not runtime-tested.
- Device verification must cover unknown-source denial/return, user cancellation,
  actual installer confirmation, tampered APK rejection, process death, and OEM system
  installer resolution. Multiple matching system installers fail closed rather than
  choosing a handler or opening a resolver. Successful handoffs consume one of three slots.
- Provider serves only exact in-process authorized snapshot names. Private directory,
  canonical path checks and mode `0400` protect against ordinary external writes;
  arbitrary same-UID code can override file permissions and must not modify snapshots.
- Peer and pre-existing files were preserved. Explicit javac commands always use `-d`
  under this test directory. Source-directory guard `.class` artifacts appeared during
  write-time Java diagnostics and were left untouched; see follow-up evidence below.

## Retention correction evidence

New lifecycle tests initially failed to compile because the shared pool types did not
exist (exit 1, eight missing-symbol errors). After extracting the prior retain-all behavior
into that pool, the observed behavior-level RED was exit 1:

```text
PASS: 35 installer guard assertions
Exception in thread "main" java.lang.AssertionError: Rejected snapshot retained
	at dev.wahh.nu11signal.tv.SnapshotLifecycleTest.check(SnapshotLifecycleTest.java:10)
	at dev.wahh.nu11signal.tv.SnapshotLifecycleTest.main(SnapshotLifecycleTest.java:19)
```

Implementing retired-snapshot cleanup produced GREEN: 35 guard assertions and 7 lifecycle
assertions, plus successful Android 34 adapter compilation. Additional triangulation covers
sealed rejected files, repeated rejection cleanup, and refusal to resurrect retired handles.
These tests exercise the lifecycle contract used by the adapter, not Android Binder runtime.
No separate per-rule RED is claimed.

Final command `bash scripts/test-android-update-installer.sh` exited 0. Exact stdout:

```text
PASS: 35 installer guard assertions
PASS: 19 snapshot lifecycle assertions
PASS: platform-only installer boundary compiled against /Users/wilmerhenao/Library/Android/sdk/platforms/android-34/android.jar
```

Stderr contained the same two deprecated-API notes shown above. `git diff --check` exited 0.
The Android compile includes the provider, its authorize/forget path, ClipData/read-only
intent grant, explicit system installer component, and `revokeUriPermission`; this confirms
public platform API compatibility, not runtime grant delivery.

Source-directory artifacts associated with this work are `TvUpdateGuard.class`,
`TvUpdateGuard$Metadata.class`, `TvUpdateGuard$Snapshot.class`, and
`TvUpdateGuard$SnapshotPool.class`. No explicit compile command targeted the source directory;
these appeared during automatic Java diagnostics on writes. The latter two appeared during
this correction; the former two already existed at its start. All were preserved for parent
cleanup, as were peer `TvUpdateRelease.class` and source files. No MainActivity edits,
commits, build.sh execution, signing, APK build, ADB, or TV operations occurred.
