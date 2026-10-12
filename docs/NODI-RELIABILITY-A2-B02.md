# A2 / B02: восстановление прерванной публикации

Base: `52b8658d68b31613f96e46a3f0e2a20362ce2a96`, main после принятого A1 / PR #3. Перед merge повторно сверены HEAD `c330d8b`, successful CI run `38222344369` и четыре обязательных jobs; merge выполнен с expected head SHA. Ветка A2: `fix/reliability-a2-b02-publish`. RED commit: `62654856f3f53050d88613278db1b12826f4ee1c`.

## Поведение

После обрыва повторить **ту же команду** `nodi send PATH` или `nodi publish ...` с прежним source/status/version. Nodi читает подготовленную операцию с диска, проверяет repository ID, source path, собственный release marker/ID, tag SHA, assets/размеры/хэши и продолжает только доказанное собственное staging. Уже загруженные вложения проверяются и не отправляются повторно. Потерянный ответ PATCH/final GET не создаёт новую версию: доказанное завершение автоматически согласуется с существующей локальной записью версии/receipt.

Новых CLI-команд, флагов, server provider, нового формата manifest, binding или send receipt нет. A3/delete recovery не реализован. Для A2 перенесено только существующее local bookkeeping в общий completion helper: журнал нельзя завершать до сохранения именно его версии. Это минимальная связка journal ↔ local success, а не отдельная реализация A4; дополнительные правила привязки имени и миграции receipts остаются вне PR.

## Журнал и сохранность

В Nodi config directory создаётся `publications/<hash of repository/endpoint/project>/<version>/`:

* `operation.json`: неизменяемые schema/UUID, repository name + numeric ID, endpoint, source path, полный manifest. Никаких токенов.
* `archive.zip`: копия подготовленного ZIP; размер/hash проверяются при копировании и перед продолжением неполной операции. Исходный временный Prepare ZIP может быть удалён, durable spool остаётся.
* `000001.json`, далее небольшие append-only checkpoints: sequence, phase, tag SHA, release ID, подтверждённые asset snapshots и upload intent. Все записи имеют checksum и строгий decoder; это проверка целостности, не подпись/авторизация.

Record сначала пишется в собственный временный файл, sync-ится и переименовывается в новое immutable имя; intent должен сохраниться до remote mutation. Незавершённые временные records сохраняются и игнорируются, повреждённые committed records отвергаются. Unix sync-ит directory; Windows sync-ит файлы. Один OS lock на local repository/project освобождается при завершении процесса; другой проект не блокируется. Journal sequence ограничен 512 checkpoints, превышение даёт отказ до нового write.

Фазы: prepared → reserve intent → tag confirmed → create intent → draft confirmed → uploading → assets verified → finalize intent → verified → complete. Намерение загрузить конкретное вложение сохраняется до POST; lost response после upload сверяется с полными bytes и действительным ID. Известное вложение, которое заменили/удалили, не «чинится» автоматически.

Release body содержит уникальный operation marker вне manifest schema 1. Marker сопоставляется с локальным журналом, точными IDs и hashes; равенства имени, bytes или tag commit SHA недостаточно. GitHub write authority остаётся границей доверия: другой обладатель write-доступа может вручную скопировать/изменить marker. Journal не выдаёт новых прав.

После remote verification сохраняется прежний `last_version`/receipt, затем checkpoint complete и удаляется только verified spool этой операции. Completed journal остаётся свидетельством операции. Если spool отсутствует, но завершённый remote result доказан, допускается read-only reconciliation. Если remote ещё неполон, отсутствующий/повреждённый ZIP или изменившийся source приводят к отказу, а не упаковке другого содержимого под старой версией.

## RED → GREEN и проверки

`TestRegressionB02InterruptedBoundSend`: 11 случаев — lost response до/после reserve/create/archive/manifest/finalize, плюс потерянный final GET. На base девять FAIL; два случая резервирования уже имели сохранное поведение. На исправленном коде все 11 PASS; безопасные пути имеют ровно один committed reserve/create/archive/manifest/finalize, один release и `last_version=0.1.0`. Lost reserve response после mutation остаётся отказом, без повторного write.

Дополнительные meaningful tests:

| Проверка | Oracle |
|---|---|
| `TestB02PublicationAcrossProcesses` | Собственный child принудительно завершён после durable upload intent; новый process завершает тот же ZIP/version; следующий process не создаёт другую версию |
| `TestB02UnsafeRecoveryIsReadOnly` | 14 состояний: corrupt operation/checkpoint, missing/changed ZIP, changed/different source, чужой marker/replaced release, changed tag, extra/replaced/incomplete asset, premature release flags, changed repository ID; все отвергнуты без remote write |
| `TestB02CompletedRemoteDoesNotNeedLocalArchive` | Missing spool + changed current source после фактического PATCH: подтвердить прежний release, сохранить прежнюю локальную version; только GET, без новой публикации |
| `TestB02ExplicitDraftRecoveryPreservesDraftContract` | Same draft ID/версия, без PATCH в released; работает при draft release-by-tag 404 |
| Journal temporary record/lock | Uncommitted temp record не стирается; concurrent local lock отвергается; новый provider читает persistent evidence |
| Journal write failure | Следующий checkpoint блокируется после load: upload не происходит без durable intent |
| Other project | Incomplete demo не блокирует other |
| Independent remote senders | Разные local journal directories одновременно конкурируют за один Git tag; один success, один conflict, один remote release |

Оригинальные tests, которые намеренно требовали вечного отказа после incomplete upload, обновлены под новый договор: только собственный доказанный journal может продолжить операцию; orphan без journal по-прежнему блокирует публикацию. A1 draft/replacement-draft/tombstone tests сохраняются и проходят. Schema-1, old named-project configs, read/write separation, redirects, ZIP/hash/path limits, unchanged-send и completion остаются в общем suite.

Доказательства: [RED](reliability/a2/evidence/red.log), [GREEN](reliability/a2/evidence/green.log), [full tests](reliability/a2/evidence/tests.log), [vet](reliability/a2/evidence/vet.log), [platform](reliability/a2/evidence/platform.log), [local race attempt](reliability/a2/evidence/race-local.log). RED запускается на `6265485` командой `go test -count=1 -v -run '^TestRegressionB02' ./internal/cli`, ожидаемый exit 1. Итоговый focused suite и весь suite — exit 0.

Локально Windows amd64 / Go 1.27.2: full tests, vet, process-kill/restart, negative cases, binary version/help, PowerShell 5.1 completion PASS. Darwin ARM64/Intel cross-build PASS; локально эти binaries не исполнялись. Local race NOT VERIFIED: CGO=0, C compiler отсутствует. Native Windows/macOS/race/upgrade/completion выполняет существующий PR CI; актуальные результаты — в Checks PR. Public release jobs не запускаются без tag; tags и infrastructure не менялись.

## Сохранные отказы и ограничения

* Lightweight Git ref не содержит operation nonce. Если POST reservation успел пройти, но его response/checkpoint не подтверждён, одинаковый SHA не доказывает ownership: автоматического продолжения нет. Journal/archive/tag сохраняются для exact inspection. GitHub reservation conflict также не превращается в automatic cleanup чужого объекта.
* Legacy incomplete 0.4.1 objects не имеют нового marker/journal; automatic adoption запрещён. Healthy legacy packages/configs продолжают читаться и получать новые версии.
* Corrupt committed journal, раннее прерывание до первого prepared checkpoint, missing spool неполного пакета и unknown remote assets требуют inspection. Нет blind retry, automatic replacement, abort/delete или новой recover-команды в A2.
* Проверяется snapshot, но GitHub не предоставляет глобальную transaction для inventory/ref/release/assets. После последней проверки остаётся окно другого writer. Уже завершённые пакеты не изменяются; чужие/несогласованные объекты не удаляются.
* Journal хранит plaintext manifest/source paths и временный plaintext ZIP в config directory; Unix modes 0700/0600, Windows наследует ACL этого directory. Credentials сохраняются прежним отдельным механизмом и не включаются в journal. Pending spools могут занимать до 256 MiB каждый; completed metadata остаётся, автоматического массового GC нет.
* Process-kill test не доказывает hardware power-loss/disk-loss. На Windows portable directory fsync не заявлен. Native CI не заменяет физический Windows ↔ Mac transfer и эксплуатационную приёмку.

B02 в целом не закрыт: delete recovery остаётся A3. A4 не объявляется завершённым. A2 PR остаётся открытым для review, auto-merge выключен.

## Переносимость на Nodi Server 0.6.0

**Частичная, средняя по трудоёмкости.** Source/manifest/ZIP freezing, local journal integrity/locking, before-mutation intents, local acknowledgement и typed rejection of ambiguous state пригодны независимо от транспорта. Повтор CLI с восстановлением прежней операции может сохраниться.

GitHub numeric repository ID, tag SHA/reservation ambiguity, body marker, Release/asset IDs, inventory и draft flags принадлежат этому adapter. В Server их должны заменить namespace UUID, server operation ID, idempotency key, conditional commit/revision и inspect operation. Серверу не нужно копировать GitHub phase machine и mutable body marker; он может доказать reservation/commit атомарно в собственном metadata transaction. Нынешние records versioned schema 1; transport migration потребует нового decoder/mapping, а не изменения старых records на месте.

Серверные interfaces/code/dependencies в A2 не добавлены. Общее выделение engine заранее не выполнялось: сначала проверяется рабочий GitHub recovery, затем отдельный design/review для provider conformance.
