# NODI-05 / Nodi 0.3.0

## Объединение и граница приёмки

NODI-04 (`ebadffd`, код `23418fb`) входит в историю NODI-05 (`7064526`). `origin/main` также является предком: объединение выполняется fast-forward, без удаления чужих изменений. Между проверенным CI commit `4b2fe3e` и `7064526` изменён только `docs/FILE-DELIVERY.md`; исходники совпадают. Для версии 0.3.0 CI повторяется на итоговом коммите и затем на теге перед публикацией.

`setup` теперь настраивает доступ; прежний механизм NODI-04 вызывается через `setup --completion`. Установщики распознают новый флаг и продолжают поддерживать старые версии. Формат manifest schema 1, .nodi, глобальные настройки, read/write slots и DPAPI/0600 не мигрируются. Прежние пакеты читаются со строгой проверкой SHA-256 и статуса.

## Восстановление macstend3d

С разрешения пользователя создан новый `macstend3d/v0.1.1`, release ID `409837242`, draft=false, prerelease=false. ZIP SHA-256: `41f347e7d47ce14106283fed174549a8999b65acccabf3ca7c198655dc176345`. До отправки проверены все шесть файлов, после — manifest, ZIP и итоговое состояние GitHub.

Исходный `macstend3d/v0.1.0` (ID `409792032`) сохранён: flags, updated_at, ID, размеры и digest обоих assets совпадают с предварительным снимком. Он остаётся несовместимым по status вследствие прежнего ручного изменения GitHub flags; проверка не ослаблена.

## План выпуска

Только после успешного native CI публикуется стабильный тег v0.3.0. Release содержит standalone binaries для Windows x64 / Mac ARM64 / Mac Intel, ZIP, SHA256SUMS, validation.json, Homebrew Formula, versioned Winget manifests и терминальные установщики. Homebrew потребляет только подтверждённый release. Microsoft PR [450627](https://github.com/microsoft/winget-pkgs/pull/450627) открыт и не объединён; новый дублирующий PR не создаётся, старые manifests сохраняются.

Автоматические результаты и окончательные SHA/CI ссылки добавляются после выполнения. Физическая приёмка на Windows и Mac пользователя: **NOT VERIFIED**. Выпуск CLI не означает окончательную готовность MVP.

## Обновление и реальная приёмка

Windows (PowerShell из этого checkout):

```powershell
.\scripts\install-windows.ps1 -Repository MaximSavhenko/nodi -Release v0.3.0 -NoCompletion
```

macOS:

```sh
brew update
brew upgrade MaximSavhenko/nodi/nodi
```

Открыть новый терминал на каждом устройстве: `nodi -v`, `nodi setup --check`, `nodi setup --completion --check`. При отсутствующем completion выполнить `nodi setup --completion` один раз с согласием на свой профиль, затем снова открыть терминал. Сохранённые credentials не вводить повторно.

1. Windows: создать отдельную папку `C:\CAD\NodiAcceptance` с обычными STEP/DXF/PDF и TXT файлами; `nodi send "C:\CAD\NodiAcceptance"`. Записать показанные project/version. Проверить Tab для `nodi se`, `nodi get nod` и `--version`.
2. Mac с отдельным Contents read credential: `nodi get nodiacceptance`. Сверить каждый файл с Windows через `Get-FileHash -Algorithm SHA256` и `shasum -a 256`. Не нужны браузер и подтверждение публикации.
3. Изменить TXT на Windows, повторить send, затем на Mac get: следующая версия в отдельной папке. Повторный неизменённый send сообщает Nothing to publish.
4. Mac: `nodi get nodiacceptance --version 0.1.0 --output "$HOME/Desktop/NodiAcceptance-history"`; повторно сверить исходные SHA. `nodi get macstend3d --version 0.1.1` должен выдать шесть STEP файлов.

Использовать новые имена выходных папок: существующие destination намеренно не перезаписываются.
