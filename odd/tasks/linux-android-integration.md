# Linux and Android integration
Integrate the completed Linux spectrum and Android TV/update branches into main.
Preserve existing commits and PR history; do not publish an APK or release.

## Specs
1. S1 — User: "ok, sigue", then "si" in response to "¿Autorizas integrar las ramas de Linux y Android, resolver posibles conflictos y fusionar los PR hacia `main`, **sin publicar todavía la APK**?"
2. S2 — Preserve the earlier review disposition: "si, sigue haciendo para los nuevos commits". The download commit 16256c3 remains explicitly deferred; previously approved slices are not reopened as a cumulative review.

## Tasks
- [x] I1 — S1–S2: Map ancestry and semantic integration risks. Route: parent Git evidence + read-only explorer. Commit: a32b08b.
- [x] I2 — S1: Integrate updater children into the tracker, then Linux spectrum, preserving history. Route: parent Git/GitHub integration. Commits: 1998eaf (updater chain), 854fa9e (Linux integration); both original tips are ancestors.
- [ ] I3 — S1–S2: Verify combined Go, script, Java and Android-build behavior; independent integration verification and new-change review as applicable. Route: parent checks + independent verifier. Commit: pending evidence record.
- [ ] I4 — S1: Merge verified tracker to main and verify resulting ancestry/CI, without releasing APKs. Route: parent authorized GitHub merge. Commit: pending final merge.

## Log
- L9: User authorized an exception only for a32b08b after native-start-retained-selection-candidate-mismatch; no lineage created. Reviews remain enabled for future changes.
- L11: Combined-tree local checks passed: job27 exit0 (make vet fmt-check, CGO_ENABLED=0 go build ./..., make test including Go race, Swift and79 script tests); job28 exit0 (Java121/35/19/11 assertions, SDK34 compile, dev APK ARM/ARM64). Build warnings concern deprecated APIs only. No combined APK installed or published; live GitHub update-channel HTTPS remains untested. Independent verifier mv723017-f-n1vb is running; CI83 and final main merge remain pending.
- L10: PR87 merged929df41, PR86 merged593f86d, PR85 merged9817ee1, PR84 mergedd80e626, each gated on successful checks. A transient no-checks-yet stop after PR86 was resolved by inspecting newly dispatched CI, not bypassing it. Local tracker incorporated the chain in1998eaf and Linux in854fa9e with automatic merges, no manual source edits. Both original tips are ancestors; main unchanged.
- L1: User request: "ok, sigue". Clarification: "¿Autorizas integrar las ramas de Linux y Android, resolver posibles conflictos y fusionar los PR hacia `main`, **sin publicar todavía la APK**?" User authorization: "si".
- L2: Account wahh-22 is ADMIN on github.com/wahh-22/nu11signal; default main=8e99dd35e3e9550c6896b63799d229ca3f36a7c3. Merge commits are allowed. PR83 is draft tracker; children PR84–87 have correct sequential bases. Existing approved issue82 remains linked with nonclosing Refs.
- L3: Linux tip397a63161f40 and Android device-test ec565771eee8 share main8e99dd3. Read-only merge-tree succeeded without textual conflicts, tree7444840dc8ad4283fa239fcf423b1f0cd571a4fa. This is not semantic verification.
- L4: PR84/86 UNSTABLE reflected cancelled superseded checks; replacement Linux/macOS checks succeeded on their current heads. No administrator bypass will be used.
- L5: Linux task document exists only on feat/linux-spectrum until integrated; inspected with git show. L1–L4 complete with real WSL evidence. Linux golden change6cf7b29 previously exceeded native review budget and was covered by suite1491; no cumulative re-review is planned.
- L6: Linux adds webplayer capture hooks, but Android excludes Linux Pulse backend via !linux || android fallback. Android owns its Levels source. Generic unsupported-platform capture can retry silently while playing; inspect combined behavior rather than assuming a clean textual merge is safe.
- L8: Explorer completed: no textual conflicts; preserve Android bridge Levels and Linux 9/20 layout. Merge children tail-to-root (#87, #86, #85, #84), without deleting branches, then merge Linux into tracker. Combined verification must cover Go/scripts/Java and Android ABIs because GitHub CI omits Java updater suites. Unsupported-platform capture retries are nonblocking follow-up, not a demonstrated integration defect.
- L7: Delivery remains the existing feature-branch chain. New authored integration changes are expected to be documentation only unless checks reveal a concrete defect. Preserve all source commits; no force-push, branch deletion, credentials, APK binaries or public release.
