# NODI-03: CLI UX и автоматическая публикация

Дата: 2026-10-11. MIT. Архитектура Go / приватный GitHub Releases и manifest schema 1 сохранены.

## Реализовано

- `-v`, `--version`, `version` показывают версию установленного бинарника и платформу.
- `completion [zsh|bash|powershell]`: команды, подкоманды, флаги, статусы, проекты, версии и пути. Общий timeout GitHub API 1,5 секунды, кеш 60 секунд; offline fallback может быть устаревшим. Кеш разделён по repository, роли и fingerprint credentials, без токенов. Tab не запрашивает авторизацию; ошибки подавляются.
- `init PROJECT [--repo OWNER/PRIVATE-REPO]`: schema 1 `.nodi/project.json` без credentials, сохранение настроек, поиск ближайшего проекта из вложенных папок. `.nodi` исключена из пакетов.
- `status`: SHA-256 Added/Modified/Deleted относительно актуального удалённого состояния, без записи файлов. Ошибки сети/credentials — NOT VERIFIED. Read credentials проверяют только released; drafts отмечаются отдельно.
- `publish`: текущий проект, preview, y/N, default draft. Первая версия 0.1.0, затем PATCH; --minor/--major учитывают фактические GitHub версии, drafts и reserved tags. Released требует набрать `release PROJECT VERSION`: подтверждение публикации автором, не инженерная проверка.
- Без изменений новый Release не создаётся. Используется SHA-256, а не mtime. Сохранены ограничения форматов, размера и путей.
- После preview повторно проверяются файлы, привязка и удалённое состояние. Атомарная резервация Git ref предотвращает перезапись при конкурентной публикации. Неполный draft/reserved tag блокирует автоматическое выделение следующей версии; нет удаления, force update или автоматического retry.
- Сохранены get/list/info/open/auth/config, старый явный publish PROJECT PATH --version X.Y.Z --status released и существующие пакеты. Старый явный синтаксис остаётся без интерактивного диалога.

Completion для publish использует локальную привязку; get/info/open используют глобальный nodi config, как сами команды. Исправление поведения с разными repositories включено в 0.2.1. Shell profiles не меняются автоматически. Homebrew устанавливает completion definitions; PowerShell подключается явно. macOS installer записывает PATH в .zprofile только с --add-path.

## Репозитории и релизы

- [Nodi](https://github.com/MaximSavhenko/nodi), public.
- [Homebrew tap](https://github.com/MaximSavhenko/homebrew-nodi), public.
- [Хранилище пакетов](https://github.com/MaximSavhenko/nodi-packages), private, требует доступа.
- [v0.2.0](https://github.com/MaximSavhenko/nodi/releases/tag/v0.2.0) опубликован после [успешных проверок](https://github.com/MaximSavhenko/nodi/actions/runs/38152478298). Тег и данные сохранены.
- [v0.2.1](https://github.com/MaximSavhenko/nodi/releases/tag/v0.2.1) — актуальный подтверждённый стабильный выпуск. Source commit `a37cf99e3a9799cd223181a912d3ce6a04d008da`; SHA-256 и CI evidence находятся в опубликованном validation.json. Все шесть binaries/ZIP скачаны и сверены по receipt, размеру и GitHub digest. Windows binary дополнительно запущен локально.

Дистрибутив: три standalone binaries, три ZIP с MIT LICENSE и completion scripts, SHA256SUMS, Formula, winget manifests и validation.json. Стабильный Release публикуется только после native Go/runtime/completion проверок всех трёх платформ и winget validate.

## Проверки

Локально Windows: 36 Go-тестов, go vet, ZIP/SHA-256/runtime с пустым PATH, winget validate и настоящая PowerShell 5.1 TabExpansion2 — PASS. Mock API покрывает offline/timeout/cache, роли и credentials, отсутствие изменений, изменение remote/local после preview, одновременную публикацию, orphan tag и interrupted upload.

| Проверка | Windows amd64 | macOS ARM64 | macOS Intel |
| --- | --- | --- | --- |
| 36 Go-тестов, go vet | PASS | PASS | PASS |
| Race detector | Не выполнялся | PASS | PASS |
| Точный release binary: version/-v/--version/help, пустой PATH | PASS | PASS | PASS |
| Native PowerShell completer: commands/flags/projects/versions/paths | PASS, локально 5.1 и CI | PASS в CI | PASS в CI |
| Native bash и zsh: Tab widget, несколько совпадений, paths с пробелами | — | PASS | PASS |
| Installer: profile сохранён по умолчанию, --add-path явный и идемпотентный | — | PASS | PASS |
| Homebrew: 0.1.0 → 0.2.1, SHA-256, completion files, brew test, uninstall | — | PASS | PASS |
| Winget: published local manifest validate/install/PATH/ARP detection/uninstall | PASS | — | — |
| Приватный GitHub: saved read credential, completion без drafts, list/get/SHA-256/open | PASS | PASS | PASS |

Доказательства:

- [CI до выпуска](https://github.com/MaximSavhenko/nodi/actions/runs/38153256700) — PASS всех native jobs.
- [Release CI 0.2.1](https://github.com/MaximSavhenko/nodi/actions/runs/38153622752) — PASS build, три native jobs, publish и winget-install. Релиз сначала создан как draft, затем опубликован после проверок.
- [Homebrew updater и реальное обновление обеих архитектур](https://github.com/MaximSavhenko/homebrew-nodi/actions/runs/38154019207) — PASS. Formula bot commit `b981726c0f7a5a5c44b6815c65b41b8301e3371c`; потреблены только подтверждённые assets. Проверены upgrade с существующей 0.1.0, upgrade текущей версии, uninstall и отсутствие nodi после удаления.
- [Mac Homebrew author → Windows/ARM64/Intel recipients](https://github.com/MaximSavhenko/nodi-packages/actions/runs/38154080235) — PASS. Автор с Contents write, отдельные получатели с Contents read. Сохранённые защищённые credentials используются после удаления временного token environment. Bash completion установленной Formula работает с реальным проектом и API-кешем. Все recipients проверили выпущенную 0.1.1 и отсутствие draft 0.1.0 в read-only completion. Никакой Go build на стороне автора/получателей не выполнялся.

CLI native shell tests используют фиктивный cache/mock index; реальный GitHub index/saved credentials дополнительно проверены в приватном workflow. Zsh Tab проверен через PTY/ZLE на CI, без изменения пользовательских профилей. Это автоматическая проверка реальной оболочки, не приёмка на физическом пользовательском Mac.

## Реальные фиктивные пакеты

Локальный интеграционный прогон создал default draft nodi-ux-local/v0.1.0; повторный publish остановился без загрузки. Изменение одного DXF обнаружено по SHA-256; следующая явно подтверждённая публикация — [nodi-ux-local/v0.1.1](https://github.com/MaximSavhenko/nodi-packages/releases/tag/nodi-ux-local/v0.1.1). Get выбранной версии проверил четыре файла; live API completion показал проект и обе версии. Использованы только examples/manufacturing с NOT FOR MANUFACTURING.

Итоговый Homebrew author создал отдельный проект `nodi-ux-1`: default draft 0.1.0, повторный publish без изменений, Modified: 1 из вложенной папки, явное released-подтверждение следующей [0.1.1](https://github.com/MaximSavhenko/nodi-packages/releases/tag/nodi-ux-1/v0.1.1). Третья 0.1.2 не создана. Release содержит только nodi-manifest.json и package.zip (1534 байта, SHA-256 `8980b4e52f9566d683d1c524be3171c3d65181bcf1d41d4c8f7ead9e685328d7`), manifest — четыре файла. Draft 0.1.0 сохранён; данные старых проектов не заменялись.

На пользовательском Windows терминальный установщик обновил `%LOCALAPPDATA%\Nodi\bin\nodi.exe` с 0.1.0 до опубликованной 0.2.1 с проверкой SHA-256. Config.json не изменился при установке, старые скачанные пакеты сохранены. Проверены фактический пользовательский PATH после обновления окружения и PowerShell 5.1 TabExpansion2. Новый пакет находится в `Desktop\Nodi\nodi-ux-1\0.1.1`; `nodi open` открыл Проводник, целевой путь подтверждён через Shell.Application. Для этого прогона GitHub credential использовался только временно в памяти процесса; в пользовательское auth-хранилище не сохранялся.

## Winget и ограничения

[Официальный PR 0.1.0 #450627](https://github.com/microsoft/winget-pkgs/pull/450627) открыт и не изменён, head 358f31e574ce011bda0d28f5c6d764bf2fef699a. Microsoft [ожидает CLA владельца](https://github.com/microsoft/winget-pkgs/pull/450627#issuecomment-6109606006); соглашение от его имени не принималось. Новые manifests лежат отдельно. До разрешения первого new-package PR второй не создаётся; после merge/index требуется отдельное обновление версии каталога.

Manifests 0.2.0 и 0.2.1 в отдельных каталогах packaging/winget, контрольные суммы приведены к фактическим опубликованным ZIP. Обе версии проходят локальный winget validate без предупреждений; 0.1.0 manifests и официальный PR не изменены. [Готовый пакет manifests 0.2.1](https://github.com/MaximSavhenko/nodi/releases/download/v0.2.1/winget-manifests.zip). Обновление между версиями через официальный winget не подтверждено до принятия и индексации каталога.

`winget search --id MaximSavhenko.Nodi --exact --source winget`: No package found. Установка/обновление из официального источника **NOT VERIFIED / PENDING**, даже если local-manifest CI прошёл. Пока используйте терминальный установщик из README.

Физические пользовательские Mac, визуальный Finder, macOS подпись/notarization и браузерный Gatekeeper quarantine не проверялись. Windows race detector не запускался. Пустой runtime PATH подтверждает запуск без Go/Git/Node.js, но не заменяет чистую физическую машину. Homebrew требует существующей установки и настройки shell framework. Для последующих сетевых операций нужны собственные ограниченные read/write credentials.

## Обновление

macOS: `brew update && brew upgrade MaximSavhenko/nodi/nodi`.

Windows: из C:\projects\nodi выполните `.\scripts\install-windows.ps1 -Repository MaximSavhenko/nodi -Release v0.2.1`. На этом компьютере обновление уже выполнено. Проекты и конфигурация сохраняются. Для текущего открытого терминала, который унаследовал старый PATH, откройте новый терминал.

```powershell
nodi -v
nodi completion powershell | Out-String | Invoke-Expression
cd PATH_TO_PROJECT
nodi init PROJECT
nodi status
nodi publish
```

zsh: `autoload -Uz compinit; compinit; source <(nodi completion zsh)`.
bash: `source <(nodi completion bash)`.
Постоянное подключение пользователь добавляет в профиль самостоятельно после просмотра скрипта.
