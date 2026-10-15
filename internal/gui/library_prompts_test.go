package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
}
