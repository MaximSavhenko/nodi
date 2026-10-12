# B07 / B08 latest and version preflight

Stacked on B05/B06 / PR #9. Implicit Get chooses the highest released version from complete paginated inventory before downloading a manifest. Damaged older history is not fetched to choose latest. Missing assets, bad status or duplicate highest versions fail strictly; there is no fallback to an older version.

List and ProjectList retain healthy results together with a nonnil aggregate error identifying damaged release tags. CLI renders those results and issues but returns a failing exit status; callers must inspect the error. Authentication/inventory failures remain fatal. This is an intentional CLI change from hiding all healthy results on the first damaged manifest.

Provider publication preflight now uses the same strict State/high-water rules as automatic CLI publication. Releases, drafts, reserved refs and tombstones are all considered before creating remote objects; malformed refs fail closed. Owned A2 recovery still resumes its existing operation before new-publication preflight. Unknown orphan reservations remain blocked until inspected/recovered, even if the requested new version is higher.

RED: damaged 0.1.0 blocks healthy latest 0.1.1; explicit 0.1.2 publishes under orphan 9.0.0. GREEN: both cases, healthy partial catalog with explicit issue, three damaged-latest cases, malformed-ref no-write case. Existing A1/A2/A3, numeric versions, pagination, read/write and history suites pass. Full Windows tests/vet PASS; [evidence](reliability/batch/b0708). Native PR CI required. Store method signatures and manifest schema are unchanged. No production operations, merge or release in this step.
