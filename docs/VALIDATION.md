# Проверка NODI-01

Дата: 2026-10-11. Среда: Windows amd64, Go 1.27.2, локальный проект `C:\projects\nodi`.

| Проверка | Результат | Доказательство / граница |
|---|---|---|
| Автоматические тесты | PASS | 23 функции тестов и их подслучаи, `go test -v ./...`; без реального GitHub |
| Статический анализ | PASS | `go vet ./...` |
| Windows amd64 | PASS | Самостоятельный `dist/nodi-windows-amd64.exe`, запуск `version` и `help` |
| macOS arm64 / amd64 | PASS для сборки | Обе программы собраны с `CGO_ENABLED=0`; платформы подтверждены `go version -m` |
| Windows-установка без Go в PATH | PASS | PowerShell-установщик из локального `dist`, отдельная папка `.tools/install-smoke`, `nodi.exe version` |
| Неверная SHA-256 установщика | PASS | Испорченный `SHA256SUMS` отклонён до установки программы |
| Повторяемость сборки | PASS для Windows | Повторная сборка с теми же флагами дала идентичный SHA-256 |
| HTTPS mock end-to-end | PASS | `TestMockEndToEnd`: публикация, список, информация, конкретная версия, SHA-256, учёт пути, вызов opener, отказ при повторной загрузке |
| Защита пакета | PASS | Zip Slip, абсолютные пути, ссылки, NTFS-пути, дубли, коллизии, размеры, SHA-256 и отмена |
| GitHub ошибки | PASS на mock | 401/403/404/422/429/500, недоступная сеть, публичное хранилище, незавершённый upload/finalization, read-токен не публикует |
| Токены и redirects | PASS на mock / локально | DPAPI round-trip Windows, отдельные read/write слоты, logout; HTTPS redirects не получают Authorization, HTTP downgrade отклоняется |
| Синтаксис установщиков | PASS | PowerShell Parser и `bash -n scripts/install-macos.sh` |
| Реальный GitHub private E2E | NOT VERIFIED | Конкретный storage-репозиторий и токены пользователем не предоставлены; ничего в GitHub не опубликовано |
| Finder / macOS-установка / Gatekeeper | NOT VERIFIED | Mac в этой проверке не использовался; macOS-программы только собраны |
| Реальное окно Explorer | NOT VERIFIED | В end-to-end opener заменён callback; Windows Desktop API отдельно проверен |
| Интерактивный скрытый ввод | NOT VERIFIED вручную | Протестирован pipe-input; терминальный режим реализован платформенным кодом |
| Race detector и GitHub Actions | NOT VERIFIED здесь | Workflow подготовлен; локально нет C-компилятора для `-race`, workflow не запущен на GitHub |

Сборки и `SHA256SUMS` лежат в `dist/`, локальный компилятор — в `.tools/go/`. Они исключены из исходного Git-репозитория. Все исходники, установщики, тесты, workflow и фиктивные DXF/PDF находятся в проекте. Команды установки приведены в README.

Для реальной проверки после настройки приватного тестового репозитория:

```text
nodi config --repo OWNER/PRIVATE-TEST-REPO
nodi auth login
nodi auth login --write
nodi publish demo ./examples/manufacturing --version 1.0.0 --status released --metadata ./examples/metadata.json
nodi list
nodi info demo
nodi get demo --version 1.0.0
nodi open demo
nodi get demo --version 1.0.0
```

Последний `get` должен завершиться отказом, сохранив существующие файлы. На другом компьютере используйте только read-токен и повторите `list/info/get/open`. Фиктивные файлы не предназначены для производства. Реальные производственные данные не загружались.
