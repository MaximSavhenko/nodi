# NODI-02: результаты распространения

Дата: 2026-10-11. Владелец: MaximSavhenko. Лицензия: MIT, выбрана владельцем.

**Homebrew и приватный GitHub-транспорт подтверждены. Winget-пакет проверен, PR отправлен, но установка из официального источника ещё не подтверждена. Полный критерий готовности NODI-02 пока не достигнут.**

## Репозитории и релизы

- [nodi](https://github.com/MaximSavhenko/nodi) — public, исходники, документация, тесты, CI/CD и бинарники.
- [homebrew-nodi](https://github.com/MaximSavhenko/homebrew-nodi) — public, Formula и автоматическое обновление.
- [nodi-packages](https://github.com/MaximSavhenko/nodi-packages) — private, инженерные пакеты через Releases.
- [Nodi v0.1.0](https://github.com/MaximSavhenko/nodi/releases/tag/v0.1.0) — стабильный CLI для Windows amd64, macOS ARM64/Intel, ZIP, SHA256SUMS, MIT LICENSE, Formula, winget manifests и validation.json.
- Фиктивные пакеты: [nodi-demo/v0.1.1](https://github.com/MaximSavhenko/nodi-packages/releases/tag/nodi-demo/v0.1.1), [nodi-demo/v0.1.2](https://github.com/MaximSavhenko/nodi-packages/releases/tag/nodi-demo/v0.1.2). Требуется доступ к приватному репозиторию.
- [PR microsoft/winget-pkgs#450627](https://github.com/microsoft/winget-pkgs/pull/450627). В [fork](https://github.com/MaximSavhenko/winget-pkgs/tree/nodi-0.1.0) добавлены только три YAML-файла новой версии.

До создания проверены 28 принадлежащих пользователю репозиториев: дубликатов нужных имён не было. Подтверждены admin/push и требуемая видимость. Сторонние проекты не менялись. Анонимный запрос к приватному хранилищу возвращает HTTP 404. Токены не включены в исходники, логи и релизы.

## Проверки по платформам

| Проверка | Windows amd64 | macOS ARM64 | macOS Intel |
| --- | --- | --- | --- |
| 24 Go-теста, go vet | PASS | PASS | PASS |
| Race detector | Не выполнялся | PASS | PASS |
| Точный release binary, version/help, PATH без среды разработки | PASS | PASS | PASS |
| Homebrew: install, SHA-256, version/help, brew test, uninstall | — | PASS | PASS |
| Winget: validate, local manifest install, SHA-256, PATH, версия, uninstall | PASS | — | — |
| Приватный GitHub: list/info/get, SHA-256, opener, отказ повторного get | PASS | PASS | PASS |

Доказательства:

- [Проверки до публикации 0.1.0](https://github.com/MaximSavhenko/nodi/actions/runs/38143183619): build, три native jobs и publish прошли. Первоначальный post-publication winget smoke завершился ошибкой поиска local-manifest установки по source ID; установка, PATH и запуск уже прошли. Отдельный исправленный smoke подтвердил полный цикл.
- [CI main после исправления smoke](https://github.com/MaximSavhenko/nodi/actions/runs/38143705379) — PASS на Windows и обеих архитектурах macOS.
- [Winget smoke опубликованного пакета](https://github.com/MaximSavhenko/nodi/actions/runs/38143734213) — PASS. ARP: Nodi, версия 0.1.0, Publisher MaximSavhenko, WinGetPackageIdentifier MaximSavhenko.Nodi. До индексации официальный ID не коррелируется с local manifest: `winget list --name Nodi --exact` показывает ARP ID и версию. Удаление по manifest и отсутствие команды подтверждены. LocalManifestFiles включался только на одноразовом CI runner.
- [Homebrew ARM64 и Intel](https://github.com/MaximSavhenko/homebrew-nodi/actions/runs/38143732210) — PASS. Formula автоматически добавлена bot-коммитом из опубликованного validation.json, SHA256SUMS и nodi.rb. `brew upgrade` установленной 0.1.0 корректно сообщает об отсутствии новой версии. Автоматический updater принимает только подтверждённые стабильные релизы.
- [Передача с разными правами, attempt 2](https://github.com/MaximSavhenko/nodi-packages/actions/runs/38143735824/attempts/2) — PASS. Автор: Contents write; три получателя: отдельные Contents read токены. Повтор понадобился после временной ошибки чтения публичного Releases API установщиком; опубликованные данные не заменялись.
- Локальный WinGet v1.29.380: `winget validate` опубликованных manifests — PASS без предупреждений. Шесть опубликованных бинарников/ZIP сверены по GitHub SHA-256 digest и размеру с подтверждённым receipt; Windows exe дополнительно скачан и проверен локально.

Mac-прогон выявил необходимость учитывать системные `/var`, `/tmp`, `/etc` → `/private/...`. Разрешены только эти проверенные нативные aliases; пользовательские symlink parents по-прежнему отклоняются. Добавлен регрессионный тест. Архитектура CLI сохранена.

## Реальный тестовый пакет и Windows

Автор использовал только examples/manufacturing: assembly.pdf, drawing.pdf, bracket.dxf и plate.dxf, помеченные NOT FOR MANUFACTURING. Реальные производственные данные не использовались.

Оба package Release содержат ровно package.zip и nodi-manifest.json, status released, draft=false. Версия 0.1.2: ZIP 1502 байта, SHA-256 `b3a63a766913129e75ca5c762205791d51469894271245fc5338f284dd5344a2`; manifest содержит размеры и SHA-256 четырёх файлов. Получатели скачали выбранную версию, проверили и распаковали её. Повторный get отказал без изменения файлов.

На пользовательском Windows установлен опубликованный CLI: `%LOCALAPPDATA%\Nodi\bin\nodi.exe`, добавлен пользовательский PATH. Использован терминальный установщик с SHA-256, поскольку официальный winget ожидается. Настроен `%APPDATA%\nodi\config.json` для MaximSavhenko/nodi-packages. Credentials не сохранялись: GitHub credential использовался временно только в процессе проверки.

Скачаны Desktop\Nodi\nodi-demo\0.1.1 и 0.1.2. Для обеих `nodi open` открыл Проводник, пути подтверждены через Shell.Application. После открытия нового терминала доступны `nodi version`, `nodi help`, `nodi open nodi-demo`. Для последующих сетевых операций нужен собственный read-токен через `nodi auth login`; для публикации — отдельный write-токен через `nodi auth login --write`.

## Ожидание и непроверенные сценарии

1. **Winget: PENDING — принятие CLA владельцем и проверка Microsoft.** [Бот запросил Contributor License Agreement](https://github.com/microsoft/winget-pkgs/pull/450627#issuecomment-6109602058). Владелец должен прочитать условия и самостоятельно выполнить указание бота, если согласен. MIT лицензия проекта не заменяет соглашение; от имени владельца оно не принималось.
2. После merge и индексации требуется реальный `winget install --id MaximSavhenko.Nodi --exact --source winget`, version/help, detection и uninstall. До этого поддержка официального winget **не завершена**.
   Локальный поиск `winget search --id MaximSavhenko.Nodi --exact --source winget` на дату отчёта: No package found. При переходе на winget нужно удалить терминальный бинарник и его PATH-запись согласно README, сохранив проекты и конфигурацию.
3. Обновление между двумя стабильными версиями через Homebrew/winget не проверено: 0.1.0 — первая версия. Formula updater и UpgradeBehavior подготовлены; Homebrew upgrade текущей версии проверен. Искусственные стабильные версии для upgrade-теста не выпускались.
4. В CI подтверждён вызов native opener; визуальный Finder и физические пользовательские Mac не проверялись. Реальный Проводник Windows подтверждён. macOS подпись/notarization и браузерная установка с Gatekeeper quarantine не проверены.
5. Windows race detector не запускался. Runtime PATH без Go/Git/Node.js проверен; чистая физическая Windows-машина без среды разработки не использовалась.

README содержит реальные команды установки, настройки read/write, публикации, получения, обновления и удаления. Uninstall CLI сохраняет проекты и credentials.
