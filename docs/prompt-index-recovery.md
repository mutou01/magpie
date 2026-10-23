# Prompt index compatibility and recovery on Windows

The storage contract is in the [Library reference](subsystems/library.md#prompt-templates).
An older binary reads `library.json` into a structure without prompt fields and
writes those fields away. Markdown bodies and native commands can remain while
the Library list becomes empty. File modification times can establish that a
rewrite predates a restart; without a write audit, they do not identify its author.

`library/prompts.json` now keeps the authoritative template records and local
deployment ownership separately. The legacy fields remain a compatibility mirror.
The first save migrates a legacy index; a read does not write. A valid empty
index is a deletion record, not permission to resurrect the mirror. A damaged
or unsupported index returns an error and is left untouched. Its backups are
normal Library file backups. Portable exports use `Bundle`, not a copy of this
local index, and exclude deployment paths and hashes.

This does not make concurrent editing by old and new versions safe for all
Library features. Use one writer per profile. It also does not infer ownership
of arbitrary existing commands merely because they contain the same text.

## Recovery evidence

Before changing a live profile, copy its `library.json`, prompt directory,
existing separate index if present, and candidate native files into a separate
recovery folder outside the normal backup-pruning tree. Record their hashes.
Compare the original bodies with the full generated native files, including
frontmatter, argument suffix and, for Codex, the explicit-only policy file.
Cross-check names, stable ids, creation times and the affected version's source.
Restore only records whose identity and original deployment are independently
established. Keep ambiguous files outside managed ownership for manual review.
Do not enable a target or overwrite an external edit based on a filename guess.
On Windows, restore ownership keys in the native absolute path form emitted by
the adapter, not a manually assembled path with different separators. Verify
the resulting deployed state as well as the file hashes.

After recovery, verify the dedicated prompts endpoint and initial Library
endpoint, open Instructions, refresh and restart. Check that names, ids, text,
target choices and the original native files remain unchanged. Shared instructions
and ordinary skills must still load normally. A UI reload alone cannot repair
an absent index.

## Automated evidence

All commands below run natively on Windows; no WSL or Linux validation is needed.
The Go packages isolate their profiles in `TestMain`.

```powershell
go test -tags nogui ./internal/library ./internal/gui -run '^TestPromptTemplate' -count=1 -timeout=5m
go build -tags nogui -o "$env:TEMP/magpie-prompt-index-windows.exe" .
node internal/library/testdata/verify-prompt-index.cjs "$env:TEMP/magpie-prompt-index-windows.exe"
```

The acceptance procedure copies the executable into a temporary portable profile,
uses a fresh HOME and restricted environment, and submits no harness request or
model prompt. It creates Chinese and English templates through the authenticated
HTTP API, rewrites the legacy mirror, restarts the process and verifies stable
ids/text. It also verifies that corrupting the independent index produces an
HTTP error rather than a successful empty list. Children and the fixture are
removed afterwards.

An optional second executable lets the actual pre-template binary perform the
rewrite through its own sync endpoint. To verify the real page in Chromium and
WebKit as well, provide Playwright on `NODE_PATH` and enable the browser checks:

```powershell
$env:PROMPT_INDEX_UI = '1'
node internal/library/testdata/verify-prompt-index.cjs "$env:TEMP/magpie-prompt-index-windows.exe" '<pre-template-magpie.exe>'
Remove-Item Env:PROMPT_INDEX_UI
node --test internal/gui/tests/library-prompts.test.cjs
go vet ./...
go test -json -count=1 -tags nogui ./... -timeout=45m
```

The existing page regression covers all five languages, narrow windows, themes,
draft retention and native target selection. `prompt_store_test.go` covers legacy
writeback, ownership updates/removal, migrations during unrelated instruction
saves, deletion tombstones, corrupt indexes and ownership backups. Existing
template tests cover portable backups, snapshots and external edits. These
checks supplement each other; the old-binary acceptance does not claim to test
arbitrary concurrent writes or certify harness invocation behavior.
