# Android TV confirmed APK updates
Add a remote-first updater to the Android shell; retain Android installation confirmation.
Base: feat/android-tv at 841c44a. No publishing, merging, credential access or TV uninstall is authorized by this feature.

## Specs
- S1: User requested: "si, y que se autoactulice solo, es posible?" and accepted the confirmed-APK proposal with "si".
- S2: Accepted proposal: "comprobar únicamente releases con APK de Android TV, avisar cuando haya una nueva y ofrecer **Descargar e instalar**, verificando su firma antes de instalar."
- S3: Accepted proposal: "¿Implemento esa opción con confirmación, manteniendo la distribución por APK?" User: "si".
- S4: User review constraint: "si, sigue haciendo para los nuevos commits".

## Tasks
- [x] T1 — S1–S3: Implement tested release/download and validation/installer adapters. Route: delegated parallel workers on disjoint Java/test surfaces. Commits: 16256c3 (download), aa0754d (installer). Backend checks and independent verification passed. Download native review explicitly deferred by user; installer native review pending T3.
- [x] T2 — S1–S3: Integrate lifecycle-safe native remote UI and document the update flow. Route: inline. UI/state, MainActivity hooks, docs and lifecycle tests pass; independent targeted recheck resolved the SEARCH focus blocker. Commit: this work unit (resolve with git log -- android/src/dev/wahh/nu11signal/tv/TvUpdateUi.java).
- [ ] T3 — S1–S4: Functional build/checks, independent security verification and scoped native review; device confirmation when available. Route: inline coordination + independent verify (risk items 2, 4, 5). In progress: build and independent checks pass; native review and device proof pending.

## Design and verification
- Entire updater lives in Java shell; no WebView JavaScript bridge for install/download and no desktop Go updater changes.
- Fixed official GitHub repo, stable core versions, exact APK asset names; bounded discovery pages/network bytes/timeouts and private unique downloads. Missing APKs are not update offers. No publishing is required for fixtures.
- Installation guard: own package, greater versionCode, exact advertised version/code, non-debuggable, pinned release certificate and installed-certificate compatibility. Android package installer must verify actual APK integrity before installation; certificate metadata alone must never be described as a full integrity proof.
- Use a narrowly scoped non-exported read-only provider and ACTION_INSTALL_PACKAGE with temporary URI grant, avoiding session callback complexity. Unknown-source permission and per-install approval are distinct. Revalidate after settings return; no automatic background installer launches.
- Plain Java policy/transport tests with injected fixtures where possible; compile Android adapters against SDK, record platform-only gaps explicitly. RED before each testable rule, GREEN afterward; preserve exit codes/stdout/stderr. Independent verifier after own checks. No live public release creation or device data loss.
- Forecast: approximately 900–1400 authored diff lines including tests/docs; separate backend/integration review units, no cumulative branch review. Delivery strategy: auto-chain, chain_strategy=feature-branch-chain, authorized by user; no push/PR authorization. Existing app code and tests remain intact.

## Log
- L1: User: "si, y que se autoactulice solo, es posible?"
- L2: User: "continua"
- L3: Assistant explained Android confirmation for sideload updates and proposed: "**Recomiendo:** comprobar únicamente releases con APK de Android TV, avisar cuando haya una nueva y ofrecer **Descargar e instalar**, verificando su firma antes de instalar." Asked: "¿Implemento esa opción con confirmación, manteniendo la distribución por APK?" User: "si".
- L4: Explorer mapped native overlays/lifecycle, platform-only javac build, deterministic release naming, existing Go checker intentionally disabled on TV. Source map: MainActivity.java lifecycle 732–760 and key routing 902–1053; android/build.sh 16–26, 58–69; scripts/android-release.sh 47–52,76–95.
- L5: Android documentation confirms canRequestPackageInstalls source permission is separate from installation confirmation. PackageManager signing metadata must not be confused with full archive integrity; investigate platform behavior and rely on system installer validation as mandatory final boundary.
- L6: User selected "Cadena sobre feat/android-tv — recomendado" (feature-branch-chain), authorizing separate work-unit commits. No publishing, push or PR creation.
- L7: T1 parent seam check: client runner passed 121 assertions; installer runner passed 35 guard + 19 snapshot lifecycle assertions and Android 34 compilation, exit 0. Earlier run during active correction failed on missing SnapshotPool; final joint run resolves it. Native ASSESS could not classify undeclared untracked files; treated as high and independent verifier mv6v3dvk-8-a1qg launched before closing T1. Installation on actual TV remains untested.
- L8: Independent verifier mv6v3dvk-8-a1qg accepted T1 without confirmed blockers; host suites repeated successfully. Canonical directory is an integration precondition.
- L9: Download commit 16256c3 on feat/android-tv-update-download. Native START failed retained-selection-candidate-mismatch before lineage creation; user answered "si" to continue while leaving only this commit pending native review, not disabling future reviews.
- L10: Installer commit aa0754d on feat/android-tv-update-installer, stacked on download branch. No TV or publishing operations performed.
- L11: T2 lifecycle RED: missing TvUpdateState compilation, exit 1. GREEN: 7 lifecycle assertions. Combined seam run exit0: UI7/client121/guard35/snapshot19 and SDK34 compile. Background full debug APK build job-1 exit0 for arm + arm64; no release credentials or TV operations. Native dialogs use long Menu; short Menu still search. Independent integration verifier mv6vmdg2-9-qtpu launched; T2 not yet closed at that point.
- L12: Independent T2 verification reproduced SEARCH/IME offer race. Added inputIdle predicate used in MainActivity; behavioral RED exit1 then GREEN11 assertions. Targeted recheck mv6vtngq-a-dzaa resolved blocker using extracted production predicate and real copied UI/state sources; confirmed offers blocked during pending SEARCH or IME and eligible when idle. Full corrected APK build job-2 exit0 (both ABIs). No actual device installation proof yet.
