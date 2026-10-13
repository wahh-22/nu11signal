# Android TV follow-ups
Close deferred review/documentation work and stop unsupported spectrum-capture retries.
No new APK release or device installation is part of this follow-up.

## Specs
1. S1 — User: "ok, sigue con esos pendientes"; confirmed "si" to including "la mejora opcional de los reintentos de audio en Android, además de subir el cierre documental y retomar las dos revisiones diferidas".

## Tasks
- [x] F1 — S1: Revisit exact commits 16256c3 and a32b08b with native review. Route: parent native lifecycle. Evidence commit: this planning record.
- [x] F2 — S1: Stop capture retries for unsupported platforms, preserve transient Linux retry and lifecycle behavior. Route: inline test-first + independent verifier (concurrency). Commit: 558da28.
- [x] F3 — S1: Publish verified correction and integration closing document. Route: parent push/PR with CI. PR88 contains c8b2999 (original 8859fc0), d1cd611 and correction 558da28; Linux/macOS CI passed on that candidate. Merge and APK publication are separate decisions.

## Log
- L7: PR88 published at https://github.com/wahh-22/nu11signal/pull/88; Linux/macOS checks passed (job47 exit0). Native review review-d603fa873c8a3e77 approved resilience/reliability and exact acknowledgement completed. All requested follow-ups are implemented and published; no PR merge or new APK release/installation performed. This final record changes documentation only.
- L5: RED observed for direct and wrapped ErrUnsupported (unexpected retry wait). GREEN:10 focused race tests; full vet/fmt, Go race, Swift,79 script tests and Android ARM/ARM64 build passed (job44). Independent verifier mv771euw-g-wzam passed271 race tests across webplayer/pulsecapture/androidtv; no blockers. WaitGroup/gate warnings are false positives, rangeint hint unrelated. No runtime TV installation performed.
- L6: Native assessment of the46-line code/test change: medium, reviewDue=false. Historical F1 reviews are approved/acknowledged; no further review required for this candidate by the native plan. Destination main remains7ac0baf, approved issue82 open, account wahh-22 verified. Publication pending.
- L1: User: "ok, sigue con esos pendientes". Clarification: "¿Incluyes también la mejora opcional de los reintentos de audio en Android, además de subir el cierre documental y retomar las dos revisiones diferidas?" User: "si".
- L2: Exact download commit16256c3 approved/acknowledged in review-5f37cd37c916cf7e (risk/resilience/reliability); exact plan a32b08b approved/acknowledged in review-6532f0e0924c3515 (low). Both deferrals resolved. Generated historical test artifacts preserved under ignored android/build, not deleted.
- L3: Branch fix/unsupported-capture-retry starts from main7ac0baf; c8b2999 cherry-picks verified closing-document commit8859fc0. Existing approved issue82 provides context. No change to release0.0.6 or TV.
- L4: Current captureLevels retries every error while playing, including pulsecapture.ErrUnsupported on Android/macOS. Keep transient-error recovery and pause/stop/close behavior; terminate the current capture worker on an unsupported-platform error without UI errors or spectrum output. Planned authored scope below400 lines; one cohesive follow-up PR, no cumulative Android/vendor review.
