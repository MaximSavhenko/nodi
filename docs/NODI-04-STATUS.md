# NODI-04: постоянный Tab Completion

Дата: 2026-10-11. Релиз не публиковался; локальная сборка сохраняет номер 0.2.1.

Добавлены `nodi setup` и `nodi setup --check`. Настройка показывает блок и требует согласия для каждого профиля. Диагностика только читает файлы, не запускает пользовательские профили и явно отмечает runtime NOT VERIFIED. Профиль расширяется без переписывания существующих байтов; сохраняются BOM, UTF-16, окончания строк и настройки. Перед записью в существующий файл создаётся резервная копия. Повторная настройка идемпотентна; конфликтующий блок или symlink автоматически не исправляется.

Shell-скрипты и алгоритм подсказок сохранены. Проверка новой сессии выявила особенность PowerShell 5.1.26100: native stdin начинался с `efbbbf`, несмотря на BOM-less `$OutputEncoding`. Во внутреннем `__complete --json` добавлен приём UTF-8 BOM, с прежним ограничением 64 KiB и проверкой слов. Регрессионный тест покрывает команды, конечное пустое слово и повреждённый JSON. Manifest JSON и файловый транспорт не менялись.

Windows: `$PROFILE.CurrentUserAllHosts` отдельно для 5.1 и 7, через Windows known Documents folder. Установщик вызывает setup; `-NoCompletion` сохраняет возможность установки без этого шага. Старый exe без setup также поддерживается. macOS: экспортированный ZDOTDIR или HOME, штатный Homebrew site-functions на ARM64/Intel; standalone использует существующий completion zsh. Установщик интерактивно предлагает настройку, а без терминального stdin выполняет только check. Формула Homebrew и текущие опубликованные артефакты не менялись.

| Проверка | Результат |
| --- | --- |
| Go tests и go vet | PASS, локально Windows и все три native CI runner; macOS race checks — PASS |
| `--check`, отказ и EOF без записи, UTF-8 BOM / UTF-16 LE/BE, backup, повторный setup, ручной host-profile, конфликт блока, изменение файла во время подтверждения | PASS, Go-тесты |
| Новый PowerShell 5.1.22621.4249 с обычной загрузкой профилей, без `-NoProfile` и ручной активации | PASS: TabExpansion2, команды/проекты/версии |
| Новый PowerShell 7.6.6 с обычной загрузкой профилей | PASS: TabExpansion2, команды/проекты/версии |
| Новый PowerShell 5.1.26100.33438 и 7.6.6 на Windows CI, с существующей пользовательской настройкой в профиле | PASS: автоматическая загрузка, TabExpansion2, команды/проекты/версии, сохранение настройки |
| `nodi get nod` → `nodi-ux-1` | PASS, изолированный свежий completion cache, не live API |
| Повторный Windows-установщик, setup и check | PASS: SHA-256 обоих профилей неизменен, по одному блоку |
| Новый Windows-установщик со старым exe 0.2.1 без setup | PASS: установка в отдельный каталог, корректная инструкция, без изменений профилей |
| Поддержка обоих Homebrew prefix и стабильного пути при обновлении Cellar | PASS, Go-тесты генерации блока |
| Новый интерактивный/login Zsh и настоящий Tab, macOS ARM64/Intel | PASS: standalone installer, команды/проекты/версии, согласие/отказ, check, повторный setup/замена бинарника, сохранение `.zshrc` |
| Реальная локальная Homebrew Formula, ARM64/Intel | PASS: install, brew test, новый Zsh и Tab, повторная установка, штатный `_nodi`, существующий shellenv/compinit без правки профиля, uninstall |

[Полный CI-прогон](https://github.com/MaximSavhenko/nodi/actions/runs/38158786376): **SUCCESS**, код `23418fb35ede5ab3cfe8f732f6e90d2d0a14a952`. Build, Windows, macOS ARM64 и Intel прошли; publish и winget-install пропущены, поскольку запуск выполнен из отдельной ветки, без тега. Финальная запись отчёта добавлена отдельным коммитом, без изменения проверенного кода. Ветка `nodi-04-validation` не слита в main.

На этом компьютере с согласия пользователя обновлён `%LOCALAPPDATA%/Nodi/bin/nodi.exe` и созданы оба профиля. Старый exe сохранён в `.tools/nodi-04/installed-before.exe`. PowerShell 7 использован из проверенного по SHA-256 официального portable ZIP в `.tools/pwsh-validation`, без системной установки.

Для повторной проверки уже настроенного Windows-хоста: `scripts/test-persistent-completion.ps1 -Binary PATH_TO_NODI -Shell PATH_TO_POWERSHELL`. Скрипт создаёт изолированные credentials/cache, затем запускает новый процесс **без** `-NoProfile`, dot-sourcing и ручного completion. `-ConfigureProfiles` разрешает подготовку/восстановление реальных профилей только для disposable CI runners. macOS: `scripts/test-persistent-zsh.py` запускает новый интерактивный login Zsh с изолированными HOME/ZDOTDIR и нажимает Tab через PTY; в тестовой сессии нет команд compinit/source. В CI отдельно используется временный локальный tap с ZIP текущей сборки, без публикации.

Ограничения: произвольный shell framework статически доказать нельзя. Синтаксические ошибки, изменение completion после Nodi-блока или ExecutionPolicy могут мешать запуску; политика не меняется. Неэкспортированный ZDOTDIR, заданный в `.zshenv`, требует запуска setup из Zsh с эффективным ZDOTDIR. Старые ANSI-профили требуют ручного перевода в UTF-8/UTF-16. Bash setup не добавлен. Winget portable не имеет post-install hook: нужен однократный `nodi setup`. Пользовательские физические Mac и ручная проверка GUI Windows Terminal не проверялись: Windows проверен через новый реальный процесс и TabExpansion2, Mac — через новый интерактивный Zsh и настоящий Tab в PTY на native CI. Проекты/версии проверялись с тестовым кешем, без live private API. Storage, auth, версионность и manifest schema 1 не изменены.
