# NODI-02: состояние распространения

Дата: 2026-10-11. Владелец: MaximSavhenko. Лицензия публичного кода: MIT (выбрана владельцем).

Репозитории:

- [nodi](https://github.com/MaximSavhenko/nodi) — public, исходники, CI и бинарные релизы.
- [homebrew-nodi](https://github.com/MaximSavhenko/homebrew-nodi) — public, подтверждённые Homebrew Formula и native проверки.
- [nodi-packages](https://github.com/MaximSavhenko/nodi-packages) — private, пакеты и scoped transport workflow.

До создания проверены 28 принадлежащих пользователю репозиториев; совпадений этих имён не было. Доступ к новым репозиториям: admin/push; существующие сторонние проекты не менялись. Полные токены не входят в исходники, логи и пакеты.

## Проверки до выпуска

- Windows amd64: Go tests, vet и native release binary — PASS.
- macOS ARM64: Go tests, vet, race и native release binary — PASS.
- macOS Intel: Go tests, vet, race и native release binary — PASS.
- [Подтверждающий CI](https://github.com/MaximSavhenko/nodi/actions/runs/38142720287).
- Локальный `winget validate`: PASS без предупреждений, WinGet v1.29.380.
- Пакет Windows: ZIP с `nodi.exe` и MIT LICENSE, alias `nodi`, SHA-256 ZIP.
- macOS Formula: разные ZIP/SHA-256 для ARM64 и Intel, готовые бинарники, без сборки Go.

На момент этого коммита выпуск по тегу, actual Homebrew install, winget install и live private transport ещё готовятся. Их итоговые ссылки и состояние официального Microsoft PR будут внесены после завершения проверок. Не считать отсутствие итоговой отметки подтверждением успеха.
