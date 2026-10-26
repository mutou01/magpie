package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A pre-template writer knows neither Library.Prompts nor Applied.Prompts.
// Preserve its other fields, as an old binary's load/save would do.
func rewriteLibraryWithoutPrompts(t *testing.T) {
	t.Helper()
	var old map[string]json.RawMessage
	if err := json.Unmarshal([]byte(read(t, path())), &old); err != nil {
		t.Fatal(err)
	}
	delete(old, "prompts")
	var applied map[string]map[string]json.RawMessage
	if err := json.Unmarshal(old["applied"], &applied); err != nil {
		t.Fatal(err)
	}
	for _, a := range applied {
		delete(a, "prompts")
	}
	b, err := json.Marshal(applied)
	if err != nil {
		t.Fatal(err)
	}
	old["applied"] = b
	b, err = json.MarshalIndent(old, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write(t, path(), string(b))
}

func TestPromptTemplateSurvivesLegacyLibraryRewrite(t *testing.T) {
	h := promptEnabledSandbox(t)
	ok(t)(SavePrompt(PromptChange{Name: "review-code", Text: "Keep my template.", Agents: []string{"claude"}}))
	rewriteLibraryWithoutPrompts(t)
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Items) != 1 || v.Items[0].Name != "review-code" || v.Items[0].Text != "Keep my template." {
		t.Fatalf("legacy writer erased the template index: %+v", v.Items)
	}
	// Ownership must survive too: updating must not mistake our file for a
	// user's command, and removal must still retire only that owned file.
	ok(t)(SavePrompt(PromptChange{ID: v.Items[0].ID, Name: "review-code", Text: "Updated template.", Agents: []string{"claude"}}))
	command := filepath.Join(h, ".claude/commands/review-code.md")
	if !strings.Contains(read(t, command), "Updated template.") {
		t.Fatal("legacy writer erased deployment ownership")
	}
	ok(t)(RemovePrompt(v.Items[0].ID))
	if _, err := os.Stat(command); !os.IsNotExist(err) {
		t.Fatalf("owned command was not removed: %v", err)
	}
}

func TestPromptTemplateRejectsIncompleteOwnershipIndex(t *testing.T) {
	sandbox(t)
	ok(t)(SavePrompt(PromptChange{Name: "review", Text: "Keep this text.", Agents: []string{}}))
	// A readable JSON file is not necessarily a complete ownership record.
	// Falling back to the mirror would hide damage and allow a later sync
	// to overwrite the only remaining record of our deployments.
	index := filepath.Join(Dir(), "prompts.json")
	broken := `{"version":1,"prompts":[],"applied":{"claude":{"p1":null}}}`
	write(t, index, broken)
	if _, err := ReadPrompts(); err == nil {
		t.Fatal("incomplete ownership index was treated as an empty library")
	}
	if _, err := SavePrompt(PromptChange{Name: "other", Text: "Other.", Agents: []string{}}); err == nil {
		t.Fatal("save ignored an incomplete ownership index")
	}
	if read(t, index) != broken {
		t.Fatal("failed operation rewrote the damaged index")
	}
}

func TestPromptTemplateSyncBacksUpOwnershipIndex(t *testing.T) {
	h := promptEnabledSandbox(t)
	ok(t)(SavePrompt(PromptChange{Name: "review-login", Text: "Check.", Agents: []string{"claude"}}))
	rewriteLibraryWithoutPrompts(t)
	write(t, filepath.Join(h, ".claude/skills/review-login/SKILL.md"), "---\nname: review-login\ndescription: User skill\n---\nUser text")
	res, err := Sync()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 || res.Backup == "" {
		t.Fatalf("new conflict did not preserve its deployment: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(res.Backup, "library/prompts.json")); err != nil {
		t.Fatalf("sync's reported backup omits prompt ownership: %v", err)
	}
}

func TestPromptTemplateOtherChangeBacksUpOwnershipIndex(t *testing.T) {
	h := promptEnabledSandbox(t)
	ok(t)(SavePrompt(PromptChange{Name: "review-login", Text: "Check.", Agents: []string{"claude"}}))
	write(t, filepath.Join(h, ".claude/skills/review-login/SKILL.md"), "---\nname: review-login\ndescription: User skill\n---\nUser text")
	// Server changes also sync the Library. A newly conflicting template's
	// retired deployment and ownership must share that operation's backup.
	res, err := SaveServer("", Server{Name: "fixture", Transport: "stdio", Command: "fixture", Agents: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 || res.Backup == "" {
		t.Fatalf("new conflict did not preserve its deployment: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(res.Backup, "library/prompts.json")); err != nil {
		t.Fatalf("server change's reported backup omits prompt ownership: %v", err)
	}
}

func TestPromptTemplateEmptyIndexDoesNotReviveDeletedTemplate(t *testing.T) {
	sandbox(t)
	ok(t)(SavePrompt(PromptChange{Name: "review", Text: "Remove me.", Agents: []string{}}))
	stale := read(t, path())
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	ok(t)(RemovePrompt(v.Items[0].ID))
	// Another process may still have the old mirror in memory.
	write(t, path(), stale)
	v, err = ReadPrompts()
	if err != nil || len(v.Items) != 0 {
		t.Fatalf("stale mirror revived a deleted template: %+v, %v", v, err)
	}
	bundle, err := Collect()
	if err != nil || bundle.Prompts == nil || len(*bundle.Prompts) != 0 {
		t.Fatalf("backup revived a deleted template: %+v, %v", bundle, err)
	}
}

func TestPromptTemplateMigratesLegacyIndexOnUnrelatedSave(t *testing.T) {
	sandbox(t)
	write(t, filepath.Join(Dir(), "prompts/p1.md"), "Existing template.")
	write(t, path(), `{"prompts":[{"id":"p1","name":"审查","agents":[]}],"applied":{}}`)
	v, err := ReadPrompts()
	if err != nil || len(v.Items) != 1 || v.Items[0].Text != "Existing template." {
		t.Fatalf("legacy template was not readable: %+v, %v", v, err)
	}
	if _, err := os.Stat(filepath.Join(Dir(), "prompts.json")); !os.IsNotExist(err) {
		t.Fatal("reading the page wrote a migration")
	}
	shared := "Keep the shared instructions."
	ok(t)(SaveInstructions(InstructionsChange{Shared: &shared, Agents: []string{}}))
	rewriteLibraryWithoutPrompts(t)
	v, err = ReadPrompts()
	if err != nil || len(v.Items) != 1 || v.Items[0].Name != "审查" {
		t.Fatalf("unrelated save did not protect the legacy template: %+v, %v", v, err)
	}
	iv, err := ReadInstructions()
	if err != nil || iv.Shared != shared {
		t.Fatalf("template migration changed shared instructions: %+v, %v", iv, err)
	}
}

func TestPromptTemplateBrokenIndexNeverFallsBackToMirror(t *testing.T) {
	for _, contents := range []string{"", "null", "{}", "{", `{"version":2,"prompts":[],"applied":{}}`, `{"version":1,"prompts":[],"applied":{"claude":{"p1":{"files":{"relative.md":"bad"}}}}}`} {
		t.Run(contents, func(t *testing.T) {
			sandbox(t)
			ok(t)(SavePrompt(PromptChange{Name: "review", Text: "Keep.", Agents: []string{}}))
			index := filepath.Join(Dir(), "prompts.json")
			write(t, index, contents)
			if _, err := ReadPrompts(); err == nil {
				t.Fatal("bad index was silently replaced by its mirror")
			}
			if _, err := Sync(); err == nil {
				t.Fatal("sync ignored the damaged index")
			}
			if read(t, index) != contents {
				t.Fatal("failed sync changed the damaged index")
			}
		})
	}
}
