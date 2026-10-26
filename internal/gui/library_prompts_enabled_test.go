package gui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/library"
)

func TestPromptTemplatesAPIEnabledRequiresExplicitBoolean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", "")
	mux := http.NewServeMux()
	libraryRoutes(mux, nil)
	for _, body := range []string{`{}`, `{"enabled":null}`} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/library/prompts/enabled", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("missing explicit enabled boolean must be rejected: HTTP %d", rec.Code)
		}
		view, err := library.ReadPrompts()
		if err != nil || view.Enabled || len(view.Items) != 0 {
			t.Fatalf("rejected request changed the empty library: %+v %v", view, err)
		}
	}
}

func TestPromptTemplatesAPIClosingSwitchRetiresManagedFileKeepsTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", "")
	const id, body = "legacy-template", "Keep the saved template body"
	managed := filepath.Join(home, ".claude", "commands", "audit-code.md")
	const deployed = "Previously deployed managed template"
	put := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put(managed, deployed)
	put(filepath.Join(library.Dir(), "prompts", id+".md"), body)
	hash := sha256.Sum256([]byte(deployed))
	index, err := json.Marshal(map[string]any{
		"version": 1, "enabled": true,
		"prompts": []any{map[string]any{"id": id, "name": "audit-code", "agents": []string{"claude"}}},
		"applied": map[string]any{"claude": map[string]any{id: map[string]any{"files": map[string]string{managed: hex.EncodeToString(hash[:])}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	put(filepath.Join(library.Dir(), "prompts.json"), string(index))
	mux := http.NewServeMux()
	libraryRoutes(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/library/prompts/enabled", strings.NewReader(`{"enabled":false}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("closing the prompt switch must be a supported public operation: HTTP %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Part    string               `json:"part"`
		Prompts *library.PromptsView `json:"prompts"`
		Result  *library.Result      `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Part != "prompts" || response.Prompts == nil || response.Prompts.Enabled || response.Result == nil {
		t.Fatalf("closing response omitted the disabled prompt section: %s", rec.Body.String())
	}
	if len(response.Prompts.Items) != 1 || response.Prompts.Items[0].Text != body || len(response.Prompts.Items[0].Agents) != 1 || response.Prompts.Items[0].Agents[0] != "claude" {
		t.Fatalf("closing discarded the saved body or Harness selection: %+v", response.Prompts.Items)
	}
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Fatal("closing did not retire the unchanged managed deployment", err)
	}
}

func TestPromptTemplatesAPIClosingSwitchPreservesEntireExternallyEditedCodexTemplate(t *testing.T) {
	for _, edited := range []string{"body", "policy"} {
		t.Run(edited, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
			t.Setenv("PATH", "")
			const id, body = "legacy-template", "Saved explicit-only template body"
			root := filepath.Join(home, ".codex", "skills", "audit-code")
			bodyPath, policyPath := filepath.Join(root, "SKILL.md"), filepath.Join(root, "agents", "openai.yaml")
			files := map[string]string{
				bodyPath:   "---\nname: audit-code\ndescription: Explicit-only fixture\n---\n\n" + body + "\n",
				policyPath: "policy:\n  allow_implicit_invocation: false\n",
			}
			put := func(path, content string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			hashes := map[string]string{}
			for path, content := range files {
				put(path, content)
				hash := sha256.Sum256([]byte(content))
				hashes[path] = hex.EncodeToString(hash[:])
			}
			index, err := json.Marshal(map[string]any{
				"version": 1, "enabled": true,
				"prompts": []any{map[string]any{"id": id, "name": "audit-code", "agents": []string{"codex"}}},
				"applied": map[string]any{"codex": map[string]any{id: map[string]any{"files": hashes}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			put(filepath.Join(library.Dir(), "prompts.json"), string(index))
			put(filepath.Join(library.Dir(), "prompts", id+".md"), body)
			if edited == "body" {
				files[bodyPath] += "User-authored external edit\n"
			} else {
				files[policyPath] += "# User-authored external edit\n"
			}
			for path, content := range files {
				put(path, content)
			}
			mux := http.NewServeMux()
			libraryRoutes(mux, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/library/prompts/enabled", strings.NewReader(`{"enabled":false}`)))
			if rec.Code != http.StatusOK {
				t.Fatalf("safe partial close failed: HTTP %d %s", rec.Code, rec.Body.String())
			}
			var response struct {
				Prompts *library.PromptsView `json:"prompts"`
				Result  *library.Result      `json:"result"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Prompts == nil || response.Prompts.Enabled || len(response.Prompts.Items) != 1 || response.Prompts.Items[0].Text != body || len(response.Prompts.Items[0].Agents) != 1 || response.Prompts.Items[0].Agents[0] != "codex" {
				t.Fatalf("close lost disabled state, saved body or selection: %s", rec.Body.String())
			}
			if response.Result == nil || len(response.Result.Problems) == 0 {
				t.Error("closing an externally edited template must explain why it was retained")
			}
			for path, want := range files {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Errorf("editing %s must retain both the body and invocation policy unchanged; lost %s: %v", edited, filepath.Base(path), err)
				}
			}
		})
	}
}
