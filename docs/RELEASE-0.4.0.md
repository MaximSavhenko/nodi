Nodi 0.4.0 completes the MVP package workflow: independent nested sends, explicit `send --name`, read-only `send --dry-run`, safe remote `delete --version/--all/--dry-run`, and `update --check`.

Deletion verifies manifests, archive SHA-256, release identity and tag ownership. Permanent `nodi-deleted/PROJECT/vVERSION` tombstones prevent reuse of deleted version numbers. **Upgrade every writer to 0.4.0 before deleting packages.** Writers running 0.3.0 do not understand tombstones. Never remove the history tags manually.

Existing schema-1 packages, separate saved read/write credentials, local downloads and persistent shell completion remain compatible. Windows amd64 and macOS ARM64/Intel binaries are published only after native CI, real 0.3.0 installer upgrade checks, shell completion and distribution validation. See `validation.json` for the exact source commit, checksums and CI link. Physical acceptance on the user's devices remains a separate step.

Windows: download `install-windows.ps1`, then run `powershell -ExecutionPolicy Bypass -File .\install-windows.ps1 -Repository MaximSavhenko/nodi -Release v0.4.0`.

macOS: `brew update && brew upgrade MaximSavhenko/nodi/nodi`, then `nodi version` and `nodi setup --completion --check`.

Winget manifests are included and validated, with installation checked against the published ZIP. Microsoft PR #450627 remains the existing initial submission; availability in the public Winget catalog is not implied. MIT licensed.
