# NODI-04: постоянный Tab Completion

Дата: 2026-10-11. Релиз не публиковался; локальная сборка сохраняет номер 0.2.1.

Добавлены `nodi setup` и `nodi setup --check`. Настройка показывает блок и требует согласия для каждого профиля. Диагностика только читает файлы, не запускает пользовательские профили и явно отмечает runtime NOT VERIFIED. Профиль расширяется без переписывания существующих байтов; сохраняются BOM, UTF-16, окончания строк и настройки. Перед записью в существующий файл создаётся резервная копия. Повторная настройка идемпотентна; конфликтующий блок или symlink автоматически не исправляется.

Windows: `$PROFILE.CurrentUserAllHosts` отдельно для 5.1 и 7, через Windows known Documents folder. Установщик вызывает setup; `-NoCompletion` сохраняет возможность установки без этого шага. Старый exe без setup также поддерживается. macOS: экспортированный ZDOTDIR или HOME, штатный Homebrew site-functions на ARM64/Intel; standalone использует существующий completion zsh. Установщик интерактивно предлагает настройку, а без терминального stdin выполняет только check. Формула Homebrew и текущие опубликованные артефакты не менялись.

| Проверка | Результат |
| --- | --- |
| Go tests и go vet | PASS, локально Windows |
| `--check`, отказ и EOF без записи, UTF-8 BOM / UTF-16 LE/BE, backup, повторный setup, ручной host-profile, конфликт блока, изменение файла во время подтверждения | PASS, Go-тесты |
| Новый PowerShell 5.1.22621.4249 с обычной загрузкой профилей, без `-NoProfile` и ручной активации | PASS: TabExpansion2, команды/проекты/версии |
| Новый PowerShell 7.6.6 с обычной загрузкой профилей | PASS: TabExpansion2, команды/проекты/версии |
| `nodi get nod` → `nodi-ux-1` | PASS, изолированный свежий completion cache, не live API |
| Повторный Windows-установщик, setup и check | PASS: SHA-256 обоих профилей неизменен, по одному блоку |
| Поддержка обоих Homebrew prefix и стабильного пути при обновлении Cellar | PASS, Go-тесты генерации блока |
| Новый интерактивный/login Zsh и настоящий Tab, macOS ARM64/Intel, реальная локальная Homebrew Formula | Подготовлены native CI-проверки; локально на Windows не выполнялись |

На этом компьютере с согласия пользователя обновлён `%LOCALAPPDATA%/Nodi/bin/nodi.exe` и созданы оба профиля. Старый exe сохранён в `.tools/nodi-04/installed-before.exe`. PowerShell 7 использован из проверенного по SHA-256 официального portable ZIP в `.tools/pwsh-validation`, без системной установки.

Для повторной проверки уже настроенного Windows-хоста: `scripts/test-persistent-completion.ps1 -Binary PATH_TO_NODI -Shell PATH_TO_POWERSHELL`. Скрипт создаёт изолированные credentials/cache, затем запускает новый процесс **без** `-NoProfile`, dot-sourcing и ручного completion. `-ConfigureProfiles` разрешает подготовку/восстановление реальных профилей только для disposable CI runners. macOS: `scripts/test-persistent-zsh.py` запускает новый интерактивный login Zsh с изолированными HOME/ZDOTDIR и нажимает Tab через PTY; в тестовой сессии нет команд compinit/source. В CI отдельно используется временный локальный tap с ZIP текущей сборки, без публикации.

Ограничения: произвольный shell framework статически доказать нельзя. Синтаксические ошибки, изменение completion после Nodi-блока или ExecutionPolicy могут мешать запуску; политика не меняется. Неэкспортированный ZDOTDIR, заданный в `.zshenv`, требует запуска setup из Zsh с эффективным ZDOTDIR. Старые ANSI-профили требуют ручного перевода в UTF-8/UTF-16. Bash setup не добавлен. Winget portable не имеет post-install hook: нужен однократный `nodi setup`. Реальные Mac и macOS CI в текущем локальном прогоне пока NOT VERIFIED. Storage, auth, версионность и manifest schema 1 не изменены.
