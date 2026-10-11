# NODI-03: проверка UX и автоматической публикации

Подготовка версии 0.2.0, 2026-10-11. Архитектура Go / GitHub Releases и manifest schema 1 сохранены.

Реализованы `-v`, `completion zsh|bash|powershell`, `init`, `status` и интерактивный `publish` без позиционных аргументов. Поддерживаются PATCH/MINOR/MAJOR, preview, безопасный draft по умолчанию, отдельное подтверждение released, SHA-256 сравнение и остановка без изменений. Сохранены старый синтаксис и команды.

Для конкурентной публикации добавлено атомарное создание Git ref; incomplete draft/reserved tag последней версии блокирует автоматическое выделение следующей. Теги и Releases не удаляются и не перезаписываются. `.nodi/project.json` содержит только локальную привязку и метаданные без credentials и исключается из пакетов.

Completion использует saved credentials, лёгкий GitHub index, общий timeout 1,5 секунды, кеш 60 секунд с отдельными repository/role/credential fingerprint. Ошибки и отсутствие credentials не приводят к запросу ввода. Profile-файлы не изменяются. Homebrew Formula устанавливает bash/zsh completion; winget ZIP содержит scripts для явного подключения PowerShell пользователем.

На момент этого коммита локально: 35 Go-тестов, go vet, проверка дистрибутива, winget validate и настоящая PowerShell 5.1 TabExpansion2 — PASS. Native macOS CI, интеграционный UX и публикация ещё готовятся. Эта запись не подтверждает выпуск 0.2.0.

Winget PR 0.1.0 [#450627](https://github.com/microsoft/winget-pkgs/pull/450627) открыт и не изменён. Manifest 0.2.0 хранится отдельно. До рассмотрения исходного PR и индексации официальный winget install не подтверждён; повторный new-package PR не создаётся.
