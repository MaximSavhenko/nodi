Nodi 0.4.1 fixes safe deletion of draft package Releases that GitHub does not expose through the release-by-tag endpoint. It fetches the approved release by its fixed ID and verifies both ID and tag before deletion. Before removing a Git tag, it checks the complete authenticated release inventory: a replacement draft with the same tag keeps that tag intact.

Permanent deletion-history tombstones remain mandatory and retain the original tag SHA. All writers must use 0.4.0 or newer before deleting package versions. Never delete the history refs manually. Local downloaded files, credentials and package configuration remain unchanged.

This is a bug fix without new features or data-format changes. Existing stable release tags and assets, including v0.4.0, are not replaced. Windows amd64 and macOS ARM64/Intel CI cover the draft regression, upgrade from 0.3.0, completion and installation. Physical acceptance on user devices is separate.

Windows: download the release's `install-windows.ps1`, then run `powershell -ExecutionPolicy Bypass -File .\install-windows.ps1 -Repository MaximSavhenko/nodi -Release v0.4.1`.

macOS: after the Homebrew tap is updated, run `brew update && brew upgrade MaximSavhenko/nodi/nodi` and verify `nodi version`.

Winget manifests are validated separately; Microsoft PR #450627 remains the existing initial submission. Public Winget catalog availability is not implied.
