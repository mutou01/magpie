# Harness prompt availability: Windows verification

This record supplements the [Library contract](subsystems/library.md#prompt-templates)
and the [earlier native verification](pi-prompt-verification.md). It records the
`bugfix/harness-prompt-availability` work on 2026-10-23; cross-platform builds do
not constitute runtime acceptance outside Windows.

## Original evidence and failure-first changes

- Codex's installed app-server JSON schema and native initialize/skills-list
  exchange identify `remoteControl/status/changed` as an independent notification.
  `TestPromptTemplateCodexStatusNotificationDoesNotBlockAvailability` failed with
  invalid native output before the fix. Its request, null-id, malformed, unknown,
  skill-change, approval and metadata-warning neighbors remain blocking.
- Original DSH `runProfile`, profile composition, scoped registry, filesystem
  provider, plugin inventory, Agent commands and ACL asset implementation supply
  the data. No Harness module, method or protocol is replaced. The original
  remote endpoint lacks the completeness evidence needed for deployment.
  `TestPromptTemplateOriginalDSHInventoryCompleteness` first failed because
  discovered resource paths were not stamped before verification. Later slices
  exposed rejected custom read roots and missing command implementation sources.
  Further failure-first slices exposed dependency alias retargeting and wildcard
  publication omissions. Both resolved manifests and installed wildcard resources
  participate in evidence; absent earlier resolution candidates are retained.
- `TestPromptTemplateOriginalDSHRejectsCounterfeitACLBody` loads an independent
  adversarial plugin through the original runtime. It copies the correct ACL
  assets and claims the ACL provider name, but returns a different loaded body.
  The test first failed with a successful inventory; body/metadata verification
  now rejects it. A further subclass overrides the native filesystem provider's
  discovery methods to hide its inventory; it also first returned a successful
  inventory, and exact native provider/method capability checks now reject it.
  No installed module or registry method is patched.
- The screenshot's Codex `chrome/latest` source is a Windows junction, reported
  by Go as an irregular entry rather than a symlink. Failure-first public tests
  now accept directory junctions, invalidate evidence after byte-identical
  retargeting, and reject cycles and broken required source links. Fresh
  component resolution also covers a junction in a resource's parent directory.
  The actual bundled plugin tree contains 539,392,577 file bytes before adding
  the native executable; it exceeded the previous 512 MiB scan cap. Full-byte
  evidence remains bounded to 1 GiB/50,000 entries, with an over-limit regression.
- `TestPromptTemplateDSHEnvironmentLayerCannotBeSilentlyOmitted` failed when a
  home `.env` was dropped by the temporary profile. It is now copied physically
  and read by original `loadLayeredEnv`. Original-runtime tests verify inherited
  process > project > home precedence, Windows name folding, separate processes
  for multiple project contexts, home equal to invoking directory, and native
  bootstrap-variable rejection without exposing its value. Original files remain
  byte-identical. No custom dotenv parser substitutes for native behavior.
- Native dependency-resolution diagnostics exposed `dsh-at-file` importing the
  missing `@deepseek-ai/dsh-settings` peer on this account. Both an independent
  missing-dependency fixture and the actual configured profile now identify the
  plugin/dependency without returning arbitrary startup logs. An arbitrary
  startup error containing a secret sentinel remains blocked without disclosure.
- New-name tests first accepted unsupported Unicode/underscore names. The legacy
  regression exercises literal old stored records through read, export, edit
  refusal, sync protection, disable, re-enable refusal, import, rename and delete.
  Reading old data uses its original grammar; new operations use ASCII kebab case.

The native dependency closure contains thousands of source entries, including
published assets. A 30-second two-pass budget failed on this installation.
Content verification is retained; bounded parallel reads and cached directory
metadata reduce overhead, and the DSH two-pass budget is 90 seconds. This can
still make checks and passive source validation expensive on large installations.

## Observed results

- Public `TestPromptTemplateOriginalDSHInventoryCompleteness`: the final
  expanded-source run passed in 123.76 seconds, reporting 38 command/skill
  observations and 9,896 source entries. The adversarial ACL/provider test
  passed in 9.23 seconds, including the filesystem subclass. An empty custom root remained verifiable; its first
  skill invalidated passive evidence and an explicit recheck reported the real
  conflicting file. Retargeting a dependency to a byte-identical manifest in
  another physical tree invalidated passive evidence.
- `verify-dsh-prompts.mjs` used original installed modules with a physical
  temporary base/web profile and only a loopback Chat Completions model.
  The public Go acceptance checks availability, saves through `SavePrompt`, then
  supplies that exact deployed file to the native invocation procedure. Its
  SHA-256 must match the file loaded by the original runtime. Ordinary chat,
  native `/audit-code` expansion, a separate unique body and trailing user text,
  and absence of implicit body injection passed. Every discovered source, original
  CLI/configuration file, absent candidate and dependency alias was compared
  before/after native invocation with content hashes and canonical path identity;
  the comparison passed. A separate loopback model actually requested the native
  `skill` tool for the explicit-only template: the original handler refused with
  `not available for model invocation`, and its next request contained no unique
  template body. Creating the idle Agent produced no model request. Its retained
  `model-requested-load.json` contains the request/rejection evidence.
  This is not an operating-system sandbox
  or a proof that arbitrary third-party startup code has no side effects.
- Original Pi 1.4.0: event-only extension, registered command, skill namespace,
  English/Chinese native template expansion and management HTTP API passed with
  the newly built magpie executable and a loopback model.
- Original DSH environment-precedence and isolation checks passed in 35.80
  seconds; sanitized missing-dependency/error checks passed in 8.87 seconds.
  The actual configured DSH profile passed its expected-block regression in
  4.91 seconds, identifying `dsh-at-file: @deepseek-ai/dsh-settings`; original
  `.env`, profile and patch fingerprints remained unchanged. The dependency was
  not installed and the plugin was not disabled.
- Original Codex inventory remains blocked on this Windows account by existing
  configuration warnings and skill metadata loading diagnostics. Its user-wide skill discovery can cross a
  temporary HOME through Windows native known-folder lookup. The existing
  `git-workflow/agents/openai.yaml` indentation warning has no independent proof
  of harmless invocation-policy behavior. That file was not changed. Clean
  isolated Codex end-to-end acceptance is therefore outstanding; protocol fixture
  success cannot stand in for that result. Earlier English CLI invocation
  evidence is linked above, with its warning and selection limitations.
  After junction and source-budget compatibility, the original wire exchange
  exposed a native `configWarning` notification before `skills/list`. Its schema
  requires a summary; the checker classifies the warning but keeps it blocking,
  without forwarding summary/details/path. The fixture first failed with generic
  invalid output and now passes with this specific reason. The actual configured
  CLI expected-block regression passed in 2.88 seconds with
  `Codex configuration contains warnings; native verification is incomplete`.
  Raw diagnostics remain private; neither warning nor metadata has been repaired
  by changing the user's Harness configuration.
- Final focused `TestPromptTemplate*` Library regression: passed in 200.906 seconds;
  GUI passed in 3.421 seconds, including the new warning, junction, over-budget
  and environment-copy cases. The original-runtime opt-in checks above are
  separate runs, not implied by the normally skipped tests in this regression.
  Chromium and WebKit prompt management/removal passed all 34 reported cases
  across five languages in 119.934 seconds; dictionary validation passed all
  four checks after adding the Codex configuration-warning translations.
  `TestBuiltinsTranslated` also passed after the final translation changes.
- Windows vet and Windows/Linux/macOS builds passed again after the final code
  changes; Linux vet and Windows whole-repository test compilation passed. The race
  detector could not run because CGO and a C compiler are unavailable.

Full `go test -tags nogui ./... -count=1 -timeout=12m` was executed on the branch
and a separate archive of develop `90364cd1`. Both fail: offline Agent detection,
Linux desktop-panel commands on Windows, Codex account/keychain fixtures, Cursor
session cache, Windows shortcut behavior, malformed Windows environment process
creation and desktop account metadata all have failures on both runs. Agent,
backup, CLI, DAV, gateway, GUI, Library and provider packages also time out on both
runs. The branch additionally timed out in profile; its last running
`TestApplySkipsUndetectedAgents` passes alone. Timing differences are unresolved,
not proof of harmlessness.

The branch-only `TestBuiltinsTranslated` failure was traced to CRLF in the edited
translation file: develop passes; restoring its original LF format makes the
existing test pass. Separately, focused develop/branch runs both reproduce
`TestGoneAgentListedToTakeOut` and `TestCheckMixedServerEachSide`, while
`TestRemoveProjectKeep` passes on both. These comparisons do not establish a green
full suite or waive failures. No commit, push, merge or release is implied.
The final Windows whole-repository compilation follows the Windows CI check;
it does not execute the full suite. No WSL distribution is available on this
machine for a Linux runtime suite. These limits remain acceptance gaps, not
permission to publish over a red run.

## Reproduce opt-in native checks

Use the installed paths from the machine under test, without changing its files:

```powershell
$env:MAGPIE_VERIFY_DSH_HOME = '<original DSH home>'
$env:MAGPIE_VERIFY_NODE_BIN = '<original node.exe>'
go test -tags nogui ./internal/library -run '^TestPromptTemplateOriginalDSH' -count=1 -v -timeout=12m
$env:MAGPIE_VERIFY_DSH_EXPECT_DIAGNOSTIC = 'dsh-at-file: @deepseek-ai/dsh-settings'
go test ./internal/library -run '^TestPromptTemplateOriginalDSHConfiguredDependencyFailure$' -count=1 -v
$env:MAGPIE_VERIFY_CODEX_HOME = '<original Codex home>'
$env:MAGPIE_VERIFY_CODEX_BIN = '<original native codex.exe>'
go test ./internal/library -run '^TestPromptTemplateOriginalCodexConfiguredPolicyFailure$' -count=1 -v
node internal/library/testdata/verify-dsh-prompts.mjs '<original CLI lib/bin.js>' '<original DSH home>'
node internal/library/testdata/verify-pi-prompts.cjs '<original Pi dist/cli.js>' '<newly built magpie.exe>'
```

The DSH invocation procedure retains its temporary artifact directory for
inspection. An earlier Codex diagnostic directory's cleanup was rejected by
automatic approval review with only “policy blocked”; that directory remains
and no alternative deletion was attempted.
