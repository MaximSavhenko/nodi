# NODI-05 / Nodi 0.3.0

## Объединение и граница приёмки

NODI-04 (`ebadffd`, код `23418fb`) входит в историю NODI-05 (`7064526`). `origin/main` также является предком: объединение выполняется fast-forward, без удаления чужих изменений. Между проверенным CI commit `4b2fe3e` и `7064526` изменён только `docs/FILE-DELIVERY.md`; исходники совпадают. Для версии 0.3.0 CI повторяется на итоговом коммите и затем на теге перед публикацией.

`setup` теперь настраивает доступ; прежний механизм NODI-04 вызывается через `setup --completion`. Установщики распознают новый флаг и продолжают поддерживать старые версии. Формат manifest schema 1, .nodi, глобальные настройки, read/write slots и DPAPI/0600 не мигрируются. Прежние пакеты читаются со строгой проверкой SHA-256 и статуса.

## Восстановление macstend3d

С разрешения пользователя создан новый `macstend3d/v0.1.1`, release ID `409837242`, draft=false, prerelease=false. ZIP SHA-256: `41f347e7d47ce14106283fed174549a8999b65acccabf3ca7c198655dc176345`. До отправки проверены все шесть файлов, после — manifest, ZIP и итоговое состояние GitHub.

Исходный `macstend3d/v0.1.0` (ID `409792032`) сохранён: flags, updated_at, ID, размеры и digest обоих assets совпадают с предварительным снимком. Он остаётся несовместимым по status вследствие прежнего ручного изменения GitHub flags; проверка не ослаблена.

## Выпуск и доказательства

После успешного [CI main](https://github.com/MaximSavhenko/nodi/actions/runs/38163197100) создан тег `v0.3.0` на `3a703f04d5993347526b8e95cf7d8884b0b61c28`. [Релизный CI](https://github.com/MaximSavhenko/nodi/actions/runs/38163666358) повторно выполнил build/native проверки и затем опубликовал [стабильный release](https://github.com/MaximSavhenko/nodi/releases/tag/v0.3.0), ID `409845792`, draft=false, prerelease=false. Проверка установки опубликованного portable пакета через Winget также PASS. Существующие releases не заменялись.

Release содержит standalone binaries для Windows x64 / Mac ARM64 / Mac Intel, ZIP, SHA256SUMS, validation.json, Homebrew Formula, versioned Winget manifests и оба терминальных установщика. Все скачанные asset sizes/digests совпали с GitHub, шесть binaries/ZIP согласованы с confirmed receipt/source SHA; проверены содержимое ZIP и запуск Windows binary. Опубликованные установщики совпали с проверенными исходниками.

Хеши ZIP: Windows `a8d021327ef7ed4ed704d9f59203ae23ab8a36ba90701d5d2c30d0009cc6b68f`; Mac ARM64 `d356de6ad10b4f4696ac73ceb2c0b83c898750ca1ddefa8a4405e7fb5be77721`; Mac Intel `82de9f750a1bc3d392d72cf5f17800bbfba47c215875f960d8a8064def67c874`.

Подготовительные ZIP из Windows PowerShell 5.1/.NET Framework отличаются от CI PowerShell 7/.NET способом упаковки NoCompression: внутри побайтно одинаковые binary/LICENSE/completion. Поэтому окончательные Formula/manifests взяты именно из проверенных опубликованных assets, а не из локальной сборки. Тег и release не переписывались; синхронизация packaging — последующий commit без изменения программы.

Локальный Windows installer обновил прежнюю установку до 0.3.0. Все 23 прежних файла global config, DPAPI credentials, профилей и загрузок сохранили SHA-256. `setup --check` прочитал прежние credentials без их перезаписи. Реальный completion команд/проектов/версий прошёл в новых процессах PowerShell 5.1.22621.4249 и 7.6.6 с автоматической загрузкой прежних профилей. CI отдельно проверил installer upgrade 0.2.1 → 0.3.0, byte preservation и декодирование обоих slots.

Homebrew tap commit `e7077665eff6c24e375a9fe88110e4592ae51f3b` содержит точную опубликованную Formula. [CI tap](https://github.com/MaximSavhenko/homebrew-nodi/actions/runs/38164303458) проверяет install, upgrade с 0.1.0 и 0.2.1, reinstall/uninstall, bash/zsh/PTY Tab и сохранение config/downloads/0600 credentials на Mac ARM64 и Intel. Формула потребляет только подтверждённый release.

[Стабильная передача](https://github.com/MaximSavhenko/nodi-packages/actions/runs/38164200135) выполняет send со standalone binary из release на Windows и get через опубликованные установщики на Windows / Mac ARM64 / Mac Intel. Проверки включают реальные saved Contents read credentials с 403 на write endpoint, latest/history, независимые SHA-256 шести файлов, исключения, неизменённый send, защиту пользовательских edits, legacy schema 1 и восстановленный macstend3d. Workflow изолирован в `work/stable-v030-check`; main приватного хранилища не менялся, пользовательские токены в CI не передавались. Итог каждого прогона доступен по ссылке CI.

Microsoft PR [450627](https://github.com/microsoft/winget-pkgs/pull/450627) открыт и не объединён. Его `license/cla` уже success; прежнее предположение об ожидаемом CLA устарело. Новый дублирующий PR не создавался, старые manifests/PR сохранены. Manifests 0.3.0 подготовлены с хешем опубликованного ZIP и прошли validate/install/PATH/ARP/list/uninstall в disposable CI. Приёмка Microsoft и доступность `winget install --source winget` не подтверждены.

Физическая пользовательская приёмка на Windows и Mac: **NOT VERIFIED**. Проверки CLI на текущем Windows и native CI не заменяют ручную приёмку на обоих устройствах пользователя. Выпуск CLI не означает окончательную готовность MVP.

## Обновление и реальная приёмка

Windows (PowerShell из этого checkout):

```powershell
.\scripts\install-windows.ps1 -Repository MaximSavhenko/nodi -Release v0.3.0 -NoCompletion
```

Без checkout — скачать установщик из того же стабильного релиза:

```powershell
$installer = Join-Path $env:TEMP 'nodi-install-0.3.0.ps1'
Invoke-WebRequest -UseBasicParsing 'https://github.com/MaximSavhenko/nodi/releases/download/v0.3.0/install-windows.ps1' -OutFile $installer
& $installer -Repository MaximSavhenko/nodi -Release v0.3.0 -NoCompletion
```

`-NoCompletion` сохраняет уже настроенные профили и не вызывает диалог настройки при обновлении. Сам установщик проверяет SHA-256 бинарника.

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

Независимая сверка всех файлов (служебные/секретные файлы в приёмочную папку не добавлять):

```powershell
Get-ChildItem 'C:\CAD\NodiAcceptance' -Recurse -File | Get-FileHash -Algorithm SHA256
```

```sh
cd "$HOME/Desktop/Nodi/nodiacceptance/0.1.0"
find . -type f -exec shasum -a 256 {} \;
```
