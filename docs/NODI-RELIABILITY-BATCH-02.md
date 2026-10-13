# NODI-RELIABILITY-BATCH-02

Date: 2026-10-12, Europe/Kiev. All test mutations use fictional TLS fixtures, temporary sources/configs/installations. Real package releases/assets, credentials and active installations are untouched. The later user instruction authorizes sequential merge and 0.5.0 release only after all gates pass.

## Findings and reviewable changes

| Finding | Change / RED → GREEN evidence | PR | Base |
|---|---|---|---|
| B01 | Complete release/assets snapshot and final hashes; earlier A1 evidence | [#3](https://github.com/MaximSavhenko/nodi/pull/3), merged | main |
| B02 publish | A2 durable spool/journal, marker/IDs/checkpoints/lock | [#4](https://github.com/MaximSavhenko/nodi/pull/4) | main |
| B02 delete | Four before/after DELETE failures; partial batch restarts, eight unsafe refusal cases | [#5](https://github.com/MaximSavhenko/nodi/pull/5) | #4 |
| B03 | First file/directory custom name survives lost final GET; no second publication | [#6](https://github.com/MaximSavhenko/nodi/pull/6) | #5 |
| B04 | Killed child: 1/1200 files RED → 1200 verified GREEN; separate record-gap RED → GREEN | [#7](https://github.com/MaximSavhenko/nodi/pull/7) | #6 |
| B10 | Four bad candidate/postcheck RED → GREEN cases; locked target and expected-version checks | [#8](https://github.com/MaximSavhenko/nodi/pull/8) | #7 |
| B05 | Windows case alias loses custom identity RED → same object/name GREEN; legacy/recreation checks | [#9](https://github.com/MaximSavhenko/nodi/pull/9) | #8 |
| B06 | Different repositories and outputs overwrite records RED → independent records GREEN; concurrency/legacy | #9 | #8 |
| B07 | Damaged old version blocks latest RED → healthy latest and explicit catalog issues GREEN | [#10](https://github.com/MaximSavhenko/nodi/pull/10) | #9 |
| B08 | Explicit publish below orphan ref succeeds RED → read-only refusal GREEN; malformed refs | #10 | #9 |
| B09 | Hidden omissions/unsupported include RED → visible exclusions and preview/manifest parity GREEN | [#11](https://github.com/MaximSavhenko/nodi/pull/11) | #10 |
| B11 | Forty public help forms fail RED → side-effect-free help GREEN; unknown arguments stay errors | [#12](https://github.com/MaximSavhenko/nodi/pull/12) | #11 |

Reports and raw logs are linked from `NODI-RELIABILITY-{A2-B02,A3-B02,A4-B03,B04,B10,B05-B06,B07-B08,B09,B11}.md`. Every implementation ran full `go test -count=1 ./...` and vet on local Windows; native evidence comes from the corresponding exact-HEAD PR workflow. A2 exact `9333a53` was reviewed and CI run 38248147031 verified before any new branch. No blocking A2 defect was found. Later A4/B05/B09 extend its existing identity/selection fields without introducing a second journal engine.

Merge order: #4 → #5 → #6 → #7 → #8 → #9 → #10 → #11 → #12 → final validation/release preparation. Retarget the next PR to main after each predecessor merge; verify compatibility, exact head, fresh required checks and main before proceeding. Main protection requires strict build and all three native jobs, including administrators; force pushes and deletions are disabled. Protection was enabled under the explicit release gate.

## Validation and delivery gate

The final validation branch adds mandatory case-sensitive APFS identity checks, installer failures on PowerShell 5.1/7, and actual upgrades from 0.4.1 alongside existing 0.3.0 upgrade checks. Builds use candidate version 0.5.0. Native default insensitive-filesystem skips are justified by the separate mandatory case-sensitive APFS test; Windows-only case-alias/locked-executable tests are not applicable on macOS. Local Windows case-sensitive APFS/macOS/race execution is NOT VERIFIED; native CI supplies those checks.

No merge/release is complete merely because code compiles or a branch is pushed. Final CI and release evidence are recorded in the delivery report outside this candidate commit to avoid changing the artifact after validation. Tag/release creation requires all B01–B11 closed, exact-head required jobs successful, no unresolved compatibility/conflict/critical issue, full final-main tests, all native platforms, 0.4.1 upgrade/rollback/preservation, and verified release artifact/source provenance. Any missing mandatory gate stops integration/publication at the last safe state.

## Contract changes and limits

The strengthened B05 regression also requires one unchanged version after a file-case alias send. It exposed two versions on the earlier identity fix. Directory-entry spelling is now canonicalized using SameFile before packaging, preserving both the established identity and unchanged-file behavior. Separate case-content RED/GREEN evidence is in `batch/b0506`. The fix was applied to PR #9 and propagated through dependent PRs without force pushes; integration paused until their fresh HEAD checks passed.

Historical CI failures are preserved in Actions: B04 run 38251511855 timed out at the original 20-second NTFS flush barrier; the 120-second bound passed on the updated B04 HEAD. Final run 38255677783 detected inherited PowerShell 7 module paths preventing a PowerShell 5.1 fixture from reaching its injected phase. Child environments now resolve their own PowerShell modules, Windows fixtures retain their native home, and negative tests assert the actual failure reason to prevent false positives from unrelated environment errors. Only fresh successful checks on the corrected HEAD authorize delivery.

- Normal `send PATH` stays no-prompt; exact delete remains confirmation-gated. Recovery uses the same command/source/selection/status/version and original IDs, not matching names or contents.
- Optional `--include` is additive. Private-config/key/credential/path rules cannot be bypassed. Explicit ordinary config.json and build/dist roots are supported.
- Partial catalog results are displayed together with errors and nonzero exit. Latest selection never silently downgrades.
- Legacy records decode; optional local source-object/selection fields retain schema-1 package compatibility. Historical replacement of a legacy source before migration cannot be reconstructed.
- No-replace rename fails closed on unsupported filesystems/platforms. Windows file sync and process-kill evidence do not establish hardware power-loss durability. Local writers can race ancestor checks; GitHub writers can race the final remote snapshot check.
- Unknown orphan operations, corrupt committed records, changed remote identities and ambiguous reservation replies require inspection. Retained journals/spools/installer backups need space; no broad automatic cleanup is added.
- Physical-device pilot and hardware storage failures remain NOT VERIFIED. Installer process kill before/after replacement and rollback-failure injection preserve the original backup on local Windows; both platforms execute those cases in final native CI. Recovery from an interrupted installer remains exact-path manual inspection, preserving backups and the macOS lock. Native postcheck rollback and preservation are exercised on disposable environments. These limitations are not represented as physical acceptance.

## Nodi Server 0.6.0

The final concurrent-publication fixture rendezvous is now placed before reservation POSTs. The old barrier counted preflight GETs, which one writer could perform twice by itself; it did not guarantee actual contention. The new barrier requires both independent writers at the atomic reservation point, strengthening the one-winner/one-conflict oracle without relaxing assertions. A separate late-draft injection proves read-only conflict handling between preflight snapshots. All occupied-version paths retain the same explicit conflict classification.

Final native review also exercised scheduling where a competing writer completed a reservation between preflight reads. A known occupied version now reports `version conflict` even when its release is still incomplete, while unknown/network state remains a conservative verification error. Additional conflict-classification RED/GREEN logs are in `batch/b0708`; all ownership/hash/no-write guards remain in place. A Windows fresh-profile check on the B05 HEAD failed once and passed on a same-HEAD job rerun; no assertion was removed or skipped.

After a stable 0.5.0 pilot, design provider conformance around server operation IDs, idempotency keys and conditional commit/revision. Reuse prepared ZIP/manifest freezing, local journaling, before-mutation intent and local acknowledgement. Replace GitHub-specific refs/body markers/asset IDs with server-owned transaction proof. Keep schema/CLI compatibility and review deletion/recovery ownership before migration. Do not implement or deploy 0.6.0 as part of this batch.
