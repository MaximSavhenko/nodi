# A1 / B01: подтверждённый снимок удаления

Base: `b4f58cbfa3ef0ca4abe56ca067c45039dc56048e` (audited HEAD 0.4.1). Ветка: `fix/reliability-a1-b01`. RED test commit: `c23b6330a4beaf462474a213768431fbcec820f3`. Работа выполнена в отдельном worktree; production main, существующие установки, source packages, credentials и private storage не изменены.

## Исправление

`Deletion` теперь хранит полный набор asset ID/name/size/state и флаги draft/prerelease вместе с прежними manifest/release ID/tag SHA. Повторный PlanDelete должен совпадать с утверждённым набором; идентичные bytes с новым asset ID тоже означают другой объект и требуют нового плана.

После сохранения tombstone Delete заново сравнивает release snapshot, проверяет manifest и повторно скачивает ZIP для сверки SHA-256/size. После проверки bytes ещё раз читает весь release, чтобы увидеть изменения, возникшие во время download. При любом отличии операция останавливается до DELETE release/tag. Порядок assets не является изменением набора. Никаких новых provider abstractions или server dependencies.

CLI-флаги, confirmation, draft deletion по точному ID, отказ при replacement draft, tombstones и запрет переиспользования версии сохраняются. При отказе после создания tombstone история номера остаётся: это не означает, что release уже удалён. Утверждённый release остаётся доступен, повтор требует нового review снимка.

## RED → GREEN

`TestRegressionB01AssetSnapshotChanges` содержит 16 подслучаев: восемь изменений до повторного PlanDelete и после успешного fresh plan. На старом production-коде **шесть FAIL**:

* Замена ZIP и manifest идентичными bytes, но новым ID до recheck — два случая.
* Добавление чужого asset после fresh plan — один случай.
* Замена ZIP и manifest идентичными bytes, но новым ID после fresh plan — два случая.
* Изменение ZIP bytes с прежним размером после fresh plan — один случай.

Все шесть завершали удаление изменённого release/tag; тесты проверяют отказ, сохранность targets и отсутствие destructive HTTP requests, а не только текст ошибки. После исправления все 16 PASS. Remove/rename/size/state и ранняя corruption уже отвергались старым кодом и также остаются PASS.

Дополнительные проверки: `TestRegressionB01AssetOrderDoesNotChangeSnapshot` разрешает безопасное удаление при двух перестановках того же набора; `TestDeleteChangesDuringFinalVerification` отвергает поздний asset, draft/prerelease change и изменившийся tag SHA во время финальной ZIP-проверки. Существующие deletion tests для drafts, replacement draft, tombstones, read-only role, unknown assets, partial failure, batch stop и other projects проходят.

Доказательства: [RED](reliability/a1/evidence/red.log), [GREEN deletion suite](reliability/a1/evidence/deletion-green.log), [полный test suite](reliability/a1/evidence/tests.log), [vet](reliability/a1/evidence/vet.log), [локальные платформенные проверки](reliability/a1/evidence/platform.log), [локальный race attempt](reliability/a1/evidence/race-local.log). RED относится к test-only commit; GREEN — к итоговому коду этой ветки.

Воспроизведение RED из separate checkout commit `c23b633`:

```text
go test -count=1 -v -run '^TestRegressionB01' ./internal/storage
```

Ожидаемый exit 1. На исправленном HEAD ожидаемый exit 0. Итоговые проверки:

```text
go test -count=1 -v -run '^(TestRegressionB01|TestDelete)' ./internal/storage
go test -count=1 -cover ./...
go vet ./...
git diff --check
```

## Проверки платформ и границы

Локально Windows amd64 / Go 1.27.2: deletion suite, полный test suite и vet PASS. Собран и запущен candidate Windows binary (`version`/`help`), штатный PowerShell 5.1 native completion smoke PASS. Сборки darwin/arm64 и darwin/amd64 PASS, но локально на Windows они не исполнялись. PowerShell 7 в PATH отсутствует. Persistent profile modification smoke локально не выполнялся, чтобы не изменять пользовательские profiles.

Локальный race NOT VERIFIED: `CGO_ENABLED=0`, C compiler в PATH не найден, tool отказал с exit 2. Native macOS ARM64/Intel, race и disposable-runner install/upgrade/completion проверки выполняются существующим PR CI; их актуальные результаты находятся в Checks самого PR. Наличие cross-build не является native-проверкой. Workflow и GitHub infrastructure не изменялись; release tag не создавался.

Все HTTP mutation tests используют in-process local TLS fixture и фиктивные tokens. Реальное удаление данных на GitHub и физический Windows ↔ Mac transfer не проводились.

## Остаточные риски

* Последний GET snapshot и DELETE GitHub не являются атомарной conditional operation. Writer может изменить релиз после последнего чтения; fix отвергает увиденное изменение, но не обещает безопасность против конкурента в этом оставшемся окне.
* Финальная SHA-проверка добавляет одно полное чтение ZIP при execution; network failure приводит к сохранному отказу. Resume partial delete относится к B02 и здесь не реализован.
* Tombstone может уже существовать при сохранённом release после refusal. Номер не становится доступным для повторной публикации; tombstone не backup bytes.
* Не выполнять physical power-loss/device acceptance из fixture results. Опубликованные релизы и package storage не менялись.

A2/B02 и B03 не начаты. Для их будущего review приняты уточнения пользователя: безопасное автоматическое восстановление при доказанной идентичности; ручной recover для ambiguous/destructive действий; PR делятся по логическим границам. Server 0.6.0 остаётся отдельной проектной работой.
