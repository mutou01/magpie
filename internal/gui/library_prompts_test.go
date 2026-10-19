package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptTemplateAPISavesAndReads(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("PATH", "")
	mux := http.NewServeMux()
	libraryRoutes(mux, nil)
	r := httptest.NewRecorder()
	mux.ServeHTTP(r, httptest.NewRequest("POST", "/api/library/prompts/save", strings.NewReader(`{"name":"代码审查","text":"检查边界条件。","agents":[]}`)))
	if r.Code != http.StatusOK {
		t.Fatalf("save prompt: %d %s", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	mux.ServeHTTP(r, httptest.NewRequest("GET", "/api/library/prompts", nil))
	var v struct {
		Items []struct{ ID, Name, Text string } `json:"items"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Items) != 1 || v.Items[0].Name != "代码审查" || v.Items[0].Text != "检查边界条件。" || v.Items[0].ID == "" {
		t.Fatalf("saved template missing: %d %s", r.Code, r.Body.String())
	}
	id := v.Items[0].ID
	// Reproduce the output of a version without template support. Both the
	// dedicated refresh and the initial Library page must keep the template.
	if err := os.WriteFile(filepath.Join(h, ".config/magpie/library.json"), []byte(`{"instructions":{"agents":[]},"mcp":[],"skills":[],"applied":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"/api/library/prompts", "/api/library?glance=1"} {
		r = httptest.NewRecorder()
		mux.ServeHTTP(r, httptest.NewRequest("GET", url, nil))
		if r.Code != http.StatusOK {
			t.Fatalf("read after legacy rewrite: %s: %d %s", url, r.Code, r.Body.String())
		}
		data := r.Body.Bytes()
		if strings.Contains(url, "glance") {
			var lib struct{ Prompts json.RawMessage }
			if err := json.Unmarshal(data, &lib); err != nil {
				t.Fatal(err)
			}
			data = lib.Prompts
		}
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		if len(v.Items) != 1 || v.Items[0].ID != id || v.Items[0].Text != "检查边界条件。" {
			t.Fatalf("API lost template after legacy rewrite: %s: %s", url, data)
		}
	}
}
