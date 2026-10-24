package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPromptTemplateCodexModelCatalogChangesInvalidateNativeEvidence(t *testing.T) {
	for _, reference := range []string{"relative", "absolute"} {
		for _, change := range []string{"content", "deleted"} {
			t.Run(reference+"/"+change, func(t *testing.T) {
				h, _ := promptCodexFixture(t)
				catalog := filepath.Join(h, ".codex/catalogs/models.json")
				value := "catalogs/models.json"
				if reference == "absolute" {
					catalog = filepath.Join(t.TempDir(), "models.json")
					value = filepath.ToSlash(catalog)
				}
				quoted, _ := json.Marshal(value)
				config := "model_catalog_json = " + string(quoted) + "\n"
				write(t, filepath.Join(h, ".codex/config.toml"), config)
				write(t, catalog, "{\"models\":[]}\n")
				check := func(passive bool, want string) {
					t.Helper()
					states, err := CheckPrompt(PromptChange{Name: "audit-code", Passive: passive})
					if err != nil {
						t.Fatal(err)
					}
					for _, state := range states {
						if state.Agent == "codex" {
							if state.Status != want {
								t.Fatalf("model_catalog_json source %s must invalidate native evidence without changing config.toml (passive=%v): %+v", change, passive, state)
							}
							return
						}
					}
					t.Fatal("Codex target missing")
				}
				check(false, "available")
				check(true, "available")
				if change == "deleted" {
					if err := os.Remove(catalog); err != nil {
						t.Fatal(err)
					}
				} else {
					write(t, catalog, "{\n  \"models\": []\n}\n")
				}
				if read(t, filepath.Join(h, ".codex/config.toml")) != config {
					t.Fatal("catalog mutation changed the native configuration")
				}
				check(true, "unverified")
			})
		}
	}
}
