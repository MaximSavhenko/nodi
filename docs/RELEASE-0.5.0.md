Nodi 0.5.0 fixes the B01–B11 reliability findings: exact safe deletion, journal-backed publication/deletion recovery, first-send identity recovery, atomic complete downloads and record reconciliation, native filesystem identities, independent download records, strict latest selection/version preflight, visible file exclusions, installer backup/rollback, and help for every public command.

Normal transfer remains `nodi send PATH`. Repeating an interrupted send/publish recovers only the proven original operation, retaining its version and prepared ZIP. Repeat an interrupted exact delete with the same selection and typed confirmation. Changed/foreign releases, tags, assets or corrupt journals are refused; tombstones remain permanently. Unknown legacy orphans and ambiguous lightweight-tag reservation replies are not adopted automatically.

Preview exact ordinary configuration inclusion with `nodi send PATH --include config.json --dry-run`. Repeat with the same include flags for publication/recovery. Credential/key/hidden/private-Nodi-config rules remain enforced. Name-based rules cannot detect secrets disguised inside ordinary documents.

Installers verify candidate checksum, executable platform and runtime version before replacing the old CLI, retain its backup and restore it if postcheck fails. Local bundles accept optional expected version `-Version 0.5.0` / `--version 0.5.0`; remote installation uses the requested release version automatically. Backup paths are printed. An interrupted macOS installer lock requires exact-path inspection before reuse.

Windows: `powershell -ExecutionPolicy Bypass -File .\install-windows.ps1 -Repository MaximSavhenko/nodi -Release v0.5.0`.

macOS: `sh install-macos.sh --repo MaximSavhenko/nodi --release v0.5.0 --no-path --no-completion`, then verify `~/.local/bin/nodi version`. Published formula/manifests are provided as assets; public Homebrew tap/Winget catalog availability is a separate publication step.

Manifest schema 1 and legacy project/receipt/download formats remain supported. Damaged catalogs display healthy results plus issues and return nonzero exit; damaged latest never falls back silently. New identities reject recreated source objects; legacy records cannot reconstruct object history before migration.

`validation.json` identifies the exact source commit and CI run for these assets. Native checks cover Windows amd64 and macOS ARM64/Intel, race, case-sensitive APFS, upgrades from 0.3.0/0.4.1, rollback, data and completion preservation. CI/process-kill tests do not prove hardware power-loss durability or physical-device acceptance. GitHub lacks a distributed transaction against another authorized writer after final verification. No 0.6.0 server code is included.
