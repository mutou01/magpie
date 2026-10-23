# Native prompt verification on Windows

The [harness availability follow-up](harness-prompt-verification.md) supersedes
the historical DSH blocking result below and records the current Codex warning
boundary, dependency/source protections and native deployment acceptance.

The behavior and API contract belong to the [Library reference](subsystems/library.md#prompt-templates).
These checks exercise that contract without changing live configuration or using
paid model endpoints. They do not certify Linux, WSL or Codex Desktop interaction.
Compatibility is established by capabilities, never a version or installation
hash allow-list. No procedure patches an installed Harness program or protocol.

## Sources and proof

Original inputs are the installed Pi protocol, resource/trust loaders, native
slash-command catalog and interactive dispatcher; Codex's native app-server
responses and skill metadata; and DSH's CLI, preset/plugin inventories, skill
provider implementation and remote descriptors. Extension filenames and the
screenshots are not command inventories. The Library skill checker and MCP
checker supply snapshot and process-control patterns. The current Pi catalog is
parsed as JavaScript AST without executing its modules; unrecognized structures
remain unknown. Resource hashes detect changes only.

Beyond the reported event-only extension, regression tests cover stale and
concurrent results, read failures, project trust and managed-template renames.
Go tests, Playwright and the installed harness provide separate evidence: the
protocol simulation can prove a failure path, while only real Pi can demonstrate
native command registration and template expansion. Neither proves arbitrary
extension code has no side effects.

## Codex Agent-aligned detection and CLI-only guidance (2026-10-23)

The current target reuses `Agent.Detected() && !Agent.CLIMissing()` rather than
requiring a CLI for detection. The Agent page also accepts shared-app evidence
and retains uncertain Windows shared folders. Prompt capability checks remain
stricter: without a native CLI entry, new deployments stay blocked and existing
protected deployments are retained. The old Desktop fallback no longer bypasses
native inventory checks. The UI exposes one Codex and only `/skills` / `$name`,
without Desktop-specific invocation, reload or shared-file notes.

The public Library regression first failed on the old mixed invocation string
and on an existing deployment reported as enabled after its CLI was removed.
Browser regressions first failed on the remaining Desktop note and on an
unavailable existing Codex checkbox displayed as unchecked. The editor now
preserves failed-check selections on passive save, allows explicit removal,
and prevents re-enabling a blocked target after removal.

Windows prompt-only validation passed:

- `go test -tags nogui ./internal/library ./internal/gui -run '^TestPromptTemplate' -count=1 -timeout=15m`:
  Library passed in 193.630s; GUI passed in 3.446s. This includes the legacy
  selection, ownership and setup compatibility regressions.
- `library-prompts.test.cjs` and `library-prompt-remove.test.cjs`: all 24 tests
  passed in Chromium and WebKit across five languages, light/dark themes and
  narrow/wide windows. The two editor screenshots were visually inspected.
- `git diff --check` and `gofmt -l` for the touched Go files printed nothing.

Logs, failing-test evidence and screenshots are in
`%TEMP%\magpie-prompt-codex-agent-rule-20261023`. Screenshots use isolated API
fixtures; their conflict messages do not describe live user configuration.
No original Harness was launched, no live configuration was changed, and no
full regression, build/vet or Linux/WSL validation was run for this change.
Historical consolidation evidence follows separately.

## Earlier single-target consolidation (2026-10-23)

The initial consolidation followed Skills' single `codex` selection for the shared
CLI/Desktop native files. Legacy `codex-desktop` requests, stored choices,
ownership and setup snapshots remain compatible; duplicate choices collapse
to one. Reading an old index does not migrate files or start a Harness.
At that stage, installed-CLI verification remained required when the CLI was
present, while Desktop alone used passive checks. The Agent-aligned change above
supersedes that fallback. One configuration does not certify either client's
previously unaccepted native invocation behavior.

The new public-operation regression first failed because the old target was
named `Codex CLI` and Desktop remained separate. Legacy setup comparison also
failed before normalization. Both passed after the changes. A further
Desktop-only test exposed that `CLIMissing` includes apps/IDEs sharing settings;
that implementation separately required a real CLI entry for its probe, while
Desktop installation evidence retained the target without that entry. Tests covered old
index reads without writes, shared deployment removal, external edit retention,
uninstalled targets and setup restore without changing the body.

The Windows Chromium/WebKit prompt list/editor and deletion tests passed all
24 tests across five languages, light/dark themes and wide/narrow windows.
The first deletion run could not launch the absent Playwright Chromium bundle;
the successful run used the existing local Chrome through the recorded launch
preload, with no browser installation. Editor screenshots show one Codex card
with both invocation hints; conflicts in those screenshots are explicit test
fixtures, not claims about the user's real configuration.

The final prompt-only Go run passed:

```powershell
go test -tags nogui ./internal/library ./internal/gui -run '^TestPromptTemplate' -count=1 -timeout=15m
```

It reported Library `ok` (195.290s) and GUI API `ok` (3.238s). `gofmt -l`
on the affected Go files and `git diff --check` were clean.

Evidence is under
`%TEMP%/magpie-prompt-unified-codex-20261023`: `codex-merge-red.txt`,
`codex-merge-green.txt`, `codex-legacy.txt`, `desktop-only-green.txt`,
`setup-red.txt`, `setup-green.txt`, `prompt-go-final.txt`, `prompt-ui-retry.tap`
and `screenshots/`.
Only prompt-specific regression is in scope for this change. No full regression,
Linux/WSL checks or original Harness execution was performed for this merge.
The earlier full-regression and native-acceptance records below belong to the
previous capability change and do not certify this later target merge.

## Automated regression

```powershell
go test -tags nogui ./internal/library ./internal/gui -run '^TestPromptTemplate' -count=1 -timeout=15m
node --test internal/gui/tests/library-prompts.test.cjs
node --test internal/gui/tests/i18n-keys.test.cjs
go test -json -count=1 -tags nogui ./... -timeout=45m -skip='^TestInstallWithoutNode(Parses|StopsWhenTheDownloadFails)$'
go vet ./...
go build
```

Set `NODE_PATH` to the installed Playwright module directory if necessary.
The page test runs Chromium and WebKit, all five languages, a narrow viewport
and both themes. `CHROMIUM_EXECUTABLE` can select a local Chromium executable.
The two excluded historical tests execute Bash to validate Unix installer shell
syntax; they are outside Windows-only acceptance. Do not let PATH resolve Bash
to a WSL launcher. Mock tests whose names contain WSL do not certify a real WSL
environment.

`prompt_runtime_test.go` runs a child executable that speaks Pi's RPC protocol;
it fails if asked to submit a prompt. Public Library operations exercise event-only
extensions, actual command names, fresh checks, cache reuse/failures, source changes,
missing resources, versions, project trust, concurrent edits and concurrent checks.
`prompt_runtime_windows_test.go` verifies the process has ended after cancellation.
These are protocol simulations, not evidence that a real Pi loaded resources.

## Installed Pi acceptance

Run the opt-in procedure with the original installed Pi JavaScript CLI entry
point (read its native package manifest, rather than pinning a release):

```powershell
node internal/library/testdata/verify-pi-prompts.cjs '<Pi installation>/dist/cli.js'
```

An optional second argument exercises the built magpie HTTP API too:

```powershell
go build -o "$env:TEMP/magpie-pi-windows.exe"
node internal/library/testdata/verify-pi-prompts.cjs '<Pi installation>/dist/cli.js' "$env:TEMP/magpie-pi-windows.exe"
```

The HTTP scenario starts a temporary magpie profile and a Pi launcher, checks
passive versus explicit requests, saves a deployment, changes its extension
source and verifies that a passive save keeps the earlier deployment. A fresh
check then reports the real same-name extension. Startup counters prove that
reads and saves do not launch Pi. It never uses the running user's profile.

It creates a temporary directory on the current drive, refuses ancestor shared
skills, and gives Pi a new HOME and agent directory. It starts the installed Pi
with offline RPC, enumerates an event-only extension, a registered `/d` extension
and `/skill:d`, then invokes English and Chinese prompt templates with trailing
user instructions. A loopback HTTP model fixture verifies the actual expanded
request; no external model request or real credentials are used. Processes,
server and fixture directory are removed when it ends. Success prints a JSON
record with the actual Pi version and each assertion's result.

This procedure passed on Windows with Pi 0.87.1 for the earlier runtime-check
change, and Pi 1.3.0 for the capability-based check on 2026-10-22.
It proves native command enumeration and template expansion in the fixture,
not coverage of arbitrary installed extensions or future dynamic changes.
The UI continues to show installation-specific native acceptance separately.

For a live editor smoke check, use an isolated magpie profile, open Instructions,
create a draft and click Check availability. Confirm the Pi warning is visible,
event-only extensions allow the name, a same-name extension reports its source,
and saving another name reuses the command list. Change an extension, then save
without checking: Pi must require another check and keep any earlier deployment.
Closing/reopening or refreshing the page must not launch Pi. Keep existing
Windows suite failures separate from regressions introduced by this change.

## Codex CLI and DeepSeek Harness acceptance

The opt-in Go tests run original Windows installations; ordinary regression tests
use child-process protocol fixtures and never launch installed Harnesses:

```powershell
$env:MAGPIE_VERIFY_CODEX_BIN = '<original Codex CLI executable or shim>'
$env:MAGPIE_VERIFY_NODE_BIN = '<original node.exe>'
go test -v -tags nogui ./internal/library -run '^TestPromptTemplateOriginalCodexSkillInventory$' -count=1 -timeout=2m
Remove-Item Env:MAGPIE_VERIFY_CODEX_BIN
$env:MAGPIE_VERIFY_DSH_HOME = '<original DSH home>'
go test -v -tags nogui ./internal/library -run '^TestPromptTemplateOriginalDSHInventoryCompleteness$' -count=1 -timeout=2m
Remove-Item Env:MAGPIE_VERIFY_DSH_HOME, Env:MAGPIE_VERIFY_NODE_BIN
```

Codex uses a temporary CODEX_HOME and calls only initialize/initialized/skills/list.
Windows' native known-folder lookup can still discover user-wide shared skills:
the 2026-10-22 original CLI returned a metadata warning for the existing
`git-workflow/agents/openai.yaml`. The checker correctly rejected incomplete
loading. That user file was not changed. This is evidence that the protection
works, **not** a complete usable-inventory result.

A separate 2026-10-23 original CLI 0.163.0 `exec` probe used temporary CODEX_HOME,
ephemeral sessions, read-only execution and a loopback Responses fixture.
`$audit-code` loaded the fixture body and retained the trailing user instruction;
the same text without `$` did not inject the body. `$审查` retained the user text
but did not load the body, despite discovery reporting that skill enabled.
Do not infer Chinese `$name` invocation from discovery. An additional native
app-server structured-selection probe did not establish body injection either;
its cause remains unconfirmed. Interactive `/skills` selection and Desktop
candidate selection/template loading remain pending; CLI checks cannot
substitute for Desktop acceptance. The existing metadata warning still prevents
new CLI deployments through the strict checker on this machine.
The updated CLI's executable SHA-256 was unchanged before/after this invocation
and a fresh `skills/list` probe. That diagnostic record binds the evidence to
the installation checked; it is not an accepted-hash list.

DSH copies profile/config into physical temporary storage, links only original
executable dependencies and uses original native remote descriptors. It does not
install a check plugin or submit a prompt. The original installation reached
plugin/preset and temporary-session command/skill enumeration on 2026-10-22.
The native skill endpoint omits provider completeness and effective resource
mapping. The boundary test passes by requiring a concrete unknown result with
uncovered module names. **DSH new deployments remain blocked**, including a
seemingly clean or empty list. This is not successful `/name` invocation
acceptance, and no native API/source patch or version/hash exception is used to
make it pass. Complete-source evidence must be implemented before that boundary
can be relaxed. Startup code can still have external side effects; the temporary
profile is not a sandbox.

## Capability-change verification record (2026-10-22)

Failure-first tests covered Pi 1.3.0/arbitrary diagnostic versions, installed
extension conflicts with no user extension directory, native built-ins and
unknown dispatcher structure, project MCP trust, and source content changes
that preserve size/mtime. Further regressions first failed for revived stale Pi
evidence, shared Codex ownership after an unreadable body, actual skill names
versus folder aliases, an invented project-command namespace, and a managed
skill that the native Codex inventory marked disabled. Authorization requests
cannot be skipped to certify a later inventory. Unmapped file paths and dynamic
DSH profile values are refused before startup. Each regression now
passes after its corresponding implementation. Page tests first rejected the
old `/name`-only introduction and missing capability translations.

Final review added five failure-first cases for mixed known/unknown Pi
dispatcher conditions: regular-expression tests, match/includes calls, a dynamic
helper and switch dispatch. They initially certified an incomplete catalog.
Unknown input-consuming conditions now invalidate the catalog, while the
original installed Pi 1.3.0 definitions still pass (12.853s focused run).

The final focused Windows prompt Go run passed after the dispatcher guard
(`internal/library`: 303.129s; `internal/gui`: 7.350s). Earlier focused runs also
covered the ownership/namespace corrections. Chromium/WebKit prompt
management/deletion passed 24 reported cases,
covering five languages, themes, narrow layouts, keyboard interactions and
drafts. Translation checks passed four cases. Wide/narrow screenshots were
visually inspected. Windows GUI build and `go vet ./...` passed. The real Pi
1.3.0 procedure, including HTTP API and both template languages, passed using
only a loopback model fixture. A follow-up on 2026-10-23 also exercised the
original Claude Code 2.1.308 executable with temporary HOME/CLAUDE_CONFIG_DIR,
user-only settings, no MCP servers or tools and no session persistence. English
and Chinese custom commands expanded their body and trailing user instruction
into the loopback Anthropic request. Its `--bare` mode did not load legacy
custom commands, so that mode is not accepted as evidence for this adapter.
Codex CLI English explicit invocation passed separately, with the Chinese and
selection limitations above. Codex Desktop and DSH invocation acceptance remain
pending as described above.

An additional original Codex `skills/list` probe discovered both `audit-code`
and `审查` from temporary explicit-only skills. The existing shared-skill
metadata warning still prevents certifying the complete inventory; this does
not constitute `$name` invocation acceptance. A before/after content-hash record
of 11 selected original program/configuration files found zero changes across
the real Pi and DSH probes. This records the checked files only, not an absence
of arbitrary plugin side effects. The Windows race detector remains unavailable
because CGO and a C compiler are not configured.

### Full Go regression, 2026-10-22

The full Windows run finished at 23:58:20 (UTC+08:00), with exit code 1:
60 packages passed, nine failed and 19 had no tests. The gateway package timed
out at the requested 45-minute limit while running
`TestClaudeMemRunsFromTheCgroup`. This timeout's cause has not been established.
The run recorded 63 passing top-level `TestPromptTemplate*` cases and three
opt-in original-installation skips. Tests added during final review were run
separately after their failure-first corrections; the full-suite executable
predates those final cases.

The first agent-package process was stopped before its two historical Bash
installer tests could execute. The complete agent package was then rerun with
only `TestInstallWithoutNodeParses` and
`TestInstallWithoutNodeStopsWhenTheDownloadFails` excluded. That run completed
in 1119.831s with nine failing tests. No Bash, Linux or WSL environment was used.
Tests that mock WSL on Windows are not real WSL acceptance.

Failures with the same names as the independently documented 2026-10-19
baseline are listed separately from failures whose current source is unconfirmed:

| Package | Failures also named in the earlier baseline record |
| --- | --- |
| `internal/omarchy` | `TestPlacePanel`, `TestPlacePanelNoOutput` |
| `internal/shortcut` | `TestWrite` |
| `internal/sqlitedb` | `TestSQLiteSurvivesMalformedEnv` |
| `internal/provider` | `TestCodexSignedInInKeychain`, `TestCodexKeychainRefreshWritesItem`, `TestCodexKeychainSwitch`, `TestCodexKeychainAskedSeldom`, `TestCodexKeychainPromptWaitedFor`, `TestCodexKeychainRetry`, `TestCodexKeychainSignOut`, `TestSavedButSignedOut` |
| `internal/usage` | `TestDesktopSessionAccountMetadata` |
| `internal/gui` | `TestGatewayHeldByAContainer`, `TestLodyImportHandoff`, `TestProxyCheckSaysAProxyPassedBy`, `TestUpdateThatWontStart` |
| `internal/library` | `TestGoneAgentListedToTakeOut`, `TestRealDir` |
| `internal/agent` (replacement run) | `TestAsideOfflineDisconnectRestoresWithoutDaemon`, `TestCodexSignInInKeychain`, `TestCopilotFolderFromAnIDEPluginIsNotTheCLI`, `TestQoderSettingsFromAnIDEPluginIsNotTheCLI`, `TestOwnWritesDuringASyncWithALinkedConfig` |

Unconfirmed failures are `internal/provider.TestCodexListTagKeptWhileNothingChanged`
and agent tests `TestWSLClaudeFollowerOnTheMainModelIsNoDrift`,
`TestClaudeDesktopCarriesTheLanguage`, `TestCodexSyncMendsGoneCatalog` and
`TestIsMagpieKeptWhileCatalogStays`, plus the gateway timeout above. Matching a
historical test name is not proof of the same current cause. No unrelated test
or production code was repaired to make the full run green.

### Full GUI regression, 2026-10-22–23

All 834 GUI test files were covered on Windows in three disjoint ranges. The
first run completed the 243 A–K files before repartitioning; its partial L
observations are not counted as coverage. The L–Z run completed a further 216
files before interruption. The interrupted `provider-insecure-tls.test.cjs`
file and all remaining files were rerun together: 375 files, ending at 13:17:15
(UTC+08:00) on 2026-10-23 with exit code 1. That final segment reported 5,286
tests: 5,188 passed, 98 failed, none cancelled or skipped, in 2,044,314.8612ms.
The earlier interrupted logs have no final suite summary, so these numbers are
not an aggregate result for all 834 files. Repeated parent/child failure lines
and overlapping partial observations must not be added into a total.

The full GUI regression is **not passing**. Failures include routing, settings
search, sign-in, updates and usage views. Their current causes have not been
established or reproduced against the base branch; they are not classified as
pre-existing simply because they are outside the prompt tests. Failure titles,
assertions and available stacks remain in the logs. The separately completed
24-case Chromium/WebKit prompt regression above passed. No unrelated UI or
assertion was changed to obtain a green full-suite result.

`gui-a-k-and-partial-l.tap`, `gui-l-z.tap` and `gui-resume.tap` preserve the three
runs. `gui-l-z-files.txt`, `gui-resume-files.txt` and
`gui-coverage-and-failures.json` record their file coverage and raw failure
observations. `verification-index.json` binds the final source, Windows build,
selected logs and screenshots with content hashes; it does not turn a failed
or unverified check into a passing result.

Raw local records are under
`%TEMP%/magpie-harness-prompt-20261022-verification/`: `go-all.jsonl`,
`go-agent-windows.jsonl`, `prompt-go-after-dispatch.txt`, `prompt-ui-five-targets.tap`,
`pi-native-after-dispatch.json`, `claude-native-final.txt`, `codex-native-names.json`,
`codex-native-invocation.txt`, `codex-native-names-latest.json`,
`codex-native-current-installation.json`, `codex-native-selection.txt`, `dsh-native-final.txt`,
`native-file-changes.json` and `screenshots/`.

## Verification record (2026-10-19)

Before implementation, `TestPromptTemplatePiRuntimeChecksCommandsAndReusesResults`
failed because the event-only extension was reported as unverified. The managed
rename regression failed with `deleted managed command remained in cache` before
captured owned commands were marked. The page regression first failed when an
expired answer removed Pi from the save request; an additional failing assertion
required users to be able to explicitly turn that retained target off. Each was
followed by its implementation and a passing focused run.

The final focused Go run passed for prompt templates and the existing shared
instructions, skills and MCP checks (`internal/library`: 201.897s;
`internal/gui`: 3.400s). The prompt page passed 12 cases across Chromium and
WebKit, all five languages and the existing-deployment preservation flow.
The existing instructions' Chinese IME/scroll checks passed 18 cases; skill
update/edit checks passed eight cases. Translation checks passed three cases.
Windows `go vet ./...`, `go build`, the process-command guard and formatting
checks passed. The real Pi 0.87.1 procedure, including the optional HTTP API
scenario, passed with only the loopback model fixture.

The race detector could not run: this Windows environment has CGO disabled and
no C compiler configured (`go: -race requires cgo`). This is an unverified check,
not a passing race result. No Linux verification was performed.

### Full-suite failure classification

The final full Windows run uses
`go test -json -count=1 -tags nogui ./... -timeout=45m`. Disabling the test-result
cache avoids the earlier runner's stalled cache-finalization step. That earlier
runner was stopped only after its test processes had ended; it is not counted as
a completed or passing full run.

The final run finished at 19:52:36 (UTC+08:00), after starting at 19:27:37, with
exit code 1 and no test timeout. It recorded 54 passing packages, 10 failing
packages, 20 packages without tests, 11,070 passing test events (including child
tests), 522 skipped events and 27 failing top-level tests. All 39 top-level
`TestPromptTemplate*` tests passed (49 passing events including child tests).
The full suite is not passing. Its raw local record is
`%TEMP%/magpie-pi-full-go-uncached.jsonl`; the failures below are retained rather
than suppressed or repaired outside the requested scope.

The preceding merge verification independently reproduced 25 existing Windows
failures at develop `224ea7d5bee5759e1e2bfeac8aca4a0c3d9503a5`. They remain outside
this Pi fix, as requested. The affected tests are:

| Package | Existing failures |
| --- | --- |
| `internal/omarchy` | `TestPlacePanel`, `TestPlacePanelNoOutput` |
| `internal/shortcut` | `TestWrite` |
| `internal/sqlitedb` | `TestSQLiteSurvivesMalformedEnv` |
| `internal/provider` | `TestCodexSignedInInKeychain`, `TestCodexKeychainRefreshWritesItem`, `TestCodexKeychainSwitch`, `TestCodexKeychainAskedSeldom`, `TestCodexKeychainPromptWaitedFor`, `TestCodexKeychainRetry`, `TestCodexKeychainSignOut`, `TestSavedButSignedOut` |
| `internal/gateway` | `TestClaudeAsRootEnv` |
| `internal/usage` | `TestDesktopSessionAccountMetadata` |
| `internal/agent` | `TestAsideOfflineDisconnectRestoresWithoutDaemon`, `TestCodexSignInInKeychain`, `TestCopilotFolderFromAnIDEPluginIsNotTheCLI`, `TestQoderSettingsFromAnIDEPluginIsNotTheCLI`, `TestOwnWritesDuringASyncWithALinkedConfig` |
| `internal/gui` | `TestGatewayHeldByAContainer`, `TestLodyImportHandoff`, `TestProxyCheckSaysAProxyPassedBy`, `TestUpdateThatWontStart` |
| `internal/library` | `TestGoneAgentListedToTakeOut`, `TestRealDir` |

The repeated run also exposed `internal/sessions.TestCursorEditorCopiedOncePerChange`:
`magpie-cursor-fresh`, created by the earlier suite at 18:49:43, remained in the
shared Windows temporary directory. The existing tests set `TMPDIR`, whereas
Windows `os.TempDir` uses `TMP`/`TEMP`; the later copy test enumerates the shared
`magpie-cursor-*` paths. A focused rerun reproduced the same failure, and
`git diff 224ea7d5bee5759e1e2bfeac8aca4a0c3d9503a5 -- internal/sessions` is empty.
This isolation defect is recorded without changing those tests or removing
unrelated temporary files.

`internal/agent.TestInstallWithoutNodeParses` also failed with the default PATH:
`bash` resolved to the WindowsApps WSL launcher, which reported no installed
distribution. No Linux environment started. The unchanged test passed in 2.707s
when PATH selected the Windows Git Bash executable instead; this checks shell
syntax on Windows and is not Linux acceptance.
