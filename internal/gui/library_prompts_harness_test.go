package gui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/library"
)

// Exercise the public mutation, including persisted selection and file ownership.
func TestPromptTemplatesAPIHarnessDisableCurrentTemplates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("PATH", "")
	put := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var prompts []any
	applied := map[string]any{}
	paths := map[string]string{}
	for _, id := range []string{"first", "second"} {
		path := filepath.Join(home, ".claude", "commands", id+".md")
		content := "Managed deployment " + id
		put(path, content)
		put(filepath.Join(library.Dir(), "prompts", id+".md"), "Saved body "+id)
		hash := sha256.Sum256([]byte(content))
		applied[id] = map[string]any{"files": map[string]string{path: hex.EncodeToString(hash[:])}}
		prompts = append(prompts, map[string]any{"id": id, "name": id, "agents": []string{"claude", "pi"}})
		paths[id] = path
	}
	index, err := json.Marshal(map[string]any{"version": 1, "enabled": true, "prompts": prompts, "applied": map[string]any{"claude": applied}})
	if err != nil {
		t.Fatal(err)
	}
	put(filepath.Join(library.Dir(), "prompts.json"), string(index))
	mux := http.NewServeMux()
	libraryRoutes(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/library/prompts/harness", strings.NewReader(`{"target":"claude","enabled":false}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("batch Harness disable must be supported: HTTP %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Part    string               `json:"part"`
		Prompts *library.PromptsView `json:"prompts"`
		Result  *library.Result      `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Part != "prompts" || response.Prompts == nil || response.Result == nil || len(response.Result.Problems) != 0 {
		t.Fatalf("batch disable response is incomplete: %s", rec.Body.String())
	}
	view, err := library.ReadPrompts()
	if err != nil || !view.Enabled || len(view.Items) != 2 {
		t.Fatalf("batch disable changed the global switch or lost templates: %+v %v", view, err)
	}
	for _, item := range view.Items {
		if item.Text != "Saved body "+item.ID || !slices.Equal(item.Agents, []string{"pi"}) {
			t.Fatalf("batch disable lost body or another Harness selection: %+v", item)
		}
		if _, err := os.Stat(paths[item.ID]); !os.IsNotExist(err) {
			t.Fatalf("unchanged owned deployment was not retired: %s %v", item.ID, err)
		}
	}
}

func TestPromptTemplatesAPIHarnessEnablePartialAndRepairSelected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	put := func(path string, content []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0700); err != nil {
			t.Fatal(err)
		}
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	name := "claude"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	put(filepath.Join(home, "bin", name), bin)
	t.Setenv("PATH", filepath.Join(home, "bin"))
	put(filepath.Join(home, ".claude", "settings.json"), []byte(`{}`))
	put(filepath.Join(home, ".claude", "commands", "occupied.md"), []byte("User command"))
	var prompts []any
	for _, id := range []string{"new-command", "repair-selected", "occupied"} {
		agents := []string{"pi"}
		if id == "repair-selected" {
			agents = append(agents, "claude")
		}
		prompts = append(prompts, map[string]any{"id": id, "name": id, "agents": agents})
		put(filepath.Join(library.Dir(), "prompts", id+".md"), []byte("Saved body "+id))
	}
	index, err := json.Marshal(map[string]any{"version": 1, "enabled": true, "prompts": prompts, "applied": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	put(filepath.Join(library.Dir(), "prompts.json"), index)
	mux := http.NewServeMux()
	libraryRoutes(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/library/prompts/harness", strings.NewReader(`{"target":"claude","enabled":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("batch enable must return per-template results: HTTP %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Result *library.Result `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Result == nil || len(response.Result.Problems) != 1 || response.Result.Problems[0].Agent != "claude" || !strings.Contains(response.Result.Problems[0].What, "occupied") {
		t.Fatalf("expected exactly the occupied template problem: %s", rec.Body.String())
	}
	view, err := library.ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range view.Items {
		want := []string{"claude", "pi"}
		if item.ID == "occupied" {
			want = []string{"pi"}
		}
		if item.Text != "Saved body "+item.ID || !slices.Equal(item.Agents, want) {
			t.Fatalf("wrong partial selection/body: %+v", item)
		}
		body, err := os.ReadFile(filepath.Join(home, ".claude", "commands", item.ID+".md"))
		if err != nil {
			t.Fatal(err)
		}
		if item.ID == "occupied" {
			if string(body) != "User command" {
				t.Fatal("conflicting command overwritten")
			}
		} else if !strings.Contains(string(body), "Saved body "+item.ID) {
			t.Fatalf("selected template not deployed: %s", item.ID)
		}
	}
}

func TestPromptTemplatesAPIHarnessRejectsInvalidRequests(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", "")
	mux := http.NewServeMux()
	libraryRoutes(mux, nil)
	for _, body := range []string{
		`{}`, `{"target":"claude"}`, `{"target":"claude","enabled":null}`,
		`{"target":"claude","enabled":"false"}`, `{"target":"unknown","enabled":true}`,
		`{"target":"","enabled":false}`, `{"target":"claude","enabled":true}`,
		`{"target":"claude","enabled":false}`,
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/library/prompts/harness", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid/off request accepted: %s HTTP %d", body, rec.Code)
		}
		view, err := library.ReadPrompts()
		if err != nil || view.Enabled || len(view.Items) != 0 {
			t.Fatalf("rejected request mutated library: %+v %v", view, err)
		}
	}
}
