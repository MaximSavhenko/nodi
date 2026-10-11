# NODI-06: полировка MVP и безопасное управление storage

Дата проверки: 2026-10-11. Изменения подготовлены для следующей версии; новый стабильный релиз и обновление пользовательской установки не выполнялись.

Реализованы независимые вложенные каталоги, `send --name/--dry-run`, глобальные записи идентичности непривязанных source, защита чужих имён, `delete --version/--all/--dry-run`, постоянная история номеров, typed confirmation, проверка фактического write-доступа, порядок Release → tag с API-проверками и сообщения частичных сбоев. Completion содержит новые команды/флаги и удаляет локальный cache после удаления. `update --check` читает только стабильный CLI release и не устанавливает его. Manifest schema 1 сохранена.

| Проверка | Результат | Граница доказательства |
|---|---|---|
| `go vet ./...` | PASS | Windows amd64, Go 1.27.2 |
| `go test -cover ./...` | PASS | CLI 74.1%, storage 77.3%; весь прежний suite и новые тесты |
| Независимый nested send / конфликт / partial file | PASS | TLS fixture и настоящий GitHub API на новом test project |
| Dry-run без изменений | PASS | Source/config byte snapshots и только GET на TLS fixture; реальные кандидаты проверены dry-run |
| SHA-256 / get / старый schema 1 | PASS | Прежние и новые integration tests; live fixture с независимой сверкой download |
| Одно / все / повторное удаление | PASS | TLS fixture; одно/all дополнительно real GitHub API |
| Read-only credential | PASS на fixture | API 403; отдельно настоящий read-only token для live delete не создавался |
| Частичные сбои и изменившийся снимок | PASS на fixture | history POST, Release DELETE, tag DELETE, verification GET, частичный all, замена tag, неизвестные assets, повреждённый ZIP и status mismatch |
| Монотонность после удаления | PASS | Реальный API: 0.1.0 → 0.1.1 → delete latest → 0.1.2 → delete all → 0.1.3 |
| Локальные файлы и parent binding | PASS | Реальный test source, JSON и download SHA-256 сохранились |
| Настоящий PowerShell completion | PASS | Windows PowerShell: старые и новые команды/флаги/версии |
| macOS ARM64 / Intel native CI | PASS | [CI 38170131775](https://github.com/MaximSavhenko/nodi/actions/runs/38170131775), commit 4802495; оба native runner и completion |
| Физическая приёмка пользователя | NOT VERIFIED | Требуется новый binary на Windows и Mac после разрешённого выпуска |

Реальный destructive-тест использовал только специально созданный `nodi06-delete-test-*`: четыре созданные версии удалены, их четыре version-history refs оставлены намеренно. Локальная папка fixture сохранена как evidence. Скрипт [test-storage-management.ps1](../scripts/test-storage-management.ps1) принимает только путь binary и фиксированный storage repo; credentials передаются через env, не аргументы. Он не выполняется автоматически в обычном CI.

Инвентаризация исходных package Releases: 20 releases и 20 tags. Подтверждено 16 тестовых кандидатов (8 проектов), 20 091 bytes ZIP / 37 348 bytes всех assets. Четыре CAD release сохраняются, включая уже существовавшую третью версию одного CAD-проекта и проблемную старую версию. Все восемь dry-run прошли проверку manifest/ZIP SHA/tag. Удаление существующих кандидатов **не выполнялось**. После изолированного теста исходные release IDs, updated_at, asset IDs и tag SHA совпали; в storage остались исходные 20 releases и 24 refs с четырьмя новыми history refs.

Детальная таблица приватного storage, точные release IDs и approval selection сохранены локально в `.tools/nodi06/cleanup-plan.md` и `inventory.csv`; приватный inventory не включается в публичный PR. Очистка требует отдельного подтверждения точных 16 release IDs. Нельзя расширять этот список по совпадению имени; при изменении снимка требуется новый dry-run.

Рекомендация: **0.4.0 после native CI и отдельного разрешения на выпуск**. Перед очисткой нужно обновить всех writer-клиентов: 0.3.0 читает прежние schema-1 packages, но не учитывает новые deletion-history refs. Установщики, стабильный tag, Homebrew/winget manifests и опубликованные CLI assets сейчас не меняются. Архитектурное заключение — [средняя сложность замены provider](ARCHITECTURE.md).
