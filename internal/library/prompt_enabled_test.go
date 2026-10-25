package library

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Older deployment tests explicitly opt into the enabled contract. The cold
// protocol fixtures remain untouched for default-off and activation tests.
func promptEnabledSandbox(t *testing.T) string {
	t.Helper()
	home := sandbox(t)
	write(t, promptStorePath(), `{"version":1,"enabled":true,"prompts":[],"applied":{}}`)
	return home
}

func promptEnabledPiFixture(t *testing.T) (string, string) {
	t.Helper()
	home, starts := promptPiFixture(t)
	write(t, promptStorePath(), `{"version":1,"enabled":true,"prompts":[],"applied":{}}`)
	return home, starts
}

func promptEnabledCodexFixture(t *testing.T) (string, string) {
	t.Helper()
	home, starts := promptCodexFixture(t)
	write(t, promptStorePath(), `{"version":1,"enabled":true,"prompts":[],"applied":{}}`)
	return home, starts
}

func TestPromptTemplatesEmptyLibraryStaysDisabledWithoutStartingHarness(t *testing.T) {
	home, starts := promptCodexFixture(t)
	// Sync also maintains MCP configuration. Isolate its unrelated version
	// discovery while retaining real public Sync and prompt process assertions.
	setPiVersion(t, "0.87.1")
	assertNotStarted := func(operation string) {
		t.Helper()
		for _, log := range []string{starts, os.Getenv("MAGPIE_FAKE_PI"), os.Getenv("MAGPIE_FAKE_PI") + ".version"} {
			if read(t, log) != "" {
				t.Fatalf("%s started a Harness: %s", operation, filepath.Base(log))
			}
		}
	}
	view, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	if view.Enabled || len(view.Items) != 0 {
		t.Fatalf("a new library must start disabled: %+v", view)
	}
	assertNotStarted("reading an empty disabled library")
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Keep this body", Agents: []string{"codex"}}))
	assertNotStarted("saving a disabled template")
	ok(t)(Sync())
	assertNotStarted("synchronizing a disabled template")
	view, err = ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	if view.Enabled || len(view.Items) != 1 || view.Items[0].Text != "Keep this body" || len(view.Items[0].Agents) != 1 || view.Items[0].Agents[0] != "codex" {
		t.Fatalf("save/sync must preserve the disabled template and selection: %+v", view)
	}
	assertNotStarted("reading a saved disabled template")
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "audit-code", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("saving while disabled deployed the template", err)
	}
}

func TestPromptTemplatesOpeningSwitchChecksAllDetectedHarnessesIndependently(t *testing.T) {
	home, codexStarts := promptCodexFixture(t)
	piStarts := os.Getenv("MAGPIE_FAKE_PI")
	write(t, filepath.Join(home, "codex-mode"), "metadata-warning")
	view, err := ReadPrompts()
	if err != nil || view.Enabled {
		t.Fatalf("new library must be off before explicit activation: %+v %v", view, err)
	}
	before := time.Now().UnixMilli()
	if _, err := SetPromptsEnabledContext(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	for id, log := range map[string]string{"pi": piStarts, "codex": codexStarts} {
		if read(t, log) == "" {
			t.Errorf("off-to-on must check detected %s even with no templates", id)
		}
	}
	view, err = ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var public struct {
		Checks []struct {
			Agent     string `json:"agent"`
			Status    string `json:"status"`
			CheckedAt int64  `json:"checkedAt"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(encoded, &public); err != nil {
		t.Fatal(err)
	}
	if !view.Enabled || len(view.Items) != 0 {
		t.Errorf("activation must enable without inventing a template: %+v", view)
	}
	for id, want := range map[string]string{"pi": "available", "codex": "unverified"} {
		found := false
		for _, state := range public.Checks {
			if state.Agent == id {
				found = true
				if state.Status != want || state.CheckedAt < before || state.CheckedAt > time.Now().UnixMilli() {
					t.Errorf("%s check must report independent %s and completion time: %+v", id, want, state)
				}
			}
		}
		if !found {
			t.Errorf("empty library activation omitted %s global check", id)
		}
	}
	piBefore, codexBefore := read(t, piStarts), read(t, codexStarts)
	if _, err := SetPromptsEnabledContext(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if read(t, piStarts) != piBefore || read(t, codexStarts) != codexBefore {
		t.Error("repeated on must not restart Harness checks")
	}
}

func TestPromptTemplatesTargetCheckStartsAndReturnsOnlyRequestedHarness(t *testing.T) {
	_, codexStarts := promptCodexFixture(t)
	piStarts := os.Getenv("MAGPIE_FAKE_PI")
	// Decode the public request contract so an absent target field causes a
	// behavior failure rather than a compile-time failure.
	var request PromptChange
	if err := json.Unmarshal([]byte(`{"name":"audit-code","target":"codex"}`), &request); err != nil {
		t.Fatal(err)
	}
	states, err := CheckPromptContext(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if read(t, codexStarts) == "" {
		t.Error("checking Codex did not request its native inventory")
	}
	if read(t, piStarts) != "" || read(t, piStarts+".version") != "" {
		t.Error("checking only Codex started the unrelated Pi Harness")
	}
	if len(states) != 1 || states[0].Agent != "codex" {
		t.Errorf("targeted check must return only the requested Harness: %+v", states)
	}
}

func TestPromptTemplatesTargetCheckReusesSuccessfulEvidenceUnlessForced(t *testing.T) {
	_, starts := promptCodexFixture(t)
	write(t, promptStorePath(), `{"version":1,"enabled":true,"prompts":[],"applied":{}}`)
	check := func(payload string) PromptState {
		t.Helper()
		var request PromptChange
		if err := json.Unmarshal([]byte(payload), &request); err != nil {
			t.Fatal(err)
		}
		states, err := CheckPromptContext(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if len(states) != 1 || states[0].Agent != "codex" || states[0].Status != "available" {
			t.Fatalf("expected successful timestamped Codex evidence: %+v", states)
		}
		if states[0].CheckedAt == 0 {
			t.Error("successful target check omitted its completion time")
		}
		return states[0]
	}
	first := check(`{"name":"audit-code","target":"codex"}`)
	initialStarts := read(t, starts)
	if initialStarts == "" {
		t.Fatal("initial check must start Codex")
	}
	reused := check(`{"name":"review-code","target":"codex","force":false}`)
	if read(t, starts) != initialStarts {
		t.Error("checking another name without force restarted Codex despite valid successful evidence")
	}
	if reused.CheckedAt != first.CheckedAt {
		t.Error("reusing evidence changed its original completion time")
	}
	beforeForce := read(t, starts)
	forced := check(`{"name":"review-code","target":"codex","force":true}`)
	if read(t, starts) == beforeForce {
		t.Error("force:true must start a new native inventory check")
	}
	if forced.CheckedAt <= reused.CheckedAt {
		t.Error("forced successful check must publish a newer completion time")
	}
}

func TestPromptTemplatesEmptyNameRefreshesGlobalHarnessChecksWithoutTemplate(t *testing.T) {
	home, codexStarts := promptCodexFixture(t)
	piStarts := os.Getenv("MAGPIE_FAKE_PI")
	write(t, promptStorePath(), `{"version":1,"enabled":true,"prompts":[],"applied":{}}`)
	var request PromptChange
	if err := json.Unmarshal([]byte(`{"force":true}`), &request); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UnixMilli()
	states, err := CheckPromptContext(context.Background(), request)
	if err != nil {
		t.Fatalf("empty name and ID must refresh the global Harness checklist: %v", err)
	}
	view, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != len(view.Targets) {
		t.Errorf("global refresh omitted targets: %+v", states)
	}
	for _, id := range []string{"pi", "codex"} {
		found := false
		for _, state := range states {
			if state.Agent == id {
				found = true
				if state.Status != "available" || state.CheckedAt < before || state.CheckedAt > time.Now().UnixMilli() || state.Path != "" {
					t.Errorf("global %s check must report inventory evidence without a template path: %+v", id, state)
				}
			}
		}
		if !found {
			t.Errorf("missing global %s result", id)
		}
	}
	if read(t, piStarts) == "" || read(t, codexStarts) == "" {
		t.Error("global refresh did not check all detected Harnesses")
	}
	if len(view.Items) != 0 {
		t.Fatal("global refresh invented a saved template")
	}
	for _, path := range []string{filepath.Join(home, ".codex", "skills"), filepath.Join(home, ".pi", "agent", "prompts")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("global refresh unexpectedly created a deployment directory: %s %v", path, err)
		}
	}
}

func TestPromptTemplatesCodexActiveCheckEnumeratesNativeInventoryOnce(t *testing.T) {
	_, starts := promptCodexFixture(t)
	write(t, promptStorePath(), `{"version":1,"enabled":true,"prompts":[],"applied":{}}`)
	var request PromptChange
	if err := json.Unmarshal([]byte(`{"name":"audit-code","target":"codex","force":true}`), &request); err != nil {
		t.Fatal(err)
	}
	states, err := CheckPromptContext(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Agent != "codex" || states[0].Status != "available" {
		t.Fatalf("native check must still prove availability: %+v", states)
	}
	if got := strings.Fields(read(t, starts)); len(got) != 1 || got[0] != "app-server" {
		t.Fatalf("one active Codex check must enumerate native inventory once; process starts: %v", got)
	}
}

func TestPromptTemplatesCodexEvidenceIgnoresUnrelatedDSHResourceChanges(t *testing.T) {
	home, starts := promptCodexFixture(t)
	write(t, promptStorePath(), `{"version":1,"enabled":true,"prompts":[],"applied":{}}`)
	dshConfig := filepath.Join(home, ".dsh", "cordis.patch.yml")
	write(t, dshConfig, "# unrelated DSH configuration before\n")
	states, err := CheckPromptContext(context.Background(), PromptChange{Name: "audit-code", Target: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Status != "available" || states[0].CheckedAt == 0 {
		t.Fatalf("initial Codex inventory must be verified: %+v", states)
	}
	checkedAt, before := states[0].CheckedAt, read(t, starts)
	write(t, dshConfig, "# unrelated DSH configuration after\n")
	states, err = CheckPromptContext(context.Background(), PromptChange{Name: "audit-code", Target: "codex", Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Agent != "codex" || states[0].Status != "available" || states[0].CheckedAt != checkedAt {
		t.Errorf("unrelated DSH edits must preserve Codex evidence and its check time: %+v", states)
	}
	if read(t, starts) != before {
		t.Error("passive availability check restarted Codex")
	}
}

func TestPromptTemplatesSavingManagedCodexTemplatePreservesEvidence(t *testing.T) {
	_, starts := promptCodexFixture(t)
	write(t, promptStorePath(), `{"version":1,"enabled":true,"prompts":[],"applied":{}}`)
	states, err := CheckPromptContext(context.Background(), PromptChange{Name: "audit-code", Target: "codex"})
	if err != nil || len(states) != 1 || states[0].Status != "available" {
		t.Fatalf("initial Codex inventory failed: %+v %v", states, err)
	}
	before := read(t, starts)
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Explicit template body", Agents: []string{"codex"}}))
	view, err := ReadPrompts()
	if err != nil || len(view.Items) != 1 {
		t.Fatalf("saved template missing: %+v %v", view, err)
	}
	for _, state := range view.Items[0].States {
		if state.Agent == "codex" && state.Status != "enabled" {
			t.Errorf("creating managed deployment directories must preserve verified enabled state: %+v", state)
		}
	}
	states, err = CheckPromptContext(context.Background(), PromptChange{ID: view.Items[0].ID, Name: "audit-code", Text: "Explicit template body", Target: "codex"})
	if err != nil || len(states) != 1 || states[0].Status != "enabled" {
		t.Errorf("checking the saved template must reuse enabled evidence: %+v %v", states, err)
	}
	if read(t, starts) != before {
		t.Error("saving/reading/checking the unchanged managed template restarted Codex")
	}
}

func TestPromptTemplatesCodexSkillAssetsAreNotAdditionalSkillDeclarations(t *testing.T) {
	home, starts := promptCodexFixture(t)
	write(t, promptStorePath(), `{"version":1,"enabled":true,"prompts":[],"applied":{}}`)
	skill := filepath.Join(home, ".codex", "plugins", "reference-plugin", "skills", "reference-guide", "SKILL.md")
	write(t, skill, "---\nname: reference-guide\ndescription: Native skill leaf fixture\n---\nReference instructions\n")
	write(t, filepath.Join(filepath.Dir(skill), "agents", "openai.yaml"), "policy:\n  allow_implicit_invocation: false\n")
	// Native skill discovery stops at the valid skill directory. This is a
	// bundled example document, not another independently discovered skill.
	write(t, filepath.Join(filepath.Dir(skill), "assets", "example", "SKILL.md"), "Example asset without skill metadata\n")
	rows, err := json.Marshal([]map[string]any{{"name": "reference-guide", "path": skill, "enabled": true}})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(home, "codex-skills.json"), string(rows))
	states, err := CheckPromptContext(context.Background(), PromptChange{Name: "audit-code", Target: "codex", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Agent != "codex" || states[0].Status != "available" {
		t.Fatalf("assets inside a native skill leaf must not become extra declarations: %+v", states)
	}
	if got := strings.Fields(read(t, starts)); len(got) != 1 || got[0] != "app-server" {
		t.Fatalf("native inventory must remain the single enumeration authority: %v", got)
	}
}

func TestPromptTemplatesClosingSwitchReportsBackupOfPreviouslyEnabledIndex(t *testing.T) {
	sandbox(t)
	const oldIndex = `{"version":1,"enabled":true,"prompts":[],"applied":{}}`
	write(t, promptStorePath(), oldIndex)
	result, err := SetPromptsEnabledContext(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	view, err := ReadPrompts()
	if err != nil || view.Enabled {
		t.Fatalf("closing must persist disabled state: %+v %v", view, err)
	}
	if result == nil || result.Backup == "" {
		t.Fatalf("closing an existing enabled index must report its recovery backup: %+v", result)
	}
	if got := read(t, filepath.Join(result.Backup, "library", "prompts.json")); got != oldIndex {
		t.Fatalf("reported backup must preserve the exact pre-switch enabled index: %q", got)
	}
}

func TestPromptTemplatesLegacyIndexDefaultsEnabledWithoutStartingHarness(t *testing.T) {
	_, starts := promptCodexFixture(t)
	// Literal pre-switch persisted data: reading an upgrade must infer enabled
	// from the existing template without rewriting it or launching a Harness.
	const oldIndex = `{"version":1,"prompts":[{"id":"legacy-template","name":"legacy-audit","agents":["codex"]}],"applied":{}}`
	write(t, promptStorePath(), oldIndex)
	write(t, promptPath("legacy-template"), "Preserved legacy template body")
	view, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Items) != 1 || view.Items[0].Text != "Preserved legacy template body" {
		t.Fatalf("upgrade read lost the existing template: %+v", view)
	}
	if read(t, starts) != "" {
		t.Fatal("reading upgraded prompt settings started a Harness")
	}
	if read(t, promptStorePath()) != oldIndex {
		t.Fatal("reading upgraded prompt settings rewrote the legacy index")
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var public struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(encoded, &public); err != nil {
		t.Fatal(err)
	}
	if public.Enabled == nil || !*public.Enabled {
		t.Fatal("public prompt settings must default existing legacy templates to enabled")
	}
}

// Legacy CLI/Desktop ownership can hold opposite halves of one skill. Closing
// keeps that group managed; deleting its template releases both halves at once.
func TestPromptTemplatesRemovingEditedSplitCodexOwnershipReleasesWholeSkill(t *testing.T) {
	for _, edited := range []string{"body", "policy"} {
		t.Run(edited, func(t *testing.T) {
			home := sandbox(t)
			const id = "split-template"
			root := filepath.Join(home, ".codex", "skills", "audit-split")
			bodyPath := filepath.Join(root, "SKILL.md")
			policyPath := filepath.Join(root, "agents", "openai.yaml")
			body := "---\nname: audit-split\ndescription: Retained user skill\n---\nOriginal body\n"
			policy := "policy:\n  allow_implicit_invocation: false\n"
			write(t, bodyPath, body)
			write(t, policyPath, policy)
			index := map[string]any{
				"version": 1, "enabled": true,
				"prompts": []any{map[string]any{"id": id, "name": "audit-split", "agents": []string{"codex"}}},
				"applied": map[string]any{
					"codex":         map[string]any{id: map[string]any{"files": map[string]string{bodyPath: promptHash([]byte(body))}}},
					"codex-desktop": map[string]any{id: map[string]any{"files": map[string]string{policyPath: promptHash([]byte(policy))}}},
				},
			}
			encoded, err := json.Marshal(index)
			if err != nil {
				t.Fatal(err)
			}
			write(t, promptStorePath(), string(encoded))
			write(t, promptPath(id), "Original body")
			if edited == "body" {
				body += "External user edit\n"
			} else {
				policy += "# External user edit\n"
			}
			write(t, bodyPath, body)
			write(t, policyPath, policy)
			assertFiles := func() {
				t.Helper()
				if read(t, bodyPath) != body || read(t, policyPath) != policy {
					t.Fatal("retirement changed one half of the user's retained skill")
				}
			}
			assertDiscovery := func(want bool) {
				t.Helper()
				view, err := Read(nil)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, skill := range view.FoundSkills {
					found = found || skill.Name == "audit-split"
				}
				if found != want {
					t.Fatalf("ordinary discovery found retained skill = %v, want %v", found, want)
				}
			}
			result, err := SetPromptsEnabledContext(context.Background(), false)
			if err != nil || len(result.Problems) == 0 {
				t.Fatalf("close must report retained edited group: %+v %v", result, err)
			}
			assertFiles()
			assertDiscovery(false)
			result, err = RemovePrompt(id)
			if err != nil || len(result.Problems) == 0 {
				t.Fatalf("delete must report retained edited group: %+v %v", result, err)
			}
			assertFiles()
			assertDiscovery(true)
		})
	}
}
