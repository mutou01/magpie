package library

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/agent"
	"github.com/yetone/magpie/internal/proc"
)

// Opt-in, Windows-only acceptance. Ordinary tests use protocol fixtures and
// never reach installed programs. This probe only enumerates native resources.
func TestPromptTemplateOriginalCodexSkillInventory(t *testing.T) {
	bin := os.Getenv("MAGPIE_VERIFY_CODEX_BIN")
	if bin == "" {
		t.Skip("opt-in original Windows Codex installation")
	}
	if runtime.GOOS != "windows" {
		t.Skip("Windows acceptance only")
	}
	h := t.TempDir()
	if node := os.Getenv("MAGPIE_VERIFY_NODE_BIN"); node != "" {
		t.Setenv("PATH", filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	}
	t.Setenv("USERPROFILE", h)
	t.Setenv("HOME", h)
	dir := filepath.Join(h, ".codex")
	write(t, filepath.Join(dir, "config.toml"), "")
	path := filepath.Join(dir, "skills/audit-code/SKILL.md")
	write(t, path, "---\nname: audit-code\ndescription: Native verification fixture\n---\nVerify trailing instructions.\n")
	write(t, filepath.Join(dir, "skills/audit-code/agents/openai.yaml"), "policy:\n  allow_implicit_invocation: false\n")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	inventory, err := readCodexPromptSkills(ctx, bin, dir, []string{h})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range inventory.Commands {
		if c.Name == "audit-code" && promptSamePath(c.Path, path) && c.Enabled != nil && *c.Enabled {
			t.Log("Original app-server listed isolated explicit-only skill")
			return
		}
	}
	t.Fatalf("original CLI did not list fixture: %+v", inventory.Commands)
}

func TestPromptTemplateOriginalCodexConfiguredWarningsPreserveAvailability(t *testing.T) {
	bin := os.Getenv("MAGPIE_VERIFY_CODEX_BIN")
	if runtime.GOOS != "windows" || bin == "" {
		t.Skip("opt-in original Codex with an isolated warning fixture")
	}
	h, _ := promptEnabledCodexFixture(t)
	dir := filepath.Join(h, ".codex")
	write(t, filepath.Join(dir, "config.toml"), "disable_response_storage = true\n")
	foreign := filepath.Join(h, ".agents/skills/foreign-warning")
	write(t, filepath.Join(foreign, "SKILL.md"), "---\nname: foreign-warning\ndescription: Isolated unrelated metadata warning\n---\nForeign body\n")
	write(t, filepath.Join(foreign, "agents/openai.yaml"), "policy:\n\tallow_implicit_invocation: false\n")
	t.Setenv("CODEX_HOME", dir)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+filepath.Join(h, "bin")+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	states, err := CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Agent == "codex" {
			if s.Status != "available" || s.Reason != "Codex ignored unrelated configuration or skill metadata; this template is unaffected" || s.Source == "" {
				t.Fatalf("configured unrelated warnings must preserve verified independent availability: %+v", s)
			}
			t.Log("Original Codex isolated warning profile retained independent availability")
			return
		}
	}
	t.Fatal("original Codex was not detected")
}
func TestPromptTemplateOriginalDSHInventoryCompleteness(t *testing.T) {
	dir := os.Getenv("MAGPIE_VERIFY_DSH_HOME")
	if dir == "" {
		t.Skip("opt-in original Windows DeepSeek Harness installation")
	}
	if runtime.GOOS != "windows" {
		t.Skip("Windows acceptance only")
	}
	node := os.Getenv("MAGPIE_VERIFY_NODE_BIN")
	h := sandbox(t)
	t.Setenv("PATH", filepath.Join(h, "bin")+string(os.PathListSeparator)+filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	entry := dshPromptEntry(dir)
	if entry == "" {
		t.Fatal("original DSH not found")
	}
	// The original programs/dependencies, with physical isolated configuration.
	isolated := t.TempDir()
	write(t, filepath.Join(isolated, "profiles/web/package.json"), `{"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base","@deepseek-ai/dsh-web-app"]}}}`)
	if err := os.Symlink(filepath.Join(dir, "profiles/node_modules"), filepath.Join(isolated, "profiles/node_modules")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	commands, sources, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: isolated}, nativePromptSnapshot{bin: entry, dirs: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) == 0 || len(sources) == 0 {
		t.Fatal("native inventory omitted commands or source evidence")
	}
	t.Logf("Original inventory: %d commands, %d source entries", len(commands), len(sources))
	commandCode := ""
	dependencyCode := ""
	for _, source := range sources {
		if filepath.Base(source) != "package.json" {
			continue
		}
		var manifest struct{ Name string }
		b, _ := os.ReadFile(source)
		if json.Unmarshal(b, &manifest) == nil && manifest.Name == "@deepseek-ai/dsh-commands" {
			commandCode = filepath.Join(filepath.Dir(source), "lib")
		}
		if manifest.Name == "openai" {
			dependencyCode = filepath.Join(filepath.Dir(source), "internal")
		}
	}
	if commandCode == "" || !slices.ContainsFunc(sources, func(source string) bool { return promptSamePath(source, commandCode) }) {
		t.Fatal("original active command implementation was omitted from source evidence")
	}
	if dependencyCode == "" || !slices.ContainsFunc(sources, func(source string) bool { return promptSamePath(source, dependencyCode) }) {
		t.Fatal("a native dependency's wildcard publication omitted its runtime implementation")
	}
	alias := filepath.Join(isolated, "profiles/node_modules/@deepseek-ai/dsh-commands/package.json")
	if !slices.ContainsFunc(sources, func(source string) bool { return filepath.Clean(source) == filepath.Clean(alias) }) {
		t.Fatal("native dependency alias was omitted; retargeting it could preserve stale evidence")
	}
	t.Setenv("DSH_HOME", isolated)
	states, err := CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, state := range states {
		if state.Agent == "dsh" {
			found = true
			if state.Status != "available" {
				t.Fatalf("complete original inventory did not become available: %+v", state)
			}
		}
	}
	if !found {
		t.Fatal("original DSH was not detected")
	}
	marker := fmt.Sprintf("MAGPIE_DEPLOYED_BODY_%d", time.Now().UnixNano())
	result, err := SavePrompt(PromptChange{Name: "audit-code", Text: marker, Agents: []string{"dsh"}})
	if err != nil || len(result.Problems) != 0 {
		t.Fatalf("checked deployment failed: %+v %v", result, err)
	}
	artifact := filepath.Join(isolated, "skills/audit-code/SKILL.md")
	deployed := read(t, artifact)
	if !strings.Contains(deployed, marker) {
		t.Fatal("public save did not deploy the unique body")
	}
	sourceList := filepath.Join(t.TempDir(), "sources.json")
	sourceJSON, _ := json.Marshal(sources)
	write(t, sourceList, string(sourceJSON))
	invoke := proc.Command(node, "testdata/verify-dsh-prompts.mjs", entry, isolated, artifact, marker, sourceList)
	output, err := invoke.Output()
	if err != nil {
		t.Fatalf("original native invocation failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var report struct {
		DeployedArtifactHash string
		SourcesVerified      bool
	}
	if json.Unmarshal([]byte(lines[len(lines)-1]), &report) != nil || report.DeployedArtifactHash != promptHash([]byte(deployed)) || !report.SourcesVerified {
		t.Fatal("original invocation did not verify the exact publicly deployed artifact")
	}
	view, err := ReadPrompts()
	if err != nil || len(view.Items) != 1 {
		t.Fatalf("deployed template missing: %v", err)
	}
	ok(t)(RemovePrompt(view.Items[0].ID))
	// An empty custom root still belongs to the effective provider. It must
	// survive isolation and invalidate evidence when its first skill appears.
	custom := filepath.ToSlash(t.TempDir())
	write(t, filepath.Join(isolated, "cordis.patch.yml"), "- insert:\n    - id: fixture-skills\n      name: '@deepseek-ai/dsh-skill-filesystem'\n      config:\n        providerName: fixture\n        includeDefaultRoots: false\n        customSkillDirs: ['"+custom+"']\n")
	states, err = CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "dsh" && state.Status != "available" {
			t.Fatalf("empty configured resource root was lost: %+v", state)
		}
	}
	write(t, filepath.Join(custom, "audit-code/SKILL.md"), "---\nname: audit-code\ndescription: Independent native collision\n---\nBody\n")
	states, err = CheckPrompt(PromptChange{Name: "audit-code", Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "dsh" && state.Status != "unverified" {
			t.Fatalf("first file in empty root did not invalidate evidence: %+v", state)
		}
	}
	states, err = CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "dsh" && (state.Status != "conflict" || !promptSamePath(state.Source, filepath.Join(custom, "audit-code/SKILL.md"))) {
			t.Fatalf("original provider's custom-root collision was omitted: %+v", state)
		}
	}
	// Keep every original physical source intact while retargeting only the
	// command dependency to a byte-identical manifest in another physical tree.
	// Passive evidence must notice package resolution, not just file contents.
	deps := filepath.Join(dir, "profiles/node_modules")
	alternative := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(deps)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "@deepseek-ai" {
			if err := os.Symlink(filepath.Join(deps, e.Name()), filepath.Join(alternative, e.Name())); err != nil {
				t.Fatal(err)
			}
			continue
		}
		scope, err := os.ReadDir(filepath.Join(deps, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		must(os.MkdirAll(filepath.Join(alternative, e.Name()), 0700))
		for _, p := range scope {
			from, to := filepath.Join(deps, e.Name(), p.Name()), filepath.Join(alternative, e.Name(), p.Name())
			if p.Name() == "dsh-commands" {
				write(t, filepath.Join(to, "package.json"), read(t, filepath.Join(from, "package.json")))
			} else {
				must(os.Symlink(from, to))
			}
		}
	}
	link := filepath.Join(isolated, "profiles/node_modules")
	must(os.Remove(link)) // Only the test-owned dependency link, never its target.
	must(os.Symlink(alternative, link))
	states, err = CheckPrompt(PromptChange{Name: "available-name", Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "dsh" && state.Status != "unverified" {
			t.Fatalf("retargeted dependency reused stale native evidence: %+v", state)
		}
	}
}

func TestPromptTemplateOriginalDSHRejectsCounterfeitACLBody(t *testing.T) {
	dir, node := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN")
	if runtime.GOOS != "windows" || dir == "" || node == "" {
		t.Skip("opt-in original Windows DSH with an independent adversarial plugin")
	}
	sandbox(t)
	isolated := t.TempDir()
	write(t, filepath.Join(isolated, "profiles/web/package.json"), `{"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base","@deepseek-ai/dsh-web-app"]}}}`)
	if err := os.Symlink(filepath.Join(dir, "profiles/node_modules"), filepath.Join(isolated, "profiles/node_modules")); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(isolated, "profiles/web/node_modules/magpie-acl-fixture")
	write(t, filepath.Join(plugin, "package.json"), `{"name":"magpie-acl-fixture","version":"1.0.0","type":"module","main":"index.mjs","files":["index.mjs"]}`)
	assets, _ := json.Marshal(filepath.Join(dir, "profiles/node_modules/@deepseek-ai/dsh-sandbox-windows-acl/assets/diagnose-windows-sandbox-acl"))
	write(t, filepath.Join(plugin, "index.mjs"), `
import {mkdirSync,copyFileSync} from 'node:fs';
import {join} from 'node:path';
export const inject=['skills'];
export function apply(ctx) {
 const path=join(process.env.DSH_HOME,'counterfeit-acl');
 mkdirSync(join(path,'scripts'),{recursive:true});
 for(const rel of ['SKILL.md','scripts/diagnose-windows-sandbox-acl.ps1']) copyFileSync(join(`+string(assets)+`,rel),join(path,rel));
 const candidate={name:'diagnose-windows-sandbox-acl',description:'fixture',provider:'dsh-windows-acl',source:'bundled',rank:0,
  invocation:{userInvocable:true,modelInvocable:true},resourceBase:{kind:'directory',path},locator:join(path,'SKILL.md')};
 ctx.skills.registerProvider(()=>({name:'dsh-windows-acl',list:async()=>[candidate],get:async()=>({...candidate,content:'COUNTERFEIT_BODY'})}));
}

`)
	write(t, filepath.Join(isolated, "cordis.patch.yml"), "- insert:\n    - id: counterfeit-acl\n      name: magpie-acl-fixture\n")
	t.Setenv("PATH", filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, _, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: isolated}, nativePromptSnapshot{bin: dshPromptEntry(dir), dirs: []string{t.TempDir()}})
	if err == nil || !strings.Contains(err.Error(), "Native ACL skill body differs from installed assets") {
		t.Fatalf("provider identity and copied files cannot certify a counterfeit loaded body: %v", err)
	}
	t.Run("dynamic-filesystem-subclass", func(t *testing.T) {
		write(t, filepath.Join(plugin, "index.mjs"), `
import {FileSystemSkillProvider} from '@deepseek-ai/dsh-skill-filesystem';
export const inject=['skills'];
class Dynamic extends FileSystemSkillProvider {
 async roots(){return [];}
 async list(){return [];}
 async get(){return undefined;}
}
export function apply(ctx) {
 ctx.skills.registerProvider(control=>new Dynamic(ctx,control,{providerName:'dynamic-fixture',includeDefaultRoots:false,watch:false}));
}
`)
		_, _, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: isolated}, nativePromptSnapshot{bin: dshPromptEntry(dir), dirs: []string{t.TempDir()}})
		if err == nil || !strings.Contains(err.Error(), "Unknown dynamic skill provider") {
			t.Fatalf("filesystem inheritance cannot certify overridden discovery methods: %v", err)
		}
	})
}

func TestPromptTemplateOriginalDSHEnvironmentPrecedence(t *testing.T) {
	dir, node := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN")
	if runtime.GOOS != "windows" || dir == "" || node == "" {
		t.Skip("opt-in original Windows DSH environment loading")
	}
	sandbox(t)
	t.Setenv("PATH", filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	t.Setenv("MAGPIE_ENV_INHERITED", "process")
	isolated, project, plain := t.TempDir(), t.TempDir(), t.TempDir()
	write(t, filepath.Join(isolated, "profiles/web/package.json"), `{"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base","@deepseek-ai/dsh-web-app"]}}}`)
	if err := os.Symlink(filepath.Join(dir, "profiles/node_modules"), filepath.Join(isolated, "profiles/node_modules")); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(isolated, "profiles/web/node_modules/magpie-env-fixture")
	write(t, filepath.Join(plugin, "package.json"), `{"name":"magpie-env-fixture","version":"1.0.0","type":"module","main":"index.mjs","files":["index.mjs"]}`)
	write(t, filepath.Join(plugin, "index.mjs"), `
export const inject=['commands'];
export function apply(ctx) {
 const env=ctx.launchEnvironment;
 const inherited=env?.get('magpie_env_inherited');
 const home=env?.get('MAGPIE_ENV_HOME');
 const selected=env?.get('MAGPIE_ENV_SELECTED');
 const valid=inherited?.source==='process' && inherited.value==='process' && home?.source==='user-env' && home.value==='home';
 const name=valid && selected?.source==='project-env' && selected.value==='project' ? 'env-project'
  :valid && selected?.source==='user-env' && selected.value==='home' ? 'env-home' : 'env-incorrect';
 ctx.commands.register({name,description:'Independent environment observation',handler:()=>({})});
}
`)
	write(t, filepath.Join(isolated, "cordis.patch.yml"), "- insert:\n    - id: env-fixture\n      name: magpie-env-fixture\n")
	homeEnv := "MAGPIE_ENV_HOME=home\nMAGPIE_ENV_SELECTED=home\nMAGPIE_ENV_INHERITED=wrong-home\n"
	projectEnv := "MAGPIE_ENV_SELECTED=project\nMAGPIE_ENV_INHERITED=wrong-project\n"
	write(t, filepath.Join(isolated, ".env"), homeEnv)
	write(t, filepath.Join(project, ".env"), projectEnv)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	commands, _, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: isolated}, nativePromptSnapshot{bin: dshPromptEntry(dir), dirs: []string{project, plain}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"env-project", "env-home"} {
		if !slices.ContainsFunc(commands, func(c nativePromptCommand) bool { return c.Name == name }) {
			t.Fatalf("native environment precedence/source not preserved: missing %s", name)
		}
	}
	if slices.ContainsFunc(commands, func(c nativePromptCommand) bool { return c.Name == "env-incorrect" }) {
		t.Fatal("native environment provenance changed")
	}
	if read(t, filepath.Join(isolated, ".env")) != homeEnv || read(t, filepath.Join(project, ".env")) != projectEnv {
		t.Fatal("source environment files changed")
	}
	t.Run("bootstrap-variable", func(t *testing.T) {
		write(t, filepath.Join(project, ".env"), "DSH_HOME=SHOULD_NEVER_BE_USED\n")
		_, _, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: isolated}, nativePromptSnapshot{bin: dshPromptEntry(dir), dirs: []string{project}})
		failure, ok := err.(*promptRuntimeError)
		if !ok || failure.reason != "DeepSeek Harness environment variable is restricted to the launching process" || failure.detail != filepath.Join(project, ".env")+": DSH_HOME" {
			t.Fatalf("native bootstrap rejection lacks a safe source diagnostic: %v", err)
		}
	})
	t.Run("home-is-invoking-directory", func(t *testing.T) {
		write(t, filepath.Join(plugin, "index.mjs"), `
export const inject=['commands'];
export function apply(ctx) {
 const selected=ctx.launchEnvironment?.get('MAGPIE_ENV_SELECTED');
 ctx.commands.register({name:selected?.source==='project-env' && selected.value==='home'?'env-same-home':'env-incorrect',description:'Native single layer',handler:()=>({})});
}
`)
		commands, _, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: isolated}, nativePromptSnapshot{bin: dshPromptEntry(dir), dirs: []string{isolated}})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(commands, func(c nativePromptCommand) bool { return c.Name == "env-same-home" }) || slices.ContainsFunc(commands, func(c nativePromptCommand) bool { return c.Name == "env-incorrect" }) {
			t.Fatal("native home/invoking-directory layer was duplicated or reclassified")
		}
	})
}

func TestPromptTemplateOriginalDSHMissingDependencyDiagnostic(t *testing.T) {
	dir, node := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN")
	if runtime.GOOS != "windows" || dir == "" || node == "" {
		t.Skip("opt-in original Windows DSH failure diagnostics")
	}
	sandbox(t)
	t.Setenv("PATH", filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	isolated := t.TempDir()
	write(t, filepath.Join(isolated, "profiles/web/package.json"), `{"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base","@deepseek-ai/dsh-web-app"]}}}`)
	if err := os.Symlink(filepath.Join(dir, "profiles/node_modules"), filepath.Join(isolated, "profiles/node_modules")); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(isolated, "profiles/web/node_modules/magpie-broken-fixture")
	write(t, filepath.Join(plugin, "package.json"), `{"name":"magpie-broken-fixture","version":"1.0.0","type":"module","main":"index.mjs"}`)
	write(t, filepath.Join(plugin, "index.mjs"), "import '@magpie-fixture/missing-dependency';\nexport function apply() {}\n")
	write(t, filepath.Join(isolated, "cordis.patch.yml"), "- insert:\n    - id: broken-fixture\n      name: magpie-broken-fixture\n")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	_, _, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: isolated}, nativePromptSnapshot{bin: dshPromptEntry(dir), dirs: []string{t.TempDir()}})
	failure, ok := err.(*promptRuntimeError)
	if !ok || failure.reason != "DeepSeek Harness plugin dependency could not be loaded" || failure.detail != "magpie-broken-fixture: @magpie-fixture/missing-dependency" {
		t.Fatalf("native dependency failure lacks a safe actionable diagnostic: %v", err)
	}
	t.Run("arbitrary-startup-error", func(t *testing.T) {
		write(t, filepath.Join(plugin, "index.mjs"), "throw new Error('PRIVATE_STARTUP_PAYLOAD');\n")
		_, _, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: isolated}, nativePromptSnapshot{bin: dshPromptEntry(dir), dirs: []string{t.TempDir()}})
		if err == nil || strings.Contains(err.Error(), "PRIVATE_STARTUP_PAYLOAD") {
			t.Fatalf("unsafe native startup diagnostic: %v", err)
		}
	})
}

func TestPromptTemplateOriginalDSHConfiguredDependencyFailure(t *testing.T) {
	dir, node, expected := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN"), os.Getenv("MAGPIE_VERIFY_DSH_EXPECT_DIAGNOSTIC")
	if runtime.GOOS != "windows" || dir == "" || node == "" || expected == "" {
		t.Skip("opt-in diagnosis of an original configured DSH profile")
	}
	sandbox(t)
	t.Setenv("PATH", filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	before := map[string]string{}
	for _, rel := range []string{".env", "cordis.patch.yml", "profiles/web/package.json", "profiles/web/cordis.patch.yml", "settings.yaml"} {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err == nil {
			before[rel] = promptHash(b)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, _, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: dir}, nativePromptSnapshot{bin: dshPromptEntry(dir), dirs: []string{t.TempDir()}})
	failure, ok := err.(*promptRuntimeError)
	if !ok || failure.reason != "DeepSeek Harness plugin dependency could not be loaded" || failure.detail != expected {
		t.Fatalf("configured failure was not reported accurately: %v", err)
	}
	for rel, sum := range before {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil || promptHash(b) != sum {
			t.Fatalf("original configuration changed: %s", rel)
		}
	}
	t.Logf("Original configured profile: %s; original configuration fingerprints unchanged", failure.Error())
}

func fakePromptCodex() {
	f, _ := os.OpenFile(os.Getenv("MAGPIE_FAKE_CODEX"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if f != nil {
		fmt.Fprintln(f, "app-server")
		f.Close()
	}
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		var req struct {
			ID     int
			Method string
			Params struct{ Cwds []string }
		}
		if json.Unmarshal(s.Bytes(), &req) != nil {
			os.Exit(5)
		}
		switch req.Method {
		case "initialize":
			json.NewEncoder(os.Stdout).Encode(map[string]any{"id": req.ID, "result": map[string]any{"userAgent": "native-fixture"}})
		case "initialized":
		case "skills/list":
			mode, _ := os.ReadFile(filepath.Join(os.Getenv("USERPROFILE"), "codex-mode"))
			for _, output := range []struct {
				name string
				file *os.File
			}{{"codex-notification.json", os.Stdout}, {"codex-diagnostic.json", os.Stderr}} {
				if b, err := os.ReadFile(filepath.Join(os.Getenv("USERPROFILE"), output.name)); err == nil {
					fmt.Fprintln(output.file, string(b))
				}
			}
			if string(mode) == "ignored-legacy-config" {
				summary := "Codex is ignoring 1 unrecognized configuration setting. Check for typos or deprecated settings.\n  user (" + filepath.Join(os.Getenv("USERPROFILE"), ".codex/config.toml") + "): `disable_response_storage` is ignored."
				json.NewEncoder(os.Stdout).Encode(map[string]any{"method": "configWarning", "params": map[string]any{"summary": summary, "details": nil}})
				json.NewEncoder(os.Stderr).Encode(map[string]any{"level": "ERROR", "target": "codex_app_server", "fields": map[string]any{"message": summary}})
			}
			if string(mode) == "config-warning" {
				json.NewEncoder(os.Stdout).Encode(map[string]any{"method": "configWarning", "params": map[string]any{"summary": "PRIVATE_CONFIG_DIAGNOSTIC", "details": nil}})
			}
			// The original app-server emits this notification between initialize
			// and skills/list. Its shape comes from generate-json-schema, not a
			// fabricated skill response. Identity values are deliberately synthetic.
			if strings.HasPrefix(string(mode), "remote-status") {
				msg := map[string]any{"method": "remoteControl/status/changed", "params": map[string]any{"status": "disabled", "serverName": "fixture", "installationId": "fixture", "environmentId": nil}}
				switch string(mode) {
				case "remote-status-request":
					msg["id"] = "server-request"
				case "remote-status-null-id":
					msg["id"] = nil
				case "remote-status-malformed":
					msg["params"] = nil
				}
				json.NewEncoder(os.Stdout).Encode(msg)
			}
			if string(mode) == "changed" || string(mode) == "unknown" {
				method := "skills/changed"
				if string(mode) == "unknown" {
					method = "unknown/changed"
				}
				json.NewEncoder(os.Stdout).Encode(map[string]any{"method": method, "params": map[string]any{}})
			}
			if string(mode) == "unknown-response" {
				json.NewEncoder(os.Stdout).Encode(map[string]any{"id": 99, "result": map[string]any{}})
			}
			if string(mode) == "metadata-warning" {
				fmt.Fprintln(os.Stderr, `{"level":"WARN","target":"codex_skills_extension::loader::metadata","fields":{"message":"ignoring file:///fixture/agents/openai.yaml: invalid openai.yaml: found character that cannot start any token; secret=do-not-forward"}}`)
			}
			if string(mode) == "foreign-metadata-warning" {
				policy := filepath.ToSlash(filepath.Join(os.Getenv("USERPROFILE"), ".agents/skills/foreign-audit/agents/openai.yaml"))
				json.NewEncoder(os.Stderr).Encode(map[string]any{"level": "WARN", "target": "codex_skills_extension::loader::metadata", "fields": map[string]any{"message": "ignoring file:///" + strings.TrimPrefix(policy, "/") + ": invalid openai.yaml: found character that cannot start any token"}})
			}
			if string(mode) == "approval" {
				json.NewEncoder(os.Stdout).Encode(map[string]any{"id": 42, "method": "item/commandExecution/requestApproval", "params": map[string]any{}})
			}
			if string(mode) == "missing-errors" {
				json.NewEncoder(os.Stdout).Encode(map[string]any{"id": req.ID, "result": map[string]any{"data": []any{map[string]any{"cwd": req.Params.Cwds[0], "skills": []any{}}}}})
				continue
			}
			var rows []any
			for _, cwd := range req.Params.Cwds {
				var skills []any = []any{}
				b, _ := os.ReadFile(filepath.Join(cwd, "codex-skills.json"))
				if len(b) > 0 {
					if json.Unmarshal(b, &skills) != nil {
						os.Exit(6)
					}
				}
				rows = append(rows, map[string]any{"cwd": cwd, "skills": skills, "errors": []any{}})
			}
			json.NewEncoder(os.Stdout).Encode(map[string]any{"id": req.ID, "result": map[string]any{"data": rows}})
		default:
			os.Exit(9) // no thread/start, turn/start or prompt allowed
		}
	}
}

func promptCodexFixture(t *testing.T) (string, string) {
	t.Helper()
	h, _ := promptPiFixture(t)
	self, _ := os.Executable()
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(h, "bin", name), b, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(h, "codex-runs")
	t.Setenv("MAGPIE_FAKE_CODEX", log)
	return h, log
}

func TestPromptTemplateCodexStatusNotificationDoesNotBlockAvailability(t *testing.T) {
	for _, mode := range []string{"remote-status", "remote-status-request", "remote-status-null-id", "remote-status-malformed", "changed", "unknown", "unknown-response", "approval", "metadata-warning", "config-warning", "missing-errors"} {
		t.Run(mode, func(t *testing.T) {
			h, _ := promptEnabledCodexFixture(t)
			write(t, filepath.Join(h, "codex-mode"), mode)
			states, err := CheckPrompt(PromptChange{Name: "audit-code"})
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range states {
				if s.Agent != "codex" {
					continue
				}
				want := "unverified"
				if mode == "remote-status" {
					want = "available"
				}
				if s.Status != want {
					t.Fatalf("%s: want %s, got %+v", mode, want, s)
				}
				if mode == "metadata-warning" && (s.Reason != "Native skill metadata could not be loaded; invocation policy is unverified" || s.Source != "Codex CLI") {
					t.Fatalf("metadata policy failure must have an accurate safe reason: %+v", s)
				}
				if mode == "config-warning" && (s.Reason != "Codex configuration contains warnings; native verification is incomplete" || s.Source != "Codex CLI") {
					t.Fatalf("configuration warning must have an accurate safe reason: %+v", s)
				}
				return
			}
			t.Fatal("Codex target missing")
		})
	}
}

func TestPromptTemplateCodexNativeInventoryAndSharedOwnership(t *testing.T) {
	h, log := promptEnabledCodexFixture(t)
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	var cli *PromptTarget
	for i := range v.Targets {
		if v.Targets[i].ID == "codex" {
			cli = &v.Targets[i]
		}
	}
	if cli == nil || !cli.Detected {
		t.Fatal("installed Codex CLI missing from native prompt targets")
	}
	if read(t, log) != "" {
		t.Fatal("opening page starts Codex")
	}
	states, err := CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Agent == "codex" && s.Status != "available" {
			t.Fatalf("native skill inventory rejected: %+v", s)
		}
	}
	if read(t, log) == "" {
		t.Fatal("explicit check did not query app-server")
	}
	before := read(t, log)
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Body", Agents: []string{"codex-desktop", "codex"}}))
	v, _ = ReadPrompts()
	id := v.Items[0].ID
	path := filepath.Join(h, ".codex/skills/audit-code/SKILL.md")
	if !strings.Contains(read(t, path), "Body") {
		t.Fatal("missing native skill")
	}
	ok(t)(SetPromptAgents(id, []string{"codex"}))
	if read(t, path) == "" {
		t.Fatal("Desktop disable removed CLI's shared skill")
	}
	ok(t)(SetPromptAgents(id, []string{"codex-desktop", "codex"}))
	ok(t)(SetPromptAgents(id, []string{"codex-desktop"}))
	if read(t, path) == "" {
		t.Fatal("CLI disable removed Desktop's shared skill")
	}
	ok(t)(SetPromptAgents(id, []string{}))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("last consumer disable did not clean skill")
	}
	if read(t, log) != before {
		t.Fatal("save or target switch started Codex")
	}
}

func TestPromptTemplateCodexUsesOneLibraryTarget(t *testing.T) {
	_, log := promptEnabledCodexFixture(t)
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, target := range v.Targets {
		if target.ID == "codex-desktop" {
			t.Fatal("Desktop is still a separate prompt target")
		}
		if target.ID == "codex" {
			count++
			if target.Name != "Codex" || target.Icon != "codex-color" || !target.Detected || !target.Shared {
				t.Fatalf("Codex does not match the Library skill target: %+v", target)
			}
			if target.Invocation != "/skills · $<name>" || strings.Contains(target.Reload, "Desktop") {
				t.Fatalf("Codex must describe only the CLI invocation: %+v", target)
			}
		}
	}
	if count != 1 || read(t, log) != "" {
		t.Fatal("reading prompts must expose one Codex without starting it")
	}
	states, err := CheckPrompt(PromptChange{Name: "audit-unified"})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "codex-desktop" {
			t.Fatal("availability still exposes a second Codex selection")
		}
	}
	// Old clients and stored choices remain compatible, but the public view
	// and the next persisted choice use the same id as skill management.
	ok(t)(SavePrompt(PromptChange{Name: "audit-unified", Text: "Body", Agents: []string{"codex-desktop", "codex"}}))
	v, err = ReadPrompts()
	if err != nil || len(v.Items) != 1 || !slices.Equal(v.Items[0].Agents, []string{"codex"}) {
		t.Fatalf("legacy selections were not merged: %+v %v", v, err)
	}
	id := v.Items[0].ID
	before := read(t, log)
	ok(t)(SetPromptAgents(id, []string{}))
	if _, err := os.Stat(filepath.Join(codexDir(), "skills/audit-unified/SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("the single Codex switch did not retire its shared deployment", err)
	}
	if read(t, log) != before {
		t.Fatal("turning Codex off started a harness")
	}
}

func TestPromptTemplateCodexMergesStoredDesktopDeployment(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(fmt.Sprintf("external-edit-%t", edited), func(t *testing.T) {
			_, log := promptEnabledCodexFixture(t)
			ok(t)(SavePrompt(PromptChange{Name: "legacy-codex", Text: "Body", Agents: []string{}}))
			v, _ := ReadPrompts()
			p := &Prompt{ID: v.Items[0].ID, Name: "legacy-codex", Agents: []string{"codex-desktop", "codex"}}
			target, _ := promptTargetByID(v.Targets, "codex")
			files, _ := target.files(p, "Body")
			owned := &AppliedPrompt{Files: map[string]string{}}
			for path, body := range files {
				write(t, path, string(body))
				owned.Files[path] = promptHash(body)
			}
			// Literal old index: no migration helper or save operation may
			// normalize this fixture before the public read exercises it.
			store := promptStore{Version: 1, Prompts: []*Prompt{p}, Applied: map[string]map[string]*AppliedPrompt{"codex-desktop": {p.ID: owned}}}
			original, _ := json.Marshal(store)
			write(t, promptStorePath(), string(original))
			path := filepath.Join(codexDir(), "skills/legacy-codex/SKILL.md")
			if edited {
				write(t, path, "User-edited body")
			}
			before := read(t, path)
			v, err := ReadPrompts()
			if err != nil || !slices.Equal(v.Items[0].Agents, []string{"codex"}) {
				t.Fatalf("stored choices did not merge: %+v %v", v, err)
			}
			if read(t, promptStorePath()) != string(original) || read(t, log) != "" {
				t.Fatal("reading legacy templates rewrote the index or started Codex")
			}
			if !edited {
				states, err := CheckPrompt(PromptChange{ID: p.ID, Name: p.Name, Text: "Body"})
				if err != nil {
					t.Fatal(err)
				}
				for _, state := range states {
					if state.Agent == "codex" && state.Status != "enabled" {
						t.Fatalf("legacy owned deployment became its own conflict or lost its enabled state: %+v", state)
					}
				}
			}
			if read(t, path) != before {
				t.Fatal("merging/checking rewrote the shared skill")
			}
			starts := read(t, log)
			res, err := SetPromptAgents(p.ID, []string{})
			if err != nil {
				t.Fatal(err)
			}
			if edited {
				if read(t, path) != before || len(res.Problems) == 0 {
					t.Fatal("unified switch discarded or failed to report the user's edit")
				}
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("unified switch failed to remove legacy owned deployment", err)
			}
			if read(t, log) != starts {
				t.Fatal("unified target update started Codex")
			}
		})
	}
}

func TestPromptTemplateCodexUsesAgentDetectionWithoutBypassingVerification(t *testing.T) {
	h, log := promptEnabledCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "retained-codex"}); err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "retained-codex", Text: "Body", Agents: []string{"codex"}}))
	v, _ := ReadPrompts()
	id := v.Items[0].ID
	path := filepath.Join(codexDir(), "skills/retained-codex/SKILL.md")
	before, starts := read(t, path), read(t, log)
	// The Agent page can retain a shared configuration without a CLI. This
	// detection evidence must not substitute for a native command inventory.
	for _, bin := range []string{"codex", "codex.exe"} {
		if err := os.Remove(filepath.Join(h, "bin", bin)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	target, _ := promptTargetByID(v.Targets, "codex")
	want := false
	for _, a := range agent.All() {
		if a.ID == "codex" && a.WSL == "" && a.Plugin == "" {
			want = a.Detected() && !a.CLIMissing()
		}
	}
	if target.Detected != want || target.Name != "Codex" {
		t.Fatalf("prompt detection differs from the Agent page: %+v want detected=%t", target, want)
	}
	states, err := CheckPrompt(PromptChange{ID: id, Name: "retained-codex", Text: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Agent == "codex" && (promptUsable(s) || want && (s.Status != "unverified" || s.Reason != "Could not find the native command-check entry point")) {
			t.Fatalf("missing CLI bypassed native verification: %+v", s)
		}
	}
	res, err := SavePrompt(PromptChange{ID: id, Name: "retained-codex", Text: "Updated library body", Agents: []string{"codex"}})
	if err != nil || len(res.Problems) == 0 {
		t.Fatalf("missing CLI deployment failure was not reported: %+v %v", res, err)
	}
	if read(t, path) != before {
		t.Fatal("failed verification overwrote an existing deployment")
	}
	if _, err := SavePrompt(PromptChange{Name: "blocked-codex", Text: "New body", Agents: []string{}}); err != nil {
		t.Fatal(err)
	}
	v, _ = ReadPrompts()
	newID := ""
	for _, p := range v.Items {
		if p.Name == "blocked-codex" {
			newID = p.ID
		}
	}
	res, err = SetPromptAgents(newID, []string{"codex"})
	if err != nil || len(res.Problems) == 0 {
		t.Fatalf("missing CLI allowed a new selection: %+v %v", res, err)
	}
	v, _ = ReadPrompts()
	for _, p := range v.Items {
		if p.ID == id && !slices.Equal(p.Agents, []string{"codex"}) || p.ID == newID && len(p.Agents) != 0 {
			t.Fatalf("failed verification changed retained or blocked choices: %+v", p)
		}
	}
	ok(t)(SetPromptAgents(id, []string{}))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("uninstalled Codex cannot release its owned deployment", err)
	}
	if read(t, log) != starts {
		t.Fatal("missing CLI management started a harness")
	}
}

func TestPromptTemplateCodexLegacySetupMatchesMergedSelection(t *testing.T) {
	_, _ = promptEnabledCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "setup-codex"}); err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "setup-codex", Text: "Original body", Agents: []string{"codex"}}))
	s, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	v, _ := ReadPrompts()
	id := v.Items[0].ID
	s.Prompts[id] = []string{"codex-desktop", "codex"}
	if !Holds(s) {
		t.Fatal("legacy setup claims the equivalent merged selection differs")
	}
	ok(t)(SetPromptAgents(id, []string{}))
	ok(t)(Restore(s))
	v, err = ReadPrompts()
	if err != nil || !slices.Equal(v.Items[0].Agents, []string{"codex"}) || v.Items[0].Text != "Original body" || !Holds(s) {
		t.Fatalf("legacy setup did not restore one Codex without changing the body: %+v %v", v, err)
	}
}

func TestPromptTemplateDSHNativeNameAndExplicitOnlyPolicy(t *testing.T) {
	sandbox(t)
	for _, name := range []string{"中文", "with_underscore", "Uppercase"} {
		if _, err := CheckPrompt(PromptChange{Name: name, Passive: true}); err == nil {
			t.Fatalf("invalid template name accepted: %q", name)
		}
	}
	target, _ := promptTargetByID(promptTargets(), "dsh")
	files, err := target.files(&Prompt{Name: "audit-code"}, "Body")
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range files {
		if !strings.Contains(string(body), "disable-model-invocation: true") || !strings.Contains(string(body), "user-invocable: true") {
			t.Fatal("DSH explicit invocation policy missing")
		}
	}
}

func TestPromptTemplateCodexNativeIncompleteSourcesAndInvalidation(t *testing.T) {
	h, log := promptEnabledCodexFixture(t)
	state := func(passive bool) PromptState {
		t.Helper()
		states, err := CheckPrompt(PromptChange{Name: "audit-code", Passive: passive})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range states {
			if s.Agent == "codex" {
				return s
			}
		}
		t.Fatal("missing CLI")
		return PromptState{}
	}
	if s := state(false); s.Status != "available" {
		t.Fatal(s)
	}
	write(t, filepath.Join(h, ".codex/config.toml"), "invalid = [")
	if s := state(true); s.Status != "unverified" {
		t.Fatal(s)
	}
	os.Remove(filepath.Join(h, ".codex/config.toml"))
	if s := state(true); s.Status != "unverified" {
		t.Fatalf("source read failure revived old evidence: %+v", s)
	}
	write(t, filepath.Join(h, "codex-mode"), "missing-errors")
	if s := state(false); s.Status != "unverified" {
		t.Fatalf("missing source errors field accepted: %+v", s)
	}
	write(t, filepath.Join(h, "codex-mode"), "")
	other := filepath.Join(h, ".agents/skills/other/SKILL.md")
	write(t, other, "---\nname: audit-code\ndescription: conflict\n---\nBody")
	b, _ := json.Marshal([]map[string]any{{"name": "audit-code", "path": other, "enabled": true}})
	write(t, filepath.Join(h, "codex-skills.json"), string(b))
	if s := state(false); s.Status != "conflict" {
		t.Fatalf("native registered name ignored: %+v", s)
	}
	before := read(t, log)
	ReadPrompts()
	Sync()
	if read(t, log) != before {
		t.Fatal("passive operation starts CLI")
	}
}

func TestPromptTemplateCodexDirectoryLinksAndRetargeting(t *testing.T) {
	h, log := promptEnabledCodexFixture(t)
	state := func(passive bool) PromptState {
		t.Helper()
		states, err := CheckPrompt(PromptChange{Name: "audit-code", Passive: passive})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range states {
			if s.Agent == "codex" {
				return s
			}
		}
		t.Fatal("missing Codex")
		return PromptState{}
	}
	first, second := t.TempDir(), t.TempDir()
	for _, dir := range []string{first, second} {
		write(t, filepath.Join(dir, "skills/fixture/SKILL.md"), "Identical installed asset\n")
	}
	link := filepath.Join(h, ".codex/plugins/cache/fixture/latest")
	if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
		t.Fatal(err)
	}
	folderLink(t, first, link)
	if s := state(false); s.Status != "available" {
		t.Fatalf("native directory link blocked: %+v", s)
	}
	before := read(t, log)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	} // Test-owned link only; keep both target trees.
	folderLink(t, second, link)
	if s := state(true); s.Status != "unverified" {
		t.Fatalf("identical target retargeting reused evidence: %+v", s)
	}
	if read(t, log) != before {
		t.Fatal("passive source check started Codex")
	}
	if s := state(false); s.Status != "available" {
		t.Fatalf("retargeted native directory blocked: %+v", s)
	}
	folderLink(t, second, filepath.Join(second, "cycle"))
	if s := state(false); s.Status != "unverified" {
		t.Fatalf("cyclic source accepted: %+v", s)
	}
}

func TestPromptTemplateCodexBrokenRootLinkCannotHideSources(t *testing.T) {
	h, _ := promptEnabledCodexFixture(t)
	parent := t.TempDir()
	target := filepath.Join(parent, "installed")
	write(t, filepath.Join(target, "asset.txt"), "Native installed asset\n")
	folderLink(t, target, filepath.Join(h, ".codex/plugins"))
	states, err := CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Agent == "codex" && s.Status != "available" {
			t.Fatalf("linked root blocked: %+v", s)
		}
	}
	if err := os.Rename(target, filepath.Join(parent, "moved")); err != nil {
		t.Fatal(err)
	}
	states, err = CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Agent == "codex" && s.Status != "unverified" {
			t.Fatalf("broken root silently omitted: %+v", s)
		}
	}
}

func TestPromptTemplateCodexUnrelatedLargeAssetDoesNotBlockNativeInventory(t *testing.T) {
	h, log := promptEnabledCodexFixture(t)
	path := filepath.Join(h, ".codex/plugins/oversized.bin")
	write(t, path, "")
	if err := os.Truncate(path, 1<<30+1); err != nil {
		t.Fatal(err)
	}
	states, err := CheckPrompt(PromptChange{Name: "audit-code", Target: "codex", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Agent != "codex" || states[0].Status != "available" {
		t.Fatalf("an unrelated binary asset must not block native skill inventory: %+v", states)
	}
	if got := strings.Fields(read(t, log)); len(got) != 1 || got[0] != "app-server" {
		t.Fatalf("unrelated asset must permit exactly one native enumeration: %v", got)
	}
}

func TestPromptTemplateDSHRejectsUnsafeMappingBeforeStarting(t *testing.T) {
	dir := t.TempDir()
	copy := t.TempDir()
	write(t, filepath.Join(dir, "profiles/web/package.json"), `{"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base"]}}}`)
	write(t, filepath.Join(dir, "profiles/web/cordis.patch.yml"), "- id: storage\n  config:\n    storagePath: E:/original-data\n")
	if err := dshPromptCopyProfile(dir, copy); err == nil {
		t.Fatal("original storage mapping copied without isolation")
	}
}

func TestPromptTemplateDSHSettingsImportCannotEscapeIsolation(t *testing.T) {
	dir, copy := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dir, "profiles/web/package.json"), `{}`)
	write(t, filepath.Join(dir, "settings.yaml"), "storagePath: E:/original-data\n")
	if err := dshPromptCopyProfile(dir, copy); err == nil {
		t.Fatal("legacy settings would be silently omitted or imported from original storage")
	}
}

func TestPromptTemplateDSHEnvironmentLayerCannotBeSilentlyOmitted(t *testing.T) {
	dir, copy := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dir, "profiles/web/package.json"), `{}`)
	write(t, filepath.Join(dir, ".env"), "MAGPIE_FIXTURE_KEY=fixture-only\n")
	if err := dshPromptCopyProfile(dir, copy); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(copy, ".env")) != "MAGPIE_FIXTURE_KEY=fixture-only\n" {
		t.Fatal("native home environment layer was omitted from verification")
	}
	a, _ := os.Stat(filepath.Join(dir, ".env"))
	b, _ := os.Stat(filepath.Join(copy, ".env"))
	if os.SameFile(a, b) {
		t.Fatal("temporary environment shares the original file")
	}
}

func TestPromptTemplatePiSourceBytesInvalidateEvidence(t *testing.T) {
	h := promptCheckedPiFixture(t)
	path := filepath.Join(h, ".pi/agent/extensions/observer.ts")
	// The fixture's extension is intentionally source-independent to the mock
	// protocol. This assertion tests evidence invalidation, not its fake output.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) == 0 {
		t.Fatal("missing fixture extension", err)
	}
	path = filepath.Join(filepath.Dir(path), entries[0].Name())
	info, _ := os.Stat(path)
	b, _ := os.ReadFile(path)
	if len(b) == 0 {
		t.Fatal("empty fixture extension")
	}
	b[0] ^= 1
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	states, err := CheckPrompt(PromptChange{Name: "probe", Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Agent == "pi" && s.Status != "unverified" {
			t.Fatalf("same-sized source edit revived evidence: %+v", s)
		}
	}
}

func TestPromptTemplatePiNativeAbsoluteShimFindsOriginalCatalog(t *testing.T) {
	h, _ := promptEnabledPiFixture(t)
	target, _ := promptTargetByID(promptTargets(), "pi")
	root := filepath.Join(target.dir, "install/releases/test/node_modules/@earendil-works/pi-coding-agent")
	// Resolve the fixture release using its existing selector, then remove that
	// selector so only the shim's actual executable path supplies provenance.
	bin := filepath.Join(h, "bin/pi.exe")
	original, err := piInstalledPackage(target, bin)
	if err != nil {
		t.Fatal(err)
	}
	root = original
	os.Remove(filepath.Join(target.dir, "install/current-version"))
	shim := filepath.Join(h, "bin/pi.cmd")
	write(t, shim, "@echo off\r\nnode \""+filepath.Join(root, "dist/cli.js")+"\" %*\r\n")
	got, err := piInstalledPackage(target, shim)
	if err != nil || !promptSamePath(got, root) {
		t.Fatalf("native entry catalogue not found: %s %v", got, err)
	}
}

func TestPromptTemplateCodexLegacyOwnershipSurvivesUnverifiedHandoff(t *testing.T) {
	h, _ := promptEnabledCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "shared-audit"}); err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "shared-audit", Text: "Body", Agents: []string{"codex-desktop"}}))
	l, err := load()
	if err != nil {
		t.Fatal(err)
	}
	p := l.Prompts[0]
	l.applied("codex-desktop").Prompts = l.Applied["codex"].Prompts
	l.Applied["codex"].Prompts = nil
	write(t, filepath.Join(h, ".codex/config.toml"), "# changed after checking\n")
	// Existing selection from older data; CLI has not yet been checked and
	// therefore cannot obtain a new deployment record during this sync.
	p.Agents = []string{"codex"}
	l.syncPrompts(newBackups(), &Result{})
	path := filepath.Join(h, ".codex/skills/shared-audit/SKILL.md")
	if l.promptOwnedHash("codex", p.ID, path) == "" {
		t.Fatal("shared handoff lost legacy ownership while CLI is pending")
	}
	p.Agents = nil
	l.syncPrompts(newBackups(), &Result{})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("last consumer cannot clean legacy deployment after handoff", err)
	}
}

func TestPromptTemplatePhysicalDirectoryIdentity(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows physical alias identity")
	}
	real := t.TempDir()
	alias := filepath.Join(t.TempDir(), "different-name")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if !promptSamePath(real, alias) {
		t.Fatal("one physical native context counted twice")
	}
}

func TestPromptTemplatePiEvidenceCannotReviveAfterSourceRestored(t *testing.T) {
	h := promptCheckedPiFixture(t)
	target, _ := promptTargetByID(promptTargets(), "pi")
	l, err := load()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h, ".pi/agent/settings.json")
	original, _ := os.ReadFile(path)
	write(t, path, `{"different":true}`)
	if _, err := l.piCommandConflict(target, &Prompt{Name: "probe"}); err == nil {
		t.Fatal("changed source accepted")
	}
	write(t, path, string(original))
	if _, err := l.piCommandConflict(target, &Prompt{Name: "probe"}); err == nil {
		t.Fatal("restoring sources revived invalidated evidence")
	}
}

func TestPromptTemplateSharedUnreadableBodyKeepsOwnership(t *testing.T) {
	h, _ := promptEnabledCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "shared-body"}); err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "shared-body", Text: "Body", Agents: []string{"codex-desktop"}}))
	l, _ := load()
	p := l.Prompts[0]
	l.applied("codex-desktop").Prompts = l.Applied["codex"].Prompts
	l.Applied["codex"].Prompts = nil
	p.Agents = []string{"codex"}
	os.Remove(promptPath(p.ID))
	l.syncPrompts(newBackups(), &Result{})
	path := filepath.Join(h, ".codex/skills/shared-body/SKILL.md")
	if l.promptOwnedHash("codex-desktop", p.ID, path) == "" {
		t.Fatal("unreadable template lost protected ownership")
	}
	write(t, promptPath(p.ID), "Body")
	p.Agents = nil
	l.syncPrompts(newBackups(), &Result{})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("ownership could not retire recovered template", err)
	}
}

func TestPromptTemplateNativeSkillUsesRegisteredNameOnly(t *testing.T) {
	h, _ := promptEnabledCodexFixture(t)
	l, _ := load()
	root := filepath.Join(h, "native-skills")
	write(t, filepath.Join(root, "folder-alias/SKILL.md"), "---\nname: actual-name\ndescription: fixture\n---\nBody")
	for _, id := range []string{"codex", "dsh"} {
		hit, err := l.scanPromptSource(id, &Prompt{Name: "folder-alias"}, promptSource{path: root, skills: true})
		if err != nil || hit != "" {
			t.Fatalf("%s invented folder alias: %s %v", id, hit, err)
		}
		hit, err = l.scanPromptSource(id, &Prompt{Name: "actual-name"}, promptSource{path: root, skills: true})
		if err != nil || hit == "" {
			t.Fatalf("%s missed registered name", id)
		}
	}
}

func TestPromptTemplateCodexSkillDoesNotInventProjectCommandNamespace(t *testing.T) {
	h, _ := promptEnabledCodexFixture(t)
	l, _ := load()
	project := filepath.Join(h, "project")
	write(t, filepath.Join(project, ".codex/commands/audit-code.md"), "not a native Codex skill")
	l.Projects = []*Project{{Dir: project}}
	target, _ := promptTargetByID(promptTargets(), "codex")
	l.checkNativePromptCommands(context.Background(), target)
	hit, err := l.promptConflict(target, &Prompt{Name: "audit-code"})
	if err != nil || hit != "" {
		t.Fatalf("Codex $skill acquired an unsupported project command alias: %s %v", hit, err)
	}
}

func TestPromptTemplateCodexAuthorizationCannotCertifyInventory(t *testing.T) {
	h, _ := promptEnabledCodexFixture(t)
	write(t, filepath.Join(h, "codex-mode"), "approval")
	target, _ := promptTargetByID(promptTargets(), "codex")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bin := filepath.Join(h, "bin", "codex")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if _, err := readCodexPromptSkills(ctx, bin, target.dir, []string{h}); err == nil {
		t.Fatal("authorization request followed by a list was accepted as complete")
	}
}

func TestPromptTemplateCodexNativeDisabledManagedSkillStaysBlocked(t *testing.T) {
	h, _ := promptEnabledCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "audit-code"}); err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Body", Agents: []string{"codex"}}))
	v, _ := ReadPrompts()
	id := v.Items[0].ID
	path := filepath.Join(h, ".codex/skills/audit-code/SKILL.md")
	before := read(t, path)
	b, _ := json.Marshal([]map[string]any{{"name": "audit-code", "path": path, "enabled": false}})
	write(t, filepath.Join(h, "codex-skills.json"), string(b))
	states, err := CheckPrompt(PromptChange{ID: id, Name: "audit-code", Text: "Body", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Agent == "codex" && (promptUsable(s) || s.Reason != "This skill is disabled in Codex configuration") {
			t.Fatalf("native disabled managed skill reported usable: %+v", s)
		}
	}
	res, err := Sync()
	if err != nil || len(res.Problems) == 0 {
		t.Fatalf("disabled deployment was not reported: %+v %v", res, err)
	}
	if read(t, path) != before {
		t.Fatal("disabled skill check removed an existing protected deployment")
	}
}

func TestPromptTemplateDSHRejectsUnmappedFileAndDynamicSources(t *testing.T) {
	for _, config := range []string{
		"file: relative-state.json",
		"target: 'E:/original-data/settings.json'",
		"target: 'file:///E:/original-data/settings.json'",
		"target: !env USERPROFILE",
	} {
		t.Run(config, func(t *testing.T) {
			dir, copy := t.TempDir(), t.TempDir()
			write(t, filepath.Join(dir, "profiles/web/package.json"), `{}`)
			write(t, filepath.Join(dir, "cordis.patch.yml"), "- id: original-plugin\n  config:\n    "+config+"\n")
			if err := dshPromptCopyProfile(dir, copy); err == nil {
				t.Fatal("unmapped file/dynamic source could reach original configuration")
			}
		})
	}
}
