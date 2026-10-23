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
	commands, err := readCodexPromptSkills(ctx, bin, dir, []string{h})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range commands {
		if c.Name == "audit-code" && promptSamePath(c.Path, path) && c.Enabled != nil && *c.Enabled {
			t.Log("Original app-server listed isolated explicit-only skill")
			return
		}
	}
	t.Fatalf("original CLI did not list fixture: %+v", commands)
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
	t.Setenv("PATH", filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	entry := dshPromptEntry(dir)
	if entry == "" {
		t.Fatal("original DSH not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _, err := readDSHPromptCommands(ctx, PromptTarget{ID: "dsh", dir: dir}, nativePromptSnapshot{bin: entry, dirs: []string{t.TempDir()}})
	if err == nil || !strings.Contains(err.Error(), "provider completeness") {
		t.Fatalf("native provider completeness boundary not reached: %v", err)
	}
	if strings.Contains(err.Error(), "unidentified plugin") || !strings.Contains(err.Error(), "@deepseek-ai/") {
		t.Fatalf("native inventory did not identify uncovered providers: %v", err)
	}
	t.Log(err)
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

func TestPromptTemplateCodexNativeInventoryAndSharedOwnership(t *testing.T) {
	h, log := promptCodexFixture(t)
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
	_, log := promptCodexFixture(t)
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
			_, log := promptCodexFixture(t)
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
	h, log := promptCodexFixture(t)
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
	_, _ = promptCodexFixture(t)
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
		states, err := CheckPrompt(PromptChange{Name: name, Passive: true})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, s := range states {
			if s.Agent == "dsh" {
				found = true
				if s.Status != "unsupported" {
					t.Fatalf("invalid native DSH name accepted: %+v", s)
				}
			}
		}
		if !found {
			t.Fatal("DSH target missing")
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
	h, log := promptCodexFixture(t)
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
	h, _ := promptPiFixture(t)
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
	h, _ := promptCodexFixture(t)
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
	h, _ := promptCodexFixture(t)
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
	h, _ := promptCodexFixture(t)
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
	h, _ := promptCodexFixture(t)
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
	h, _ := promptCodexFixture(t)
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
	h, _ := promptCodexFixture(t)
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
	states, err := CheckPrompt(PromptChange{ID: id, Name: "audit-code", Text: "Body"})
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
