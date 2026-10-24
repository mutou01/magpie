package library

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPromptTemplateCodexWarningExceptionsFailClosedAtTheirBoundary(t *testing.T) {
	legacy := "Codex is ignoring 1 unrecognized configuration setting. Check for typos or deprecated settings.\n  user (C:\\fixture\\config.toml): `disable_response_storage` is ignored."
	for _, mode := range []string{"unknown-key", "mixed-keys", "non-null-details", "warning-request", "wrong-stderr-target", "different-stderr-text", "missing-metadata-source", "non-file-metadata-source", "relative-metadata-source"} {
		t.Run(mode, func(t *testing.T) {
			h, _ := promptCodexFixture(t)
			summary := strings.Replace(legacy, `C:\fixture\config.toml`, filepath.Join(h, ".codex/config.toml"), 1)
			params := map[string]any{"summary": summary, "details": nil}
			warning := map[string]any{"method": "configWarning", "params": params}
			diagnostic := map[string]any{"level": "ERROR", "target": "codex_app_server", "fields": map[string]any{"message": summary}}
			switch mode {
			case "unknown-key":
				params["summary"] = strings.ReplaceAll(summary, "disable_response_storage", "unknown_skill_loading_setting")
			case "mixed-keys":
				params["summary"] = strings.ReplaceAll(summary, "1 unrecognized", "2 unrecognized") + "\n  user (C:\\fixture\\config.toml): `unknown_skill_loading_setting` is ignored."
			case "non-null-details":
				params["details"] = "PRIVATE_UNVERIFIED_DIAGNOSTIC"
			case "warning-request":
				warning["id"] = 42
			case "wrong-stderr-target":
				diagnostic["target"] = "other_native_loader"
			case "different-stderr-text":
				diagnostic["fields"] = map[string]any{"message": "PRIVATE_UNVERIFIED_DIAGNOSTIC"}
			default:
				warning = nil
				location := "file:///C:/missing/agents/openai.yaml"
				if mode == "non-file-metadata-source" {
					location = "https://example.invalid/agents/openai.yaml"
				} else if mode == "relative-metadata-source" {
					location = "agents/openai.yaml"
				}
				diagnostic = map[string]any{"level": "WARN", "target": "codex_skills_extension::loader::metadata", "fields": map[string]any{"message": "ignoring " + location + ": invalid openai.yaml: found character that cannot start any token"}}
			}
			if warning != nil {
				b, _ := json.Marshal(warning)
				write(t, filepath.Join(h, "codex-notification.json"), string(b))
			}
			b, _ := json.Marshal(diagnostic)
			write(t, filepath.Join(h, "codex-diagnostic.json"), string(b))
			states, err := CheckPrompt(PromptChange{Name: "audit-code"})
			if err != nil {
				t.Fatal(err)
			}
			for _, state := range states {
				if state.Agent == "codex" {
					if state.Status != "unverified" || strings.Contains(state.Reason+state.Source, "PRIVATE_") {
						t.Fatalf("unverified warning must remain blocked without exposing diagnostics: %+v", state)
					}
					return
				}
			}
			t.Fatal("Codex target missing")
		})
	}
}

func TestPromptTemplateCodexForeignMetadataURIKeepsEncodedPathIdentity(t *testing.T) {
	h, _ := promptCodexFixture(t)
	foreign := filepath.Join(h, ".agents/skills/foreign skill 中文/SKILL.md")
	write(t, foreign, "---\nname: foreign-native-name\ndescription: Native path identity\n---\nBody\n")
	policy := filepath.Join(filepath.Dir(foreign), "agents/openai.yaml")
	write(t, policy, "policy:\n\tallow_implicit_invocation: false\n")
	uriPath := filepath.ToSlash(policy)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := (&url.URL{Scheme: "file", Path: uriPath}).String()
	rows, _ := json.Marshal([]map[string]any{{"name": "foreign-native-name", "path": foreign, "enabled": true}})
	write(t, filepath.Join(h, "codex-skills.json"), string(rows))
	diagnostic, _ := json.Marshal(map[string]any{"level": "WARN", "target": "codex_skills_extension::loader::metadata", "fields": map[string]any{"message": "ignoring " + uri + ": invalid openai.yaml: found character that cannot start any token"}})
	write(t, filepath.Join(h, "codex-diagnostic.json"), string(diagnostic))
	states, err := CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "codex" {
			if state.Status != "available" || state.Reason == "" || state.Source == "" {
				t.Fatalf("encoded native file URI must bind to the listed foreign skill: %+v", state)
			}
			return
		}
	}
	t.Fatal("Codex target missing")
}

func TestPromptTemplateCodexEditedOwnPolicyRemainsUnavailable(t *testing.T) {
	h, _ := promptCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "audit-code"}); err != nil {
		t.Fatal(err)
	}
	result, err := SavePrompt(PromptChange{Name: "audit-code", Text: "Explicit body", Agents: []string{"codex"}})
	if err != nil || len(result.Problems) != 0 {
		t.Fatalf("deploy valid template: %+v %v", result, err)
	}
	view, err := ReadPrompts()
	if err != nil || len(view.Items) != 1 {
		t.Fatalf("read deployed template: %+v %v", view, err)
	}
	policy := filepath.Join(h, ".codex/skills/audit-code/agents/openai.yaml")
	broken := "policy:\n\tallow_implicit_invocation: false\n"
	write(t, policy, broken)
	states, err := CheckPrompt(PromptChange{ID: view.Items[0].ID, Name: "audit-code", Text: "Explicit body"})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "codex" {
			if promptUsable(state) || state.Status != "edited" || !promptSamePath(state.Source, policy) {
				t.Fatalf("malformed own policy must remain unavailable and protected: %+v", state)
			}
			if read(t, policy) != broken {
				t.Fatal("checking own malformed policy rewrote it")
			}
			return
		}
	}
	t.Fatal("Codex target missing")
}

func TestPromptTemplateCodexForeignMetadataFailurePreservesIndependentAvailability(t *testing.T) {
	h, _ := promptCodexFixture(t)
	foreign := filepath.Join(h, ".agents/skills/foreign-audit/SKILL.md")
	write(t, foreign, "---\nname: foreign-audit\ndescription: Foreign skill with native default policy\n---\nForeign body\n")
	policy := filepath.Join(filepath.Dir(foreign), "agents/openai.yaml")
	write(t, policy, "policy:\n\tallow_implicit_invocation: false\n")
	rows, _ := json.Marshal([]map[string]any{{"name": "foreign-audit", "path": foreign, "enabled": true}})
	write(t, filepath.Join(h, "codex-skills.json"), string(rows))
	write(t, filepath.Join(h, "codex-mode"), "foreign-metadata-warning")
	states, err := CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent != "codex" {
			continue
		}
		if state.Status != "available" || state.Reason == "" || state.Source == "" {
			t.Fatalf("fully listed foreign policy failure must retain independent template availability with a diagnostic: %+v", state)
		}
		occupied, err := CheckPrompt(PromptChange{Name: "foreign-audit"})
		if err != nil {
			t.Fatal(err)
		}
		for _, collision := range occupied {
			if collision.Agent == "codex" && collision.Status == "conflict" && promptSamePath(collision.Source, foreign) {
				return
			}
		}
		t.Fatalf("native foreign name must remain reserved: %+v", occupied)
	}
	t.Fatal("Codex target missing")
}

func TestPromptTemplateCodexIgnoredLegacySettingKeepsCompleteInventoryUsable(t *testing.T) {
	h, _ := promptCodexFixture(t)
	write(t, filepath.Join(h, "codex-mode"), "ignored-legacy-config")
	states, err := CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "codex" {
			if state.Status != "available" {
				t.Fatalf("ignored deprecated setting must not block a complete native inventory: %+v", state)
			}
			return
		}
	}
	t.Fatal("Codex target missing")
}

// A normal package installed in the profile uses the native installation's
// peer fallback. Linking its whole parent directory changes that native scope.
func TestPromptTemplateOriginalDSHProfileLocalPeerFallback(t *testing.T) {
	dir, node := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN")
	if runtime.GOOS != "windows" || dir == "" || node == "" {
		t.Skip("opt-in original Windows DSH profile dependency resolution")
	}
	h := sandbox(t)
	t.Setenv("PATH", filepath.Join(h, "bin")+string(os.PathListSeparator)+filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	isolated := t.TempDir()
	write(t, filepath.Join(isolated, "profiles/web/package.json"), `{"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base","@deepseek-ai/dsh-web-app"]}}}`)
	if err := os.Symlink(filepath.Join(dir, "profiles/node_modules"), filepath.Join(isolated, "profiles/node_modules")); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(isolated, "profiles/web/node_modules/magpie-profile-peer-fixture")
	write(t, filepath.Join(plugin, "package.json"), `{"name":"magpie-profile-peer-fixture","version":"1.0.0","type":"module","main":"index.mjs","peerDependencies":{"@deepseek-ai/dsh-settings":"*"}}`)
	write(t, filepath.Join(plugin, "index.mjs"), "import '@deepseek-ai/dsh-settings';\nexport function apply() {}\n")
	write(t, filepath.Join(isolated, "cordis.patch.yml"), "- insert:\n    - id: profile-peer-fixture\n      name: magpie-profile-peer-fixture\n")
	t.Setenv("DSH_HOME", isolated)
	assertAvailable := func(passive bool) {
		t.Helper()
		states, err := CheckPrompt(PromptChange{Name: "audit-code", Passive: passive})
		if err != nil {
			t.Fatal(err)
		}
		for _, state := range states {
			if state.Agent == "dsh" {
				if state.Status != "available" {
					t.Fatalf("native profile peer fallback must preserve template availability (passive=%v): %+v", passive, state)
				}
				return
			}
		}
		t.Fatal("original DSH was not detected")
	}
	assertAvailable(false)
	// Both discovery passes and subsequent passive reads must fingerprint the
	// original resources, never already deleted random temporary module copies.
	assertAvailable(true)
	assertAvailable(false)
	result, err := SavePrompt(PromptChange{Name: "audit-code", Text: "PROFILE_LOCAL_PEER_BODY", Agents: []string{"dsh"}})
	if err != nil || len(result.Problems) != 0 {
		t.Fatalf("native profile peer fallback did not permit public deployment: %+v %v", result, err)
	}
	if !strings.Contains(read(t, filepath.Join(isolated, "skills/audit-code/SKILL.md")), "PROFILE_LOCAL_PEER_BODY") {
		t.Fatal("public save omitted the template body")
	}
}

func TestPromptTemplateOriginalDSHProfileCompatibilityGrantPreservesCommand(t *testing.T) {
	dir, node := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN")
	if runtime.GOOS != "windows" || dir == "" || node == "" {
		t.Skip("opt-in original Windows DSH profile compatibility permissions")
	}
	h := sandbox(t)
	t.Setenv("PATH", filepath.Join(h, "bin")+string(os.PathListSeparator)+filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	isolated := t.TempDir()
	write(t, filepath.Join(isolated, "profiles/web/package.json"), `{"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base","@deepseek-ai/dsh-web-app"]}}}`)
	if err := os.Symlink(filepath.Join(dir, "profiles/node_modules"), filepath.Join(isolated, "profiles/node_modules")); err != nil {
		t.Fatal(err)
	}
	var native struct{ Version string }
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, "profiles/node_modules/@deepseek-ai/dsh-app-boot/package.json"))), &native); err != nil || native.Version == "" {
		t.Fatalf("read native runtime identity: %v", err)
	}
	plugin := filepath.Join(isolated, "profiles/web/node_modules/magpie-compatibility-fixture")
	write(t, filepath.Join(plugin, "package.json"), `{"name":"magpie-compatibility-fixture","version":"1.0.0","type":"module","main":"index.mjs","engines":{"dsh":"<0.0.0"}}`)
	write(t, filepath.Join(plugin, "index.mjs"), "export const inject=['commands'];\nexport function apply(ctx) { ctx.commands.register({name:'grant-command',description:'Native profile grant observation',handler:()=>({})}); }\n")
	write(t, filepath.Join(isolated, "cordis.patch.yml"), "- insert:\n    - id: compatibility-fixture\n      name: magpie-compatibility-fixture\n")
	grant, _ := json.Marshal(map[string][]string{"magpie-compatibility-fixture@1.0.0": {native.Version}})
	write(t, filepath.Join(isolated, "profiles/web/compatibility.json"), string(grant))
	t.Setenv("DSH_HOME", isolated)
	states, err := CheckPrompt(PromptChange{Name: "grant-command"})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "dsh" {
			if state.Status != "conflict" || !strings.Contains(state.Source, "grant-command") {
				t.Fatalf("native profile compatibility grant must preserve its active command reservation: %+v", state)
			}
			return
		}
	}
	t.Fatal("original DSH was not detected")
}
