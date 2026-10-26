package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The import is the same failing declaration observed in the installed
// dsh-at-file client; only the test-owned plugin is installed in this profile.
func TestPromptTemplateOriginalDSHMissingExportDiagnostic(t *testing.T) {
	isolated, plugin := originalDSHExportFixture(t)
	write(t, filepath.Join(plugin, "index.mjs"), "import { settingsNamespace } from '@deepseek-ai/dsh-settings';\nexport function apply() { return settingsNamespace('fixture'); }\n")
	state := checkDSHExportFixture(t)
	if state.Status != "available" || state.Reason != "DeepSeek Harness plugin requires an export unavailable in the installed dependency" || state.Source != "magpie-missing-export-fixture: @deepseek-ai/dsh-settings (settingsNamespace)" {
		t.Fatalf("optional missing native export must preserve availability with an actionable safe warning: %+v", state)
	}
	if _, err := os.Stat(filepath.Join(isolated, "skills/audit-code/SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("availability check unexpectedly deployed a template", err)
	}
	// Repair only this test's plugin. A fresh public check must detect the
	// changed source and clear the warning without changing availability.
	write(t, filepath.Join(plugin, "index.mjs"), "export function apply() {}\n")
	state = checkDSHExportFixture(t)
	if state.Status != "available" || state.Reason != "" || state.Source != "" {
		t.Fatalf("repaired native plugin must restore availability: %+v", state)
	}
}

func TestPromptTemplateOriginalDSHUnsafeExportErrorsStayPrivate(t *testing.T) {
	for i, message := range []string{
		"PRIVATE_ARBITRARY_SYNTAX_ERROR",
		"The requested module '/PRIVATE_DEPENDENCY_PATH' does not provide an export named 'validExport'",
		"The requested module '@fixture/dependency' does not provide an export named 'PRIVATE_INVALID-EXPORT'",
		"The requested module '@fixture/dependency' does not provide an export named '" + strings.Repeat("x", 129) + "PRIVATE_LONG_EXPORT'",
		"The requested module '@fixture/dependency' does not provide an export named 'validExport' PRIVATE_TRAILING_PAYLOAD",
	} {
		t.Run(fmt.Sprintf("unsafe-%d", i), func(t *testing.T) {
			_, plugin := originalDSHExportFixture(t)
			encoded, _ := json.Marshal(message)
			write(t, filepath.Join(plugin, "index.mjs"), "throw new SyntaxError("+string(encoded)+");\n")
			state := checkDSHExportFixture(t)
			if state.Status != "unverified" || strings.Contains(state.Reason+state.Source, "PRIVATE_") || state.Reason == "DeepSeek Harness plugin requires an export unavailable in the installed dependency" {
				t.Fatalf("untrusted syntax error must remain blocked without disclosing its payload: %+v", state)
			}
		})
	}
}

func TestPromptTemplateOriginalDSHConfiguredMissingExportDiagnostic(t *testing.T) {
	dir, node, expected := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN"), os.Getenv("MAGPIE_VERIFY_DSH_EXPECT_EXPORT_DIAGNOSTIC")
	if runtime.GOOS != "windows" || dir == "" || node == "" || expected == "" {
		t.Skip("opt-in original configured DSH missing dependency export")
	}
	h := sandbox(t)
	t.Setenv("PATH", filepath.Join(h, "bin")+string(os.PathListSeparator)+filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	t.Setenv("DSH_HOME", dir)
	before := map[string]string{}
	for _, rel := range []string{".env", "cordis.patch.yml", "profiles/web/package.json", "profiles/web/cordis.patch.yml", "profiles/web/cordis.yml", "profiles/web/node_modules/dsh-at-file/lib/index.js"} {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		before[rel] = promptHash(b)
	}
	state := checkDSHExportFixture(t)
	if state.Status != "available" || state.Reason != "DeepSeek Harness plugin requires an export unavailable in the installed dependency" || state.Source != expected {
		t.Fatalf("configured optional missing export must preserve availability with its safe warning: %+v", state)
	}
	for rel, sum := range before {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil || promptHash(b) != sum {
			t.Fatalf("original configured resource changed: %s", rel)
		}
	}
	t.Log("Original configuration and failing plugin selected fingerprints unchanged; effective namespace remains available")
}

func originalDSHExportFixture(t *testing.T) (string, string) {
	t.Helper()
	dir, node := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN")
	if runtime.GOOS != "windows" || dir == "" || node == "" {
		t.Skip("opt-in original Windows DSH missing dependency export")
	}
	h := promptEnabledSandbox(t)
	t.Setenv("PATH", filepath.Join(h, "bin")+string(os.PathListSeparator)+filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	isolated := t.TempDir()
	write(t, filepath.Join(isolated, "profiles/web/package.json"), `{"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base","@deepseek-ai/dsh-web-app"]}}}`)
	if err := os.Symlink(filepath.Join(dir, "profiles/node_modules"), filepath.Join(isolated, "profiles/node_modules")); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(isolated, "profiles/web/node_modules/magpie-missing-export-fixture")
	write(t, filepath.Join(plugin, "package.json"), `{"name":"magpie-missing-export-fixture","version":"1.0.0","type":"module","main":"index.mjs","peerDependencies":{"@deepseek-ai/dsh-settings":"*"}}`)
	write(t, filepath.Join(isolated, "cordis.patch.yml"), "- insert:\n    - id: missing-export-fixture\n      name: magpie-missing-export-fixture\n")
	t.Setenv("DSH_HOME", isolated)
	return isolated, plugin
}

func checkDSHExportFixture(t *testing.T) PromptState {
	t.Helper()
	states, err := CheckPrompt(PromptChange{Name: "audit-code"})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent != "dsh" {
			continue
		}
		return state
	}
	t.Fatal("original DSH was not detected")
	return PromptState{}
}
