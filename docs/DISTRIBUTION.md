# Выпуск Nodi

Исходники: `MaximSavhenko/nodi`, лицензия MIT. Пакеты CAD: отдельный private `MaximSavhenko/nodi-packages`. Homebrew: `MaximSavhenko/homebrew-nodi`. Репозитории используют собственные GITHUB_TOKEN с минимальными правами; PAT для связи между ними не нужен.

## Подтверждённый бинарный релиз

1. Изменения проверяются и попадают в `main`. Локально: `go test ./...`, `go vet ./...`.
2. Выпуск — стабильный Git-тег, например `git tag -a v0.1.1 -m "Confirmed Nodi 0.1.1"`, затем `git push origin v0.1.1`. Не создавайте тег для неподтверждённого кода. Workflow проверяет формат версии и принадлежность коммита истории main.
3. CI создаёт три самостоятельных бинарника и ZIP-пакеты с LICENSE. Генерируются SHA256SUMS, Homebrew Formula и трёхфайловый winget manifest. `winget validate` обязан пройти.
4. Каждый точный собранный бинарник запускается на соответствующем native runner: Windows amd64, macOS ARM64 и Intel. Проверяются `version` и `help` с PATH без среды разработки. Go-тесты выполняются на всех платформах, race detector — на macOS.
5. Только после успеха всех native jobs публикуется `validation.json` со статусом confirmed, SHA-256, исходным коммитом и ссылкой на CI. Создаётся GitHub draft с вложениями; после загрузок он становится стабильным релизом. Существующий тег/релиз нельзя перезаписать workflow.
6. Отдельный Windows job проверяет опубликованный winget manifest и настоящий portable install / version detection / uninstall на временном runner. Он **не доказывает** доступность в публичном источнике winget. Настройка LocalManifestFiles включается только на одноразовом CI runner, а не на компьютере пользователя.

Для локальной подготовки: `scripts/build.ps1`, `scripts/generate-distribution.ps1`, `scripts/verify-distribution.ps1`. Установщикам и пользователям Go не требуется. SHA-256 ZIP является хэшем именно ZIP-контейнера, а не вложенного exe.

## Homebrew

Tap workflow опрашивает стабильный релиз каждый час и может запускаться вручную. Он принимает только `validation.json` с confirmed и четырьмя обязательными проверками, согласованные SHA256SUMS и Formula. Формула скачивается из релиза, поэтому в ней находятся хэши опубликованных ZIP, а не приблизительные локальные значения. Downgrade отклоняется. Автоматический commit запускает установку/test/uninstall на ARM64 и Intel внутри того же workflow; это необходимо, потому что push через GITHUB_TOKEN не запускает новый push-workflow.

При обновлении версии пользователь выполняет `brew update`, затем `brew upgrade MaximSavhenko/nodi/nodi`. Formula не компилирует Go.

## winget

Пакет: `MaximSavhenko.Nodi`, `InstallerType: zip`, `NestedInstallerType: portable`, вложенный `nodi.exe`, alias `nodi`, SHA-256 внешнего ZIP. Используется совместимая схема 1.10.0 с поддержкой portable ZIP. WinGet управляет PATH через свои portable links и ведёт запись установленной версии. Проекты и credentials не находятся в установочной папке и не удаляются вместе с CLI.

В стабильном релизе лежит `winget-manifests.zip`; его содержимое отправляется в `microsoft/winget-pkgs` под `manifests/m/MaximSavhenko/Nodi/VERSION`. Перед PR нужно проверить отсутствие дубликата, актуальный SHA, публичный URL, `winget validate` и установку. PR ограничивается одной версией одного пакета. Публикация в каталоге — отдельное внешнее решение Microsoft; даже успешный install из локального manifest не позволяет заявить, что `winget install --source winget` уже работает.

Официальные требования: [создание manifest](https://learn.microsoft.com/en-us/windows/package-manager/package/manifest), [отправка PR и рассмотрение](https://learn.microsoft.com/en-us/windows/package-manager/package/repository), [portable приложения](https://github.com/microsoft/winget-cli/blob/master/doc/specs/%23182%20-%20Support%20for%20installation%20of%20portable%20standalone%20apps.md), [Homebrew Formula](https://docs.brew.sh/Formula-Cookbook).
