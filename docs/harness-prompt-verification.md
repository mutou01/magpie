# Harness prompt availability: Windows verification

This record supplements the [Library contract](subsystems/library.md#prompt-templates)
and the [earlier native verification](pi-prompt-verification.md). It records the
`bugfix/harness-prompt-availability` work on 2026-10-23 through 2026-10-26; cross-platform builds do
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
- Native dependency-resolution diagnostics initially reported `dsh-at-file`
  failing to import `@deepseek-ai/dsh-settings`. This was not proof that the
  original installation lacked the dependency: linking the whole profile
  `node_modules` changed the real importer path and lost native peer fallback.
  `TestPromptTemplateOriginalDSHProfileLocalPeerFallback` failed before preserving
  physical profile packages and passed afterwards. Existing native compatibility
  grants also need their original `compatibility.json`; the separate grant test
  first failed with a missing command, then passed after copying that file.
  Actual unresolved dependencies and arbitrary startup errors remain blocking,
  with sanitized diagnostics only. No dependency was installed or plugin disabled.
- Codex's ignored `disable_response_storage` setting and another skill's malformed
  metadata previously blocked unrelated templates. Separate public-interface
  tests first failed, then passed after narrow classification. An ignored-setting
  notice must contain exactly that single key and the current config path, with
  no details. A metadata notice must bind to a file in the complete native list;
  its skill name remains occupied, and the template's own bad policy remains
  blocked. Unknown/mixed warnings, requests, unrelated stderr and missing/remote
  metadata sources still fail closed.
- New-name tests first accepted unsupported Unicode/underscore names. The legacy
  regression exercises literal old stored records through read, export, edit
  refusal, sync protection, disable, re-enable refusal, import, rename and delete.
  Reading old data uses its original grammar; new operations use ASCII kebab case.

The native dependency closure contains thousands of source entries, including
published assets. A 30-second two-pass budget failed on this installation.
Content verification is retained; bounded parallel reads and cached directory
metadata reduce overhead, and the DSH two-pass budget is 90 seconds. This can
still make checks and passive source validation expensive on large installations.

## Initial 2026-10-23 observations

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
- At that stage original Codex inventory was blocked on this Windows account by existing
  configuration warnings and skill metadata loading diagnostics. Its user-wide skill discovery can cross a
  temporary HOME through Windows native known-folder lookup. The existing
  `git-workflow/agents/openai.yaml` indentation warning has no independent proof
  of harmless invocation-policy behavior. That file was not changed. Clean
  isolated Codex end-to-end acceptance was therefore outstanding; protocol fixture
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

## 2026-10-24 continuation

Tests and native acceptance in this continuation are delegated to GPT-6.1 Sol
at medium reasoning effort; implementation and read-only source review are
separate. The new original-runtime tests create their own configuration and
plugins outside the installation.

- The DSH profile peer test passed in 136.87 seconds, covering active check,
  passive reuse, a second active check and public save. Each individual check
  retains its 90-second budget. The native compatibility-grant command test
  passed in 51.05 seconds. These results supersede the earlier interpretation
  that the screenshot established a missing installed peer.
- The repeated public DSH inventory/deployment/native-invocation check passed
  in 237.25 seconds. It verified the exact saved artifact, explicit body and
  trailing text, ordinary chat without body injection, rejected model-driven
  skill loading, and original source fingerprints. Empty-root additions and
  dependency-alias retargeting invalidated earlier evidence.
  Cancellation passed in 32.20 seconds: a Windows process handle confirmed the
  original Node process exited, and a passive read could not revive its result.
  ACL/provider adversarial checks passed in 27.99 seconds, environment-layer
  checks in 64.00 seconds, and dependency/error-redaction checks in 11.86 seconds.
- `TestPromptTemplateOriginalCodexConfiguredWarningsPreserveAvailability` passed
  in 3.31 seconds with a test-owned ignored setting and unrelated malformed
  skill metadata. Windows short/long path spelling alone did not reproduce a
  failure; existing file-identity comparison already handles it.
- `verify-codex-prompts.cjs` used a newly built magpie executable and the original
  Codex CLI. Public HTTP passive check returned unverified, explicit check
  returned available with the diagnostic, and save returned enabled. The CLI
  then invoked that same deployed artifact against a loopback Responses model.
  The model received the unique body and Chinese trailing argument; ordinary
  chat received neither the body nor the explicit-only skill's discovery entry.
  A deliberately broken policy in the temporary artifact produced a native
  warning and made that skill discoverable in ordinary chat, demonstrating why
  magpie must retain the own-policy block. Discovery did not itself inject the
  body. All four native calls exited successfully; four selected original
  program/configuration file fingerprints were unchanged. This is limited
  evidence for those files, not a whole-machine side-effect audit.
- A separate isolated copy of selected real configuration sections produced a
  different `configWarning`: invalid configuration with fallback to defaults.
  Its private details identify a Windows file-not-found error. That copy kept
  root path settings such as `model_catalog_json`, but omitted their external
  file dependencies and replaced the home/environment paths. It therefore
  cannot establish a fault in the original configuration or the screenshot's
  warning. Copying the original catalog into the same relative location, with
  an equal SHA-256, removed that warning in the original app-server. The
  remaining warning was exactly the ignored `disable_response_storage` key.
  This establishes the dependency omission in the diagnostic copy. The copy
  excluded MCP servers, plugins and hooks, so this result does not certify the
  user's complete environment or every warning as harmless. Invalid-config
  fallback warnings remain blocking.
- The catalog investigation also exposed missing cache coverage.
  `TestPromptTemplateCodexModelCatalogChangesInvalidateNativeEvidence` first
  failed in all four cases: modifying or deleting a relative/absolute catalog
  reference left the passive result available while `config.toml` stayed
  unchanged. Adding the referenced file to the existing content fingerprint
  made all four cases pass in 13.82 seconds. No configuration was rewritten.
- Original Pi 1.4.0 passed the full procedure again: event-only extension,
  registered command, skill namespace, English/Chinese native expansion and
  management HTTP API, with isolated configuration and a loopback model.

The final Windows and Linux `go vet ./...` checks passed (Linux with `nogui`),
as did Windows, Linux `nogui` and macOS `nogui` builds. The Windows
`go test -tags nogui ./... -count=1 -timeout=10m -p=2` run failed after
1,761.63 seconds. Agent, gateway and Library exhausted their package budgets;
GUI, omarchy, provider, shortcut, sqlitedb and usage also reported failures.
Library reported `TestGoneAgentListedToTakeOut` and
`TestCheckMixedServerEachSide` failures before reaching the timeout during
`TestPromptTemplateCodexStatusNotificationDoesNotBlockAvailability`. At timeout,
that test's current subcase had run for only three seconds; this alone does not
establish a hung native probe. Prior develop failures do not waive this red run.
The three tests were then run separately: all 11 Codex notification subcases
passed in 34.47 seconds; the gone-agent skills and mixed Windows/WSL server
checks still failed (0.68 and 0.92 seconds). The mixed-server fixture supplies
an absolute Windows executable, which the production `checkSides` deliberately
limits to the local side; the fixture expects a WSL result anyway. That finding
does not resolve or waive either repository failure.

The normally enabled prompt regression previously passed in 282.434 seconds;
the later catalog cases passed separately after their fix. Chromium/WebKit
prompt-page checks across five languages passed all 24 reported cases, and
all four dictionary checks passed. The race detector could not start because
CGO is disabled and no C compiler is available. Non-Windows runtime acceptance,
the full user's Codex plugin/MCP environment, Codex notice consistency across
concurrent cache replacement, and DSH linked-root routing changes remain
unverified. These limits preclude a claim of complete acceptance. No push,
merge or release follows from the passing targeted checks.

## DSH missing-export diagnosis (historical)

The later `Native plugin graph contains an inactive or unresolved entry`
report was traced to `dsh-at-file`: it imports `settingsNamespace` from
`@deepseek-ai/dsh-settings`, but the installed dependency does not export that
name. An external read-only client using the original installation's profile
and native resolver reproduced the same `SyntaxError`; this was not an
isolation-copy failure or a plugin waiting for a service.

The preceding diagnostic change distinguished this exact native missing-export error from
arbitrary syntax errors. It reports a static translated reason and the bounded,
validated plugin, dependency and export names. That change left the result unverified.
No plugin, dependency or original configuration is changed, and no inactive
plugin is silently excluded to make the check pass. This diagnostic change
does not make the incompatible installed plugin usable.

Sol medium ran only checks related to this diagnostic change. The public
`CheckPrompt` missing-export test first failed in 6.41 seconds with the generic
graph error. After the fix, the same test passed, including recovery to
available after repairing only its test-owned plugin (92.03 seconds total).
Five adversarial syntax-error cases passed in 31.17 seconds: arbitrary text,
a private dependency path, an invalid export identifier, an overlong identifier
and a trailing payload all remained blocked without disclosing private text.
The native probe's 90-second budget was unchanged. The translation-key test
also failed before the dictionaries were updated; the related translation-key,
duplicate-key, syntax and formatting checks passed afterwards. No full suite
was run for this change, as requested.

The separate public check of the configured installation passed its diagnostic
assertions in 7.47 seconds: unverified with
`dsh-at-file: @deepseek-ai/dsh-settings (settingsNamespace)`. Five selected
existing files retained their fingerprints: `.env`, the web profile manifest,
its patch, `cordis.yml`, and the failing plugin's `lib/index.js`. The root patch
did not exist. This is limited evidence for those files, not a full installation
audit. The existing missing-dependency diagnostic and arbitrary-startup-error
regression also passed (11.72 seconds total).

Cleanup of this check's private diagnostic directory was rejected by automatic
approval review with only `blocked by policy`. It remains at
`C:/Users/Administrator/AppData/Local/Temp/magpie-dsh-graph-diag-56d5e70efc5d4dc3aff50fcf5dc29cb0`,
contains isolated configuration and private logs, and must not be shared.
No alternate cleanup was attempted.

## Effective DSH namespace with an optional link failure (2026-10-25)

The current change follows the effective command/skill namespace rather than
requiring every optional host extension to activate. The installed `dsh-at-file`
still has its original missing-export failure; magpie neither repairs it nor
changes its configuration. Only an independently proven native ESM instantiation
failure may accompany an available/enabled template as a safe warning. Required
startup failures, runtime throws, incomplete inventories and unknown providers
remain blocking.

Native HMR schedules an initial configuration reconciliation after readiness.
It can replace a failed Job and recompose an Entry's options with equal values
after `runProfile()` returns. A later import also has a shorter native stack.
Consequently, object-reference equality and a later audit's unadjusted stack
cannot establish startup identity. The client retains the original failed Job
before yielding, independently proves both jobs' complete linked graphs and
source bytes, compares complete options by value, and restores only those
verified startup stacks in the original audit's generated text. Go requires the
hash of every actual stderr byte to match; neither the Error nor logger changes.

Sol medium first reproduced the public availability failure in 6.58 seconds.
The public Check → Save → Read test now passed in 34.11 seconds, retaining the
safe warning after deployment and rejecting stale evidence after the failed
extension's source changed. A separate public test exposed omitted active host
group sources: editing only that implementation incorrectly kept passive
availability (28.81 seconds). Host groups now join source collection; the native
`cordis:group` is bound to the original loader's exported implementation.
That test passed after the fix (29.22 seconds). The same omission in detached
preset trees was independently reproduced (29.20 seconds); live original
runtime Fibers now supply those groups' actual Entries and declaring bases.
The preset-group regression passed in 28.97 seconds.
A dependency-alias mutation kept the old physical module bytes unchanged:
removing importer-alias collection made the public stale-evidence test fail
(28.91 seconds), and restoring it passed (28.76 seconds).

The configured-profile public Check → Save → original native invocation passed
in 57.23 seconds (package 60.205 seconds). The isolated copy preserved 804
entries and 8,171,006 bytes; the failing `dsh-at-file` remained enabled and
unrepaired. The invocation used the same saved artifact, checked all accepted
source roots before and after, and retained the original optional-link warning.
Five selected original configuration/plugin files also retained their hashes;
this is not a full installation audit.

The original public `defaultId` identified `standard` as the default preset.
The standard, PTC and cordis scopes resolved the original filesystem and
tool-skill modules, exposed the saved skill with user invocation enabled and
model invocation disabled, and passed explicit body expansion, trailing text,
ordinary chat, implicit non-disclosure and actual model-requested-load rejection.
PTC exercised its native `run_code` route. The original minimal composition has
neither skill entry point: its scope had no skill tool or saved skill, and its
slash text and trailing argument remained ordinary input without leaking the
body. Acceptance classifies effective native composition and scoped capabilities,
not preset names; it does not add a missing feature to an original preset.

The native adversarial group passed in 219.773 seconds: runtime throws with the
same SyntaxError text, required missing exports, name conflicts, dependency-alias
changes, unrelated stderr, unknown skill providers, private diagnostics,
counterfeit ACL bodies, missing dependencies, native cancellation, profile peer
fallback and existing compatibility grants. The selected DSH policy/isolation,
Pi/Codex shared-path smoke and Pi timeout/cancellation group passed in 13.799
seconds. The missing-export recovery and configured-warning checks passed in
55.72 and 30.37 seconds respectively. Windows and Linux amd64 Library package
build and vet, changed-file formatting, the three JavaScript syntax checks and
the two selected i18n key checks passed. Sol medium performed these tests and
builds; the primary agent developed the production changes.

As requested, no full repository regression or full GUI suite was run. Native
invocation used the original `runProfile` and Agent APIs with a loopback model;
the `bin.js` argument entry point, remote models and non-Windows runtime were
not tested. The failed-group fixture proves that fixture is blocked, not every
possible non-active group state. Live group source coverage does not prove the
state of disposed or never-created Fibers. Historical concurrency and routing
limitations recorded above remain outside this change.

## 2026-10-25–26: explicit activation and recent inventory evidence

The accepted interaction separates activating prompt templates (check all
installed supported Harnesses) from adding one template's Harness selection
(check that target only). Successful evidence is reused within this process;
manual refresh forces a new check. Reading, saving, syncing and deselecting do
not start native prompt checks. Empty libraries default off; legacy libraries
with templates default on. Closing the switch retains bodies and selections,
and retires unchanged managed deployments. Codex body/policy external edits
keep the whole deployment together.

Sol medium established public API failures before each implementation slice:
the missing total switch and endpoint, activation not checking installed
Harnesses independently, targeted checks starting unrelated Harnesses, missing
cache reuse/timestamps, and global refresh requiring a template name. The
corresponding focused tests then passed. These observations are native-protocol
fixtures unless explicitly marked as original-Harness checks below.

Codex now performs one native inventory enumeration per context. Tests first
failed when unrelated large assets exhausted its byte budget, DSH-only resource
edits invalidated Codex, and saving a managed template invalidated its own
evidence. Those slices passed after narrowing the stamp. The
`TestPromptTemplatesCodexSkillAssetsAreNotAdditionalSkillDeclarations` fixture
first returned unverified for a malformed example nested inside a valid skill's
assets; using the native inventory without a second static declaration scan
passed in 1.93 seconds. These are recent-inventory guarantees, not a claim of
stable discovery across two native boots or future model invocation.

Deterministic browser tests held and released responses to reproduce cancellation
loss after an added target's HTTP failure and a stale all-target response
revoking a newer successful target selection. Both passed after the queue and
per-target response-version fixes. Separate failures established the need for
explicit `force:true` on manual checks and an event-time refresh guard while
target writes are pending. No timing sleeps establish those request orderings.

The Windows original-DSH check
`TestPromptTemplatesOriginalDSHPassiveEvidenceDoesNotRereadModuleBytes` first
failed in 72.33 seconds after a successful isolated native inventory: a Windows
byte-range lock on a test-owned plugin module still allowed metadata reads, but
passive checking reopened its bytes and discarded the result. With active deep
verification retained and passive evidence using a declaration/source-identity
stamp, it passed in 66.80 seconds and retained `checkedAt`. Original user
configuration was not loaded or changed. This does not prove detection of module
edits preserving size and mtime.

The limited Go regression exposed two additional failures: restoring a deselected
Codex deployment lost its evidence because of leftover empty directories, and
deleting an externally edited template kept hiding the retained user skill.
After their fixes, split legacy CLI/Desktop ownership remained protected on
close and was released as a group on deletion. A further Windows file-sharing
lock test first failed in 6.718 seconds: removal retained an unreadable policy,
but subsequent sync could not retire it after unlocking. Distinguishing read
failure from confirmed external edits made that retry pass.

`go test -tags nogui ./internal/library -run '^TestPromptTemplate' -count=1 -v`
passed 98 top-level tests with 26 opt-in skips and no failures in 318.409 seconds.
That run preceded the final read-error distinction; the final retirement impact
group then passed all five tests in 23.361 seconds, including the new Windows
lock case, split ownership, deletion, legacy merging and setup restoration.
The three `TestPromptTemplatesAPI` tests passed in 3.197 seconds. Windows and
Linux amd64 Library package vet/build, changed-file formatting and diff checks
passed. Race checking was unavailable because CGO was disabled and no C compiler
was installed; no dependency was installed to change that environment.

The final limited browser regression passed 36 tests across Chromium/WebKit and
five languages in 86.081 seconds, plus all five translation-key checks in 33.066
seconds. It also retained existing 420/560/1100-pixel inline hint alignment,
non-overlap and horizontal-overflow assertions. Six related JavaScript files
passed syntax checks. No new screenshot was produced.

The final nogui binary passed the original-Codex public API acceptance in
9.840 seconds: default off, passive disabled, explicit activation, check, save,
then CLI invocation of that same deployment. Four loopback-model cases passed,
including explicit body expansion and ordinary chat without the body. The
binary SHA256 was
`58D828EC6CDC2CD4A0A9E6FD913EEBE6F4C78F92590D7297633B0729AA2654C6`.
The original CLI and three selected original configuration file fingerprints
were unchanged. This is a limited file audit, not a whole-installation diff.

This change does not modify Harness programs, user configuration, dependencies
or plugins. Full repository regression is explicitly excluded by the requested
test scope; earlier full-suite/native invocation evidence above remains
historical, not evidence of rerunning those checks for this change.

## 2026-10-26: save cost and independent deployment facts

The public Windows regression
`TestPromptTemplateSaveDoesNotRescanAllSourcesForEveryTemplate` reproduced the
save amplification using `GetProcessIoCounters`, without a production hook or
elapsed-time assertion. With 256 unrelated Claude commands, editing one template
caused 2,099 file reads in a one-template library and 17,079 in an eight-template
library; the twofold allowance failed. Its measurement excludes fixture setup,
active checks and assertion reads. With the fix, the same public regression
recorded 1,081 and 1,319 reads and passed. The read-only `promptReader` now reuses source
evidence within a phase; publication, retirement and response use separate phases.
Destination ownership and content checks remain immediately before mutation.

The public deployment-fact regression first failed after an isolated Codex config
edit invalidated native evidence: unchanged owned files returned `unverified`
without a deployment fact. It passed after adding independent `deployed`, while
the write gate remained `unverified` and passive reads neither launched Codex nor
changed its files. This is a file/deployment contract, not direct Codex Desktop UI
acceptance.

Real HTTP timing uses `measure-prompt-http.cjs`, isolated configuration, 128
unrelated declarations and seven warm samples, without injected delays. Before
the change, Claude Save medians were 216.95 ms for one template and 4,978.33 ms
for sixteen; passive-check/save/read totals were 342.23 and 6,750.70 ms. The
original samples are in
`C:/Users/Administrator/AppData/Local/Temp/magpie-prompt-http-timing-frGywz/timing.json`.
The before binary SHA256 is
`58D828EC6CDC2CD4A0A9E6FD913EEBE6F4C78F92590D7297633B0729AA2654C6`.
Process restart is measured separately; the operating-system file cache is not
flushed. These are measured fixtures, not a latency promise for every user setup.

With the current binary, Claude Save medians were 139.45 ms for one template and
201.27 ms for sixteen; the three-request totals were 226.75 and 302.57 ms.
The sixteen-template Save improved from 4,978.33 to 201.27 ms (about 96%).
The matching after samples are in
`C:/Users/Administrator/AppData/Local/Temp/magpie-prompt-http-timing-4lQ8TM/timing.json`;
the binary SHA256 is
`B6ACF379C97783CE1B857491ADA9DEA63A9BEB03F7CB8624818EEA7D33BC5A9E`.

The matched Codex timing fixture uses eight unrelated declarations, one/four
templates and three warm samples. Save medians changed from 280.12/4,070.18 ms
to 227.68/727.75 ms; three-request totals changed from 476.82/6,022.51 ms to
372.83/1,254.13 ms. Its before/after samples are respectively
`C:/Users/Administrator/AppData/Local/Temp/magpie-prompt-http-timing-J20BNT/timing.json`
and `C:/Users/Administrator/AppData/Local/Temp/magpie-prompt-http-timing-aLfNzq/timing.json`,
with the same before/after binary hashes above. A larger Codex baseline with
128 declarations timed out before completing the sixteen-template comparison;
it is not evidence of a completed before/after measurement. The optimization
does not eliminate filesystem verification cost or guarantee a latency bound.

The three related browser files initially passed 48 tests across Chromium/WebKit
and five languages; the five translation checks also passed. After adding
420/560-pixel overview bounds, Pi stale-evidence and unknown-error controls, the
affected enabled file passed 24 tests in 49.308 seconds. Both browser engines
confirm keyboard access, localized passed status and reason/source/time details.
Only recognized expired-evidence reasons suppress a row alarm for an intact
deployment; unknown native errors retain warnings.

The DSH gate previously required the machine's particular optional-plugin
missing-export warning even on a healthy installation. An isolated original-DSH
healthy-namespace test reproduced that false rejection in 26.73 seconds and
passed in 26.83 seconds after separating general invocation acceptance from the
strict optional-failure case. The latter still requires the original enabled
plugin and exact native missing-export failure; its original invocation passed
in 156.77 seconds. This adjusts acceptance clients, not production availability
rules or installed Harness files.

The mandatory dedicated entry point and its scope are documented in the
[Library regression gate](subsystems/library.md#prompt-template-regression-gate).
Full repository regression is excluded by the user's explicit scope.

The final single invocation of `scripts/test-prompt-templates.ps1` exited zero
with all eight stages passing. Public Library/GUI prompt regressions took
298.47 seconds; Chromium/WebKit across five languages passed 48/48 tests in
56.96 seconds, and the selected native-diagnostic translation check passed.
Original Claude, Pi and Codex public Save/edit/invocation acceptance passed in
3.27, 4.92 and 9.00 seconds. The original DSH general invocation stage passed
in 76.58 seconds with its named test explicitly passing and zero skips.
Its standard/default, ptc and cordis presets expanded the edited body and tail
and rejected model-requested loading; minimal has no native skill route and
preserved the slash text without loading the body. This does not grant template
support to that preset. Selected original-file fingerprints were unchanged:
Claude three, Pi three, Codex four and DSH five files, not a whole-install audit.

The final gate's 1,138 production source fingerprints had zero mismatches, and
its freshly built binary has the same after hash recorded above. Stage commands,
logs and source evidence are retained under
`C:/Users/Administrator/AppData/Local/Temp/magpie-prompt-unified-final-20261026/`
(`stages.json`, `final-proof.json`, `source-snapshot.json`). The related packages
also passed `go vet -tags nogui ./internal/library ./internal/gui` and
`go build -tags nogui ./internal/library ./internal/gui` for Windows and Linux
amd64, changed-Go `gofmt -l` and eight related JavaScript syntax checks. Those
commands/results are in
`C:/Users/Administrator/AppData/Local/Temp/magpie-prompt-reader-final-gate/static-checks-final.json`.
Linux evidence is static/cross-build only; original invocation acceptance is
Windows-only, and Codex acceptance exercises the CLI, not Desktop UI automation.

An earlier browser attempt failed waiting for a Japanese Codex Space activation.
It did not reproduce in the subsequent full run or the final gate; its cause
is unconfirmed. Bounded event diagnostics remain in that test, and the final
test retains keyboard activation and the original assertions/time limit.

## Reproduce opt-in native checks

Use the installed paths from the machine under test, without changing its files:

```powershell
$env:MAGPIE_VERIFY_DSH_HOME = '<original DSH home>'
$env:MAGPIE_VERIFY_NODE_BIN = '<original node.exe>'
$env:MAGPIE_VERIFY_DSH_CONFIGURED_OPTIONAL = '1'
go test -tags nogui ./internal/library -run '^TestPromptTemplateOriginalDSHConfiguredOptionalFailureNativeInvocation$' -count=1 -v
$env:MAGPIE_VERIFY_CODEX_BIN = '<original native codex.exe>'
go test ./internal/library -run '^TestPromptTemplateOriginalCodexConfiguredWarningsPreserveAvailability$' -count=1 -v
node internal/library/testdata/verify-codex-prompts.cjs '<original native codex.exe>' '<newly built magpie.exe>'
node internal/library/testdata/verify-dsh-prompts.mjs '<original CLI lib/bin.js>' '<original DSH home>'
node internal/library/testdata/verify-pi-prompts.cjs '<original Pi dist/cli.js>' '<newly built magpie.exe>'
```

The DSH invocation procedure retains its temporary artifact directory for
inspection. An earlier Codex diagnostic directory's cleanup was rejected by
automatic approval review with only “policy blocked”; that directory remains
and no alternative deletion was attempted.
