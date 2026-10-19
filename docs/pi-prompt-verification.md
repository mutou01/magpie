# Pi prompt verification on Windows

The behavior and API contract belong to the [Library reference](subsystems/library.md#prompt-templates).
These checks exercise that contract without using live configuration or paid
model endpoints. They do not certify Linux, other Pi releases or Codex Desktop.

## Sources and proof

The protocol and trust behavior were checked against the installed Pi 0.87.1
files `dist/modes/rpc/rpc-mode.js`, `dist/core/resource-loader.js`,
`dist/core/package-manager.js` and `dist/core/trust-manager.js`, rather than
inferring command names from extension filenames. The Library skill checker and
MCP checker supply the local snapshot and process-control patterns.

Beyond the reported event-only extension, regression tests cover stale and
concurrent results, read failures, project trust and managed-template renames.
Go tests, Playwright and the installed harness provide separate evidence: the
protocol simulation can prove a failure path, while only real Pi can demonstrate
native command registration and template expansion. Neither proves arbitrary
extension code has no side effects.

## Automated regression

```powershell
go test -tags nogui ./internal/library ./internal/gui -run '^TestPromptTemplate' -count=1 -timeout=5m
node --test internal/gui/tests/library-prompts.test.cjs
node --test internal/gui/tests/i18n-keys.test.cjs
go test -json -count=1 -tags nogui ./... -timeout=45m
go vet ./...
go build
```

Set `NODE_PATH` to the installed Playwright module directory if necessary.
The page test runs Chromium and WebKit, all five languages, a narrow viewport
and both themes. `CHROMIUM_EXECUTABLE` can select a local Chromium executable.

`prompt_runtime_test.go` runs a child executable that speaks Pi's RPC protocol;
it fails if asked to submit a prompt. Public Library operations exercise event-only
extensions, actual command names, fresh checks, cache reuse/failures, source changes,
missing resources, versions, project trust, concurrent edits and concurrent checks.
`prompt_runtime_windows_test.go` verifies the process has ended after cancellation.
These are protocol simulations, not evidence that a real Pi loaded resources.

## Installed Pi acceptance

Run the opt-in procedure with the installed Pi 0.87.1 JavaScript CLI entry point:

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

This procedure passed on Windows with Pi 0.87.1 for the runtime-check change.
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
