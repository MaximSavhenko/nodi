# Changelog

## 0.5.0 — 2026-10-12

- Verify the entire release asset snapshot and archive hashes immediately before deletion (B01).
- Recover proven interrupted publication and exact partial deletion using durable private journals; preserve tombstones and reject changed/foreign objects (B02).
- Retain the chosen first-send name before mutation, and reconcile its original version after a lost final read (B03).
- Commit a complete verified download directory with native no-replace operations; recover the download record after a commit/record gap (B04).
- Identify Windows/macOS sources by filesystem object, recognizing aliases and rejecting recreated objects after migration (B05).
- Preserve independent repository/version/destination download records and read legacy records (B06).
- Choose latest before fetching manifests; healthy latest is independent of damaged history, while damaged latest is never downgraded (B07).
- Share strict releases/reserved-refs/tombstone version preflight for automatic and explicit publication (B08).
- Report exclusions on normal and unchanged send; add exact `--include` for ordinary data and accept explicitly selected build/dist roots (B09).
- Verify installer candidates before replacement, retain a previous-binary backup, and restore it on failed postchecks (B10).
- Provide side-effect-free `-h`/`--help` for every public command, while retaining unknown-argument errors (B11).

Manifest schema 1, ZIP/file SHA-256, read/write credentials, immutable version history, existing project bindings, normal no-prompt send, and old local records remain supported. Partial catalog output now includes issues and returns nonzero exit. Pending recovery requires the same exact source/selection/status/version. Unknown legacy orphans and ambiguous lightweight-tag reservation replies still require inspection.

Native CI covers Windows amd64, macOS ARM64/Intel, macOS race, case-sensitive APFS, real installers, upgrades from 0.3.0/0.4.1, rollback and user-data preservation. Hardware power loss and a physical-device pilot remain separate acceptance work. No Nodi Server implementation is included.
