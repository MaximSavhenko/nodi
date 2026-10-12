# A4 / B03 first-send identity recovery

Stacked on A3 / PR #5 (`b0ea4a3`), which depends on A2 / PR #4. No merges performed.

Before an unbound source's first remote publication, Nodi persists its chosen name as a pending receipt. Pending means a proposed identity, never permission to adopt an existing remote name. A2 journal ownership checks must prove recovery; completion persists the legacy receipt shape before acknowledging the journal. Already completed receipts retain their ownership on later sends.

RED on A3: `TestRegressionA4FirstSendIdentity` (file and directory) publishes custom `demo`, loses final GET, then repeats plain `send PATH` and incorrectly publishes another package/version identity. GREEN: restart recovers `demo 0.1.0` with no second reserve/create/upload/finalize; receipt is completed. Another source with identical bytes cannot adopt the name. Existing bound recovery, changed source, local acknowledgement failure and CLI tests remain passing.

Evidence: [RED](reliability/batch/a4/a4-red.log), [GREEN](reliability/batch/a4/a4-green.log), [full tests](reliability/batch/a4/a4-tests.log), [vet](reliability/batch/a4/a4-vet.log). Local Windows PASS; native CI at this PR HEAD is required. Legacy completed receipts still decode; pending adds an optional local field, not a manifest change or new CLI flag. Source object/case aliases are handled separately by B05. Hardware power-loss and real-device pilot NOT VERIFIED. No production credentials or installations used.
