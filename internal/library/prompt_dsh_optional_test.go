package library

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/proc"
)

func TestPromptTemplateOriginalDSHConfiguredOptionalFailureNativeInvocation(t *testing.T) {
	verifyConfiguredDSHInvocation(t, true)
}

func TestPromptTemplateOriginalDSHConfiguredNativeInvocation(t *testing.T) {
	verifyConfiguredDSHInvocation(t, false)
}

func TestPromptTemplateOriginalDSHConfiguredAcceptanceAllowsHealthyNamespace(t *testing.T) {
	_, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "export function apply() {}\n")
	state := checkDSHExportFixture(t)
	if state.Status != "available" || state.Reason != "" || state.Source != "" {
		t.Fatalf("healthy original namespace must be available without a warning: %+v", state)
	}
	configuredDSHInvocationWarning(t, state, false)
}

func configuredDSHInvocationWarning(t *testing.T, state PromptState, requireOptional bool) string {
	t.Helper()
	const expected = "dsh-at-file: @deepseek-ai/dsh-settings (settingsNamespace)"
	if !requireOptional && state.Status == "available" && state.Reason == "" && state.Source == "" {
		return ""
	}
	if state.Status != "available" || state.Reason != "DeepSeek Harness plugin requires an export unavailable in the installed dependency" || !regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*: (?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]* \([A-Za-z_$][A-Za-z0-9_$]*\)$`).MatchString(state.Source) || (requireOptional && state.Source != expected) {
		t.Fatalf("configured optional link failure must preserve verified effective namespace: %+v", state)
	}
	return state.Source
}

func verifyConfiguredDSHInvocation(t *testing.T, requireOptional bool) {
	dir, node := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN")
	if runtime.GOOS != "windows" || dir == "" || node == "" || os.Getenv("MAGPIE_VERIFY_DSH_CONFIGURED_OPTIONAL") != "1" {
		t.Skip("opt-in original configured DSH optional failure and loopback invocation")
	}
	// TestMain removes its private temp root after the run. Keep failed native
	// diagnostics beside that root so only safe classifications reach stdout.
	diagnosticRoot := filepath.Dir(os.TempDir())
	before := map[string]string{}
	for _, rel := range []string{".env", "profiles/web/package.json", "profiles/web/cordis.patch.yml", "profiles/web/cordis.yml", "profiles/web/node_modules/dsh-at-file/lib/index.js"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); !requireOptional && os.IsNotExist(err) {
			before[rel] = "missing"
			continue
		}
		before[rel] = promptHash([]byte(read(t, filepath.Join(dir, rel))))
	}
	t.Cleanup(func() {
		for rel, sum := range before {
			if sum == "missing" {
				if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
					t.Errorf("original absent source changed: %s", rel)
				}
				continue
			}
			if promptHash([]byte(read(t, filepath.Join(dir, rel)))) != sum {
				t.Errorf("original selected source changed: %s", rel)
			}
		}
	})
	h := promptEnabledSandbox(t)
	t.Setenv("PATH", filepath.Join(h, "bin")+string(os.PathListSeparator)+filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	isolated := t.TempDir()
	if err := dshPromptCopyProfile(dir, isolated); err != nil {
		t.Fatal("original profile cannot be safely isolated", err)
	}
	copyOutput, err := proc.Command(node, "testdata/copy-dsh-configured-profile.mjs", dir, isolated).Output()
	if err != nil {
		t.Fatal("configured profile copy failed", err)
	}
	var copyReport struct{ CopiedEntries, CopiedBytes int }
	if json.Unmarshal(copyOutput, &copyReport) != nil || copyReport.CopiedEntries == 0 {
		t.Fatal("configured profile copy omitted its evidence")
	}
	t.Logf("Configured profile isolated: %d entries, %d bytes", copyReport.CopiedEntries, copyReport.CopiedBytes)
	t.Setenv("DSH_HOME", isolated)
	state := checkDSHExportFixture(t)
	expected := configuredDSHInvocationWarning(t, state, requireOptional)
	const marker = "CONFIGURED_OPTIONAL_FAILURE_SAVED_BODY"
	const initialMarker = "CONFIGURED_OPTIONAL_FAILURE_REPLACED_BODY"
	result, err := SavePrompt(PromptChange{Name: "audit-code", Text: initialMarker, Agents: []string{"dsh"}})
	if err != nil || result == nil || len(result.Problems) != 0 {
		t.Fatalf("configured optional failure prevented checked public save: %+v %v", result, err)
	}
	artifact := filepath.Join(isolated, "skills/audit-code/SKILL.md")
	if !strings.Contains(read(t, artifact), initialMarker) {
		t.Fatal("initial public save did not publish its body")
	}
	view, err := ReadPrompts()
	if err != nil || len(view.Items) != 1 {
		t.Fatalf("public save did not expose the editable template: %+v %v", view, err)
	}
	result, err = SavePrompt(PromptChange{ID: view.Items[0].ID, Name: "audit-code", Text: marker, Agents: []string{"dsh"}})
	if err != nil || result == nil || len(result.Problems) != 0 {
		t.Fatalf("public edit save failed: %+v %v", result, err)
	}
	deployed := read(t, artifact)
	if !strings.Contains(deployed, marker) || strings.Contains(deployed, initialMarker) {
		t.Fatal("edit save did not replace the same deployed artifact")
	}
	// Export the accepted source roots only as independent invocation evidence;
	// all availability and deployment assertions above use the public boundary.
	nativePromptChecks.Lock()
	roots := append([]string{}, nativePromptChecks.m[nativePromptCacheKey(PromptTarget{ID: "dsh", dir: isolated})].roots...)
	nativePromptChecks.Unlock()
	if len(roots) == 0 {
		t.Fatal("configured native check omitted source proof")
	}
	sources := filepath.Join(t.TempDir(), "sources.json")
	encoded, _ := json.Marshal(roots)
	write(t, sources, string(encoded))
	output, err := proc.Command(node, "testdata/verify-dsh-prompts.mjs", dshPromptEntry(dir), isolated, artifact, marker, sources, "--preserve-profile", initialMarker, expected).Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			diagnostic, createErr := os.MkdirTemp(diagnosticRoot, "magpie-configured-invocation-private-")
			if createErr == nil {
				_ = os.WriteFile(filepath.Join(diagnostic, "stderr.log"), exit.Stderr, 0600)
				t.Logf("Private invocation diagnostic retained at %s", diagnostic)
			}
			line := regexp.MustCompile(`verify-dsh-prompts\.mjs:(\d+)`).FindSubmatch(exit.Stderr)
			if safe := regexp.MustCompile(`MAGPIE_SAFE_EXPLICIT preset=\d+ body=(?:true|false) trailing=(?:true|false) tag=(?:true|false)`).Find(exit.Stderr); len(safe) > 0 {
				t.Log(string(safe))
			}
			for _, safe := range regexp.MustCompile(`MAGPIE_SAFE_(?:PRESET_RESULT|NO_REQUEST) [^\r\n]+`).FindAll(exit.Stderr, -1) {
				t.Log(string(safe))
			}
			if len(line) == 2 {
				t.Logf("Invocation failure at acceptance script line %s; assertion=%t; type-error=%t", line[1], strings.Contains(string(exit.Stderr), "AssertionError"), strings.Contains(string(exit.Stderr), "TypeError"))
			}
		}
		t.Fatal("configured original native invocation failed", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var report struct {
		DeployedArtifactHash, RetainedOptionalFailure string
		SourcesVerified, PreservedProfile             bool
		PresetCount                                   int
		SupportedPresetCount, UnsupportedPresetCount  int
		PresetResults                                 json.RawMessage
	}
	if json.Unmarshal([]byte(lines[len(lines)-1]), &report) != nil || report.DeployedArtifactHash != promptHash([]byte(deployed)) || !report.SourcesVerified || !report.PreservedProfile || report.RetainedOptionalFailure != expected || report.PresetCount < 1 {
		t.Fatal("configured invocation did not prove the same saved artifact with the same optional failure")
	}
	t.Logf("Native preset capability results: %s", report.PresetResults)
	t.Logf("Configured copy: %d entries, %d bytes; original selected files unchanged: %d; original native loopback invocation across %d presets; retained warning: %q", copyReport.CopiedEntries, copyReport.CopiedBytes, len(before), report.PresetCount, expected)
}

func TestPromptTemplateOriginalDSHOptionalExportExceptionRejectsRuntimeThrow(t *testing.T) {
	_, plugin := originalDSHExportFixture(t)
	// Exactly imitates Node's safe-looking linking diagnostic, but executes as
	// plugin code. Error text alone is not evidence of a static link failure.
	message, _ := json.Marshal("The requested module '@deepseek-ai/dsh-settings' does not provide an export named 'settingsNamespace'")
	write(t, filepath.Join(plugin, "index.mjs"), "throw new SyntaxError("+string(message)+");\n")
	state := checkDSHExportFixture(t)
	if state.Status != "unverified" {
		t.Fatalf("runtime code cannot impersonate a proven optional linking failure: %+v", state)
	}
}

func TestPromptTemplateOriginalDSHOptionalExportExceptionRejectsFailedGroup(t *testing.T) {
	isolated, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "import { settingsNamespace } from '@deepseek-ai/dsh-settings';\nexport function apply() { return settingsNamespace('fixture'); }\n")
	group := filepath.Join(filepath.Dir(plugin), "magpie-group-fixture")
	write(t, filepath.Join(group, "package.json"), `{"name":"magpie-group-fixture","version":"1.0.0","type":"module","main":"index.mjs"}`)
	write(t, filepath.Join(group, "index.mjs"), "export function apply() { const error = new Error('PRIVATE_GROUP_FAILURE'); error.stack = 'Error: PRIVATE_GROUP_FAILURE'; throw error; }\n")
	patch := filepath.Join(isolated, "cordis.patch.yml")
	write(t, patch, read(t, patch)+"- insert:\n    - id: group-fixture\n      name: magpie-group-fixture\n      group: true\n      config: []\n")
	state := checkDSHExportFixture(t)
	if state.Status != "unverified" || strings.Contains(state.Reason+state.Source, "PRIVATE_") {
		t.Fatalf("optional link warning cannot authorize another failed host entry hidden by group metadata: %+v", state)
	}
	t.Logf("Failed group blocked: %s; %s", state.Reason, state.Source)
}

func TestPromptTemplateOriginalDSHActiveGroupSourceInvalidatesOptionalEvidence(t *testing.T) {
	isolated, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "import { settingsNamespace } from '@deepseek-ai/dsh-settings';\nexport function apply() { return settingsNamespace('fixture'); }\n")
	group := filepath.Join(filepath.Dir(plugin), "magpie-group-fixture")
	write(t, filepath.Join(group, "package.json"), `{"name":"magpie-group-fixture","version":"1.0.0","type":"module","main":"index.mjs"}`)
	write(t, filepath.Join(group, "index.mjs"), "export function apply() {}\n")
	patch := filepath.Join(isolated, "cordis.patch.yml")
	write(t, patch, read(t, patch)+"- insert:\n    - id: group-fixture\n      name: magpie-group-fixture\n      group: true\n      config: []\n")
	state := checkDSHExportFixture(t)
	if state.Status != "available" {
		t.Fatalf("an active host plugin with group metadata must preserve effective availability: %+v", state)
	}
	write(t, filepath.Join(group, "index.mjs"), "export function apply() {}\n// changed active group source\n")
	states, err := CheckPrompt(PromptChange{Name: "audit-code", Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range states {
		if changed.Agent == "dsh" {
			if changed.Status != "unverified" {
				t.Fatalf("group metadata cannot exclude an active host implementation from source evidence: %+v", changed)
			}
			return
		}
	}
	t.Fatal("active group source edit omitted DSH state")
}

func TestPromptTemplateOriginalDSHActivePresetGroupSourceInvalidatesOptionalEvidence(t *testing.T) {
	isolated, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "import { settingsNamespace } from '@deepseek-ai/dsh-settings';\nexport function apply() { return settingsNamespace('fixture'); }\n")
	group := filepath.Join(filepath.Dir(plugin), "magpie-preset-group-fixture")
	write(t, filepath.Join(group, "package.json"), `{"name":"magpie-preset-group-fixture","version":"1.0.0","type":"module","main":"index.mjs"}`)
	write(t, filepath.Join(group, "index.mjs"), "export function apply() {}\n")
	// Restate the original standard preset, preserving every native row, and
	// append one test-owned group inside its independent preset composition.
	standard := read(t, filepath.Join(os.Getenv("MAGPIE_VERIFY_DSH_HOME"), "profiles/node_modules/@deepseek-ai/dsh-web-app/presets/standard.patch.yml"))
	standard = strings.ReplaceAll(standard, "\r\n", "\n")
	// This fixture is Windows-only. Freeze just the two original platform
	// predicates to their actual values instead of introducing dynamic config.
	standard = strings.ReplaceAll(standard, "!!js process.platform === 'win32'", "true")
	standard = strings.ReplaceAll(standard, "!!js process.platform !== 'win32'", "false")
	_, standard, found := strings.Cut(standard, "- insert:\n")
	if !found {
		t.Fatal("original standard preset patch has no insert composition")
	}
	rows := strings.Split(strings.TrimRight(standard, "\n"), "\n")
	for i := range rows {
		rows[i] = strings.TrimPrefix(rows[i], "    ")
	}
	standard = strings.Join(rows, "\n") + "\n      - id: preset-group-fixture\n        name: magpie-preset-group-fixture\n        group: true\n        config: []\n"
	patch := filepath.Join(isolated, "cordis.patch.yml")
	write(t, patch, read(t, patch)+standard)
	state := checkDSHExportFixture(t)
	if state.Status != "available" {
		t.Fatalf("native preset with an active test group must remain available: %+v", state)
	}
	write(t, filepath.Join(group, "index.mjs"), "export function apply() {}\n// changed active preset group source\n")
	states, err := CheckPrompt(PromptChange{Name: "audit-code", Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range states {
		if changed.Agent == "dsh" {
			if changed.Status != "unverified" {
				t.Fatalf("preset group implementation must participate in native source evidence: %+v", changed)
			}
			return
		}
	}
	t.Fatal("active preset group source edit omitted DSH state")
}

func TestPromptTemplateOriginalDSHRequiredMissingExportStillBlocks(t *testing.T) {
	isolated, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "import { settingsNamespace } from '@deepseek-ai/dsh-settings';\nexport function apply() { return settingsNamespace('fixture'); }\n")
	// Add a required host row without disabling or removing original entries.
	// The original agent-loop is preset-scoped; a root ID override is ignored.
	write(t, filepath.Join(isolated, "cordis.patch.yml"), "- insert:\n    - id: agent-loop\n      name: magpie-missing-export-fixture\n")
	state := checkDSHExportFixture(t)
	if state.Status != "unverified" {
		t.Fatalf("native required startup failure must block template deployment: %+v", state)
	}
}

func TestPromptTemplateOriginalDSHOptionalExportExceptionPreservesConflicts(t *testing.T) {
	isolated, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "import { settingsNamespace } from '@deepseek-ai/dsh-settings';\nexport function apply() { return settingsNamespace('fixture'); }\n")
	path := filepath.Join(isolated, "skills/audit-code/SKILL.md")
	write(t, path, "---\nname: audit-code\ndescription: Independent native skill\n---\nExisting body\n")
	state := checkDSHExportFixture(t)
	if state.Status != "conflict" || !promptSamePath(state.Source, path) {
		t.Fatalf("optional failure exception cannot hide an effective native skill collision: %+v", state)
	}
}

func TestPromptTemplateOriginalDSHOptionalExportDependencyAliasInvalidatesEvidence(t *testing.T) {
	_, plugin := originalDSHExportFixture(t)
	dependency := filepath.Join(filepath.Dir(plugin), "magpie-undeclared-export-dependency")
	write(t, filepath.Join(dependency, "package.json"), `{"name":"magpie-undeclared-export-dependency","version":"1.0.0","type":"module","main":"index.mjs"}`)
	write(t, filepath.Join(dependency, "index.mjs"), "export const present = true;\n")
	write(t, filepath.Join(dependency, "alternate.mjs"), "export const missing = true;\n")
	// This import is deliberately absent from the plugin's manifest: the
	// actual native link graph must still establish its source/alias proof.
	write(t, filepath.Join(plugin, "index.mjs"), "import { missing } from 'magpie-undeclared-export-dependency';\nexport function apply() { return missing; }\n")
	state := checkDSHExportFixture(t)
	if state.Status != "available" || state.Source != "magpie-missing-export-fixture: magpie-undeclared-export-dependency (missing)" {
		t.Fatalf("native optional import must retain provenance even when undeclared in its manifest: %+v", state)
	}
	// Keep all previously linked module bytes unchanged, but retarget the
	// dependency's entry point. Passive reads must reject that stale proof.
	write(t, filepath.Join(dependency, "package.json"), `{"name":"magpie-undeclared-export-dependency","version":"1.0.0","type":"module","main":"alternate.mjs"}`)
	states, err := CheckPrompt(PromptChange{Name: "audit-code", Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range states {
		if changed.Agent == "dsh" {
			if changed.Status != "unverified" {
				t.Fatalf("dependency manifest retarget must invalidate optional-link evidence: %+v", changed)
			}
			return
		}
	}
	t.Fatal("dependency manifest retarget omitted DSH state")
}

func TestPromptTemplateOriginalDSHOptionalExportEarlierDependencyCandidateInvalidatesEvidence(t *testing.T) {
	_, plugin := originalDSHExportFixture(t)
	// Keep the module in lib so adding node_modules does not itself modify a
	// recursively fingerprinted module directory. The prior package stays intact.
	write(t, filepath.Join(plugin, "package.json"), `{"name":"magpie-missing-export-fixture","version":"1.0.0","type":"module","main":"lib/index.mjs","files":["lib"]}`)
	write(t, filepath.Join(plugin, "lib/index.mjs"), "import { missing } from 'magpie-undeclared-export-dependency';\nexport function apply() { return missing; }\n")
	dependency := filepath.Join(filepath.Dir(plugin), "magpie-undeclared-export-dependency")
	write(t, filepath.Join(dependency, "package.json"), `{"name":"magpie-undeclared-export-dependency","version":"1.0.0","type":"module","main":"index.mjs"}`)
	write(t, filepath.Join(dependency, "index.mjs"), "export const present = true;\n")
	state := checkDSHExportFixture(t)
	if state.Status != "available" {
		t.Fatalf("original fallback dependency must establish available optional-link evidence: %+v", state)
	}
	// Introduce a nearer Node search candidate without touching any bytes in
	// the original plugin lib, manifest, or previously resolved dependency.
	shadow := filepath.Join(plugin, "node_modules/magpie-undeclared-export-dependency")
	write(t, filepath.Join(shadow, "package.json"), `{"name":"magpie-undeclared-export-dependency","version":"1.0.0","type":"module","main":"index.mjs"}`)
	write(t, filepath.Join(shadow, "index.mjs"), "export const missing = true;\n")
	states, err := CheckPrompt(PromptChange{Name: "audit-code", Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range states {
		if changed.Agent == "dsh" {
			if changed.Status != "unverified" {
				t.Fatalf("a nearer dependency candidate must invalidate prior native link evidence: %+v", changed)
			}
			return
		}
	}
	t.Fatal("nearer dependency candidate omitted DSH state")
}

func TestPromptTemplateOriginalDSHOptionalExportExceptionRejectsUnrelatedStderr(t *testing.T) {
	isolated, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "import { settingsNamespace } from '@deepseek-ai/dsh-settings';\nexport function apply() { return settingsNamespace('fixture'); }\n")
	noise := filepath.Join(filepath.Dir(plugin), "magpie-stderr-fixture")
	write(t, filepath.Join(noise, "package.json"), `{"name":"magpie-stderr-fixture","version":"1.0.0","type":"module","main":"index.mjs"}`)
	write(t, filepath.Join(noise, "index.mjs"), "export function apply() { console.error('PRIVATE_UNRELATED_STDERR'); }\n")
	write(t, filepath.Join(isolated, "cordis.patch.yml"), read(t, filepath.Join(isolated, "cordis.patch.yml"))+"- insert:\n    - id: stderr-fixture\n      name: magpie-stderr-fixture\n")
	state := checkDSHExportFixture(t)
	if state.Status != "unverified" || strings.Contains(state.Reason+state.Source, "PRIVATE_") {
		t.Fatalf("proven optional warning cannot authorize additional untrusted stderr: %+v", state)
	}
}

func TestPromptTemplateOriginalDSHOptionalExportExceptionRejectsUnknownSkillSource(t *testing.T) {
	isolated, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "import { settingsNamespace } from '@deepseek-ai/dsh-settings';\nexport function apply() { return settingsNamespace('fixture'); }\n")
	provider := filepath.Join(filepath.Dir(plugin), "magpie-dynamic-source-fixture")
	write(t, filepath.Join(provider, "package.json"), `{"name":"magpie-dynamic-source-fixture","version":"1.0.0","type":"module","main":"index.mjs","peerDependencies":{"@deepseek-ai/dsh-skill":"*"}}`)
	write(t, filepath.Join(provider, "index.mjs"), "export const inject=['skills'];\nexport function apply(ctx) { ctx.skills.registerProvider(() => ({name:'unverified-dynamic-source', list:async()=>[], get:async()=>undefined})); }\n")
	write(t, filepath.Join(isolated, "cordis.patch.yml"), read(t, filepath.Join(isolated, "cordis.patch.yml"))+"- insert:\n    - id: dynamic-source-fixture\n      name: magpie-dynamic-source-fixture\n")
	state := checkDSHExportFixture(t)
	if state.Status != "unverified" {
		t.Fatalf("optional link failure exception cannot certify an unknown effective skill source: %+v", state)
	}
}

// This host-only extension fails during native ESM linking. The unmodified
// base/web-app bundles still supply the complete effective skill namespace.
func TestPromptTemplateOriginalDSHOptionalMissingExportPreservesTemplateAvailability(t *testing.T) {
	isolated, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "import { settingsNamespace } from '@deepseek-ai/dsh-settings';\nexport function apply() { return settingsNamespace('fixture'); }\n")
	state := checkDSHExportFixture(t)
	if state.Status != "available" || state.Reason != "DeepSeek Harness plugin requires an export unavailable in the installed dependency" || state.Source != "magpie-missing-export-fixture: @deepseek-ai/dsh-settings (settingsNamespace)" {
		t.Fatalf("native optional link failure must retain verified template availability with a safe warning: %+v", state)
	}
	const body = "OPTIONAL_FAILURE_EXPLICIT_TEMPLATE_BODY"
	result, err := SavePrompt(PromptChange{Name: "audit-code", Text: body, Agents: []string{"dsh"}})
	if err != nil || result == nil || len(result.Problems) != 0 {
		t.Fatalf("verified namespace must allow public deployment despite optional link failure: %+v %v", result, err)
	}
	if !strings.Contains(read(t, filepath.Join(isolated, "skills/audit-code/SKILL.md")), body) {
		t.Fatal("public deployment omitted the template body")
	}
	view, err := ReadPrompts()
	if err != nil || len(view.Items) != 1 || view.Items[0].Text != body {
		t.Fatalf("public read lost the deployed template: %+v %v", view, err)
	}
	found := false
	for _, saved := range view.Items[0].States {
		if saved.Agent == "dsh" {
			found = true
			if saved.Status != "enabled" || saved.Reason != state.Reason || saved.Source != state.Source {
				t.Fatalf("deployed template must remain enabled with its optional-plugin warning: %+v", saved)
			}
		}
	}
	if !found {
		t.Fatal("deployed template omitted its DSH state")
	}
	// Even an ignored extension remains part of the source proof. Changing it
	// must invalidate passive evidence before a new native check is accepted.
	write(t, filepath.Join(plugin, "index.mjs"), read(t, filepath.Join(plugin, "index.mjs"))+"\n// changed optional extension source\n")
	states, err := CheckPrompt(PromptChange{ID: view.Items[0].ID, Name: "audit-code", Text: body, Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range states {
		if changed.Agent == "dsh" {
			if changed.Status != "unverified" {
				t.Fatalf("changed optional plugin source must invalidate previously accepted evidence: %+v", changed)
			}
			return
		}
	}
	t.Fatal("changed optional plugin omitted its DSH state")
}
