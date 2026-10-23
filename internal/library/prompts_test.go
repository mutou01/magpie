package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/edit"
)

func TestPromptTemplateTargetIcons(t *testing.T) {
	sandbox(t)
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range v.Targets {
		if target.ID == "codex" && target.Icon != "codex-color" {
			t.Fatalf("Codex must use the existing skill icon, got %q", target.Icon)
		}
	}
}

func TestPromptTemplateAgentUpdateProtectsLatestTemplateAndTargets(t *testing.T) {
	h := promptCheckedPiFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "audit-login", Text: "Original", Agents: []string{}}))
	v, _ := ReadPrompts()
	id := v.Items[0].ID
	ok(t)(SavePrompt(PromptChange{ID: id, Name: "audit-updated", Text: "Latest body", Agents: []string{}}))
	write(t, filepath.Join(h, ".claude/commands/audit-updated.md"), "User command")
	res, err := SetPromptAgents(id, []string{"claude", "pi", "pi"})
	if err != nil {
		t.Fatal(err)
	}
	v, _ = ReadPrompts()
	p := v.Items[0]
	if p.Name != "audit-updated" || p.Text != "Latest body" || !slices.Equal(p.Agents, []string{"pi"}) {
		t.Fatalf("target-only update lost latest content or accepted conflicting target: %+v", p)
	}
	if len(res.Problems) != 1 || res.Problems[0].Agent != "claude" {
		t.Fatalf("partial conflict missing: %+v", res)
	}
	if read(t, filepath.Join(h, ".claude/commands/audit-updated.md")) != "User command" {
		t.Fatal("user command overwritten")
	}
	if !strings.Contains(read(t, filepath.Join(h, ".pi/agent/prompts/audit-updated.md")), "Latest body") {
		t.Fatal("compatible target was not deployed")
	}
	for _, bad := range [][]string{nil, {"unknown-harness"}} {
		if _, err := SetPromptAgents(id, bad); err == nil {
			t.Fatalf("invalid target list accepted: %v", bad)
		}
	}
	if _, err := SetPromptAgents("missing", []string{}); err == nil {
		t.Fatal("missing template accepted")
	}
	v, _ = ReadPrompts()
	if !slices.Equal(v.Items[0].Agents, []string{"pi"}) {
		t.Fatal("failed request changed selection")
	}
	path := filepath.Join(h, ".pi/agent/prompts/audit-updated.md")
	write(t, path, "External edit")
	if _, err := SetPromptAgents(id, []string{"pi"}); err != nil {
		t.Fatal(err)
	}
	v, _ = ReadPrompts()
	if !slices.Equal(v.Items[0].Agents, []string{"pi"}) {
		t.Fatal("external edit implicitly deselected target")
	}
	res, err = SetPromptAgents(id, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 || read(t, path) != "External edit" {
		t.Fatal("explicit disable must report and retain the external edit")
	}
}

func TestPromptTemplateAgentUpdateDoesNotStartPiAndRetainsExpiredSelection(t *testing.T) {
	h, log := promptPiFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "audit-login", Text: "Body", Agents: []string{}}))
	v, _ := ReadPrompts()
	id := v.Items[0].ID
	res, err := SetPromptAgents(id, []string{"pi", "claude"})
	if err != nil || len(res.Problems) != 1 || res.Problems[0].Agent != "pi" {
		t.Fatalf("expired new target should be rejected independently: %+v %v", res, err)
	}
	if read(t, log) != "" || read(t, log+".version") != "" {
		t.Fatal("target-only action started Pi")
	}
	if s := piPromptState(t, PromptChange{ID: id, Name: "audit-login"}); s.Status != "available" {
		t.Fatal(s)
	}
	before := read(t, log)
	ok(t)(SetPromptAgents(id, []string{"claude", "pi"}))
	write(t, filepath.Join(h, ".pi/agent/extensions/new.ts"), "export default pi => {}")
	res, err = SetPromptAgents(id, []string{"pi"})
	if err != nil || len(res.Problems) == 0 {
		t.Fatalf("expired retained target must report verification failure: %+v %v", res, err)
	}
	v, _ = ReadPrompts()
	if !slices.Equal(v.Items[0].Agents, []string{"pi"}) || read(t, filepath.Join(h, ".pi/agent/prompts/audit-login.md")) != "Body\n\n$ARGUMENTS\n" {
		t.Fatal("expired verification lost selection or owned deployment")
	}
	ok(t)(SetPromptAgents(id, []string{}))
	if _, err := os.Stat(filepath.Join(h, ".pi/agent/prompts/audit-login.md")); !os.IsNotExist(err) {
		t.Fatal("explicit removal did not retire expired deployment")
	}
	if read(t, log) != before {
		t.Fatal("target update silently rechecked Pi")
	}
}

func TestPromptTemplateAgentUpdateKeepsBodyAndPartialDeploymentFailure(t *testing.T) {
	h := promptCheckedPiFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "audit-login", Text: "Body", Agents: []string{}}))
	v, _ := ReadPrompts()
	id := v.Items[0].ID
	path := filepath.Join(h, ".claude/commands/audit-login.md")
	var res *Result
	err := edit.Filtered(func(p string, b []byte) []byte {
		if strings.Contains(filepath.ToSlash(p), "/.claude/commands/") {
			write(t, path, "Intervening user file")
		}
		return b
	}, func() error {
		var err error
		res, err = SetPromptAgents(id, []string{"claude", "pi"})
		return err
	})
	if err != nil || len(res.Problems) == 0 {
		t.Fatalf("intervening file not reported: %+v %v", res, err)
	}
	v, _ = ReadPrompts()
	if !slices.Equal(v.Items[0].Agents, []string{"claude", "pi"}) || read(t, path) != "Intervening user file" {
		t.Fatal("partial deployment erased intent or user file")
	}
	for _, s := range v.Items[0].States {
		if s.Agent == "pi" && s.Status != "enabled" || s.Agent == "claude" && s.Status == "enabled" {
			t.Fatalf("selection was mistaken for successful deployment: %+v", s)
		}
	}
	if err := os.Remove(promptPath(id)); err != nil {
		t.Fatal(err)
	}
	res, err = SetPromptAgents(id, []string{"pi"})
	if err != nil || len(res.Problems) == 0 {
		t.Fatalf("missing text must remain unknown: %+v %v", res, err)
	}
	if _, err := os.Stat(promptPath(id)); !os.IsNotExist(err) {
		t.Fatal("target update recreated missing text")
	}
	ok(t)(SetPromptAgents(id, []string{}))
}

func TestPromptTemplatesAppearInLibrary(t *testing.T) {
	sandbox(t)
	v, err := Glance(nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got["prompts"]) == 0 {
		t.Fatal("Library omits prompt templates")
	}
}

func TestPromptTemplateDeploysAndDoesNotBecomeInstructions(t *testing.T) {
	h := promptCheckedPiFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "代码审查", Text: "检查边界条件。", Agents: []string{"claude", "pi"}}))
	for _, rel := range []string{".claude/commands/代码审查.md", ".pi/agent/prompts/代码审查.md"} {
		b, err := os.ReadFile(filepath.Join(h, rel))
		if err != nil {
			t.Fatalf("template not deployed to %s: %v", rel, err)
		}
		if !strings.Contains(string(b), "检查边界条件。") || !strings.Contains(string(b), "$ARGUMENTS") {
			t.Fatalf("template or user arguments missing: %s", b)
		}
	}
	if strings.Contains(read(t, filepath.Join(h, ".claude/CLAUDE.md")), "检查边界条件。") {
		t.Fatal("template became permanent instructions")
	}
}

func TestPromptTemplateBuiltinAndSkillConflictsArePerTarget(t *testing.T) {
	h := promptCheckedPiFixture(t)
	res, err := SavePrompt(PromptChange{Name: "plan", Text: "Template", Agents: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 {
		t.Fatal("built-in /plan was not protected")
	}
	if _, err := os.Stat(filepath.Join(h, ".claude/commands/plan.md")); !os.IsNotExist(err) {
		t.Fatal("built-in conflict was deployed")
	}
	write(t, filepath.Join(h, ".claude/skills/review-login/SKILL.md"), "---\nname: review-login\ndescription: existing\n---\nUser skill")
	res, err = SavePrompt(PromptChange{Name: "review-login", Text: "Template", Agents: []string{"claude", "pi"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 {
		t.Fatal("installed skill conflict was not detected")
	}
	if _, err := os.Stat(filepath.Join(h, ".claude/commands/review-login.md")); !os.IsNotExist(err) {
		t.Fatal("skill conflict was deployed")
	}
	if !strings.Contains(read(t, filepath.Join(h, ".pi/agent/prompts/review-login.md")), "Template") {
		t.Fatal("one target's conflict blocked another target")
	}
}

func TestPromptTemplateNormalSyncAndMissingBody(t *testing.T) {
	h := sandbox(t)
	ok(t)(SavePrompt(PromptChange{Name: "审查", Text: "first", Agents: []string{"claude"}}))
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	id := v.Items[0].ID
	write(t, promptPath(id), "second")
	ok(t)(Sync())
	path := filepath.Join(h, ".claude/commands/审查.md")
	if !strings.Contains(read(t, path), "second") {
		t.Fatal("normal sync did not update template")
	}
	if err := os.Remove(promptPath(id)); err != nil {
		t.Fatal(err)
	}
	res, err := Sync()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 {
		t.Fatal("missing template body was treated as empty")
	}
	if !strings.Contains(read(t, path), "second") {
		t.Fatal("missing body removed the installed command")
	}
}

func TestPromptTemplateCodexDesktopIsExplicitAndNotAnImportableSkill(t *testing.T) {
	promptCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "review-login"}); err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "review-login", Text: "Review this request.", Agents: []string{"codex-desktop"}}))
	path := filepath.Join(codexDir(), "skills/review-login/SKILL.md")
	if !strings.Contains(read(t, path), "Review this request.") {
		t.Fatal("Desktop skill body missing")
	}
	if !strings.Contains(read(t, filepath.Join(filepath.Dir(path), "agents/openai.yaml")), "allow_implicit_invocation: false") {
		t.Fatal("Desktop template can be invoked implicitly")
	}
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range v.FoundSkills {
		if s.Name == "review-login" {
			t.Fatal("template skill can be imported or removed by ordinary skill management")
		}
	}
}

func TestPromptTemplateBackupAndOldBundleCompatibility(t *testing.T) {
	sandbox(t)
	ok(t)(SavePrompt(PromptChange{Name: "审查", Text: "original", Agents: []string{}}))
	v, _ := ReadPrompts()
	id := v.Items[0].ID
	b, err := Collect()
	if err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{ID: id, Name: "审查", Text: "changed", Agents: []string{}}))
	ok(t)(Put(b))
	v, _ = ReadPrompts()
	if len(v.Items) != 1 || v.Items[0].Text != "original" {
		t.Fatal("backup did not restore prompt text")
	}
	// Backups written before templates existed must not erase this feature.
	ok(t)(Put(&Bundle{}))
	v, _ = ReadPrompts()
	if len(v.Items) != 1 {
		t.Fatal("old bundle erased templates it did not carry")
	}
}

func TestPromptTemplateSetupRestoresSelection(t *testing.T) {
	promptCheckedPiFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Check.", Agents: []string{"pi"}}))
	v, _ := ReadPrompts()
	id := v.Items[0].ID
	s, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{ID: id, Name: "audit-code", Text: "Check.", Agents: []string{}}))
	if Holds(s) {
		t.Fatal("profile ignored changed template selection")
	}
	ok(t)(Restore(s))
	v, _ = ReadPrompts()
	if len(v.Items[0].Agents) != 1 || v.Items[0].Agents[0] != "pi" {
		t.Fatal("profile did not restore template selection")
	}
}

// A snapshot keeps templates alongside develop's repaired instruction sets:
// a legacy managed block is never captured as part of the user's set text.
func TestPromptTemplateSnapshotKeepsCleanInstructions(t *testing.T) {
	promptCheckedPiFixture(t)
	shared := "Keep user rules."
	ok(t)(SaveInstructions(InstructionsChange{Shared: &shared, Agents: []string{"claude"}}))
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Review this request.", Agents: []string{"pi"}}))
	write(t, filepath.Join(Dir(), "instructions.md"), "Keep user rules.\n\n"+blockBegin+"\nLegacy duplicated rules.\n"+blockEnd+"\n")
	s, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if s.Instructions.Shared != shared {
		t.Fatalf("snapshot includes a legacy managed block: %q", s.Instructions.Shared)
	}
	if !Holds(s) {
		t.Fatal("snapshot does not match its own instruction and template selections")
	}
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	id := v.Items[0].ID
	if len(s.Prompts[id]) != 1 || s.Prompts[id][0] != "pi" {
		t.Fatal("snapshot lost template selection")
	}
	ok(t)(SavePrompt(PromptChange{ID: id, Name: "audit-code", Text: "Review this request.", Agents: []string{}}))
	changed := "Changed rules."
	ok(t)(SaveInstructions(InstructionsChange{Shared: &changed, Agents: []string{"claude"}}))
	ok(t)(Restore(s))
	iv, err := ReadInstructions()
	if err != nil {
		t.Fatal(err)
	}
	if iv.Shared != shared || !Holds(s) {
		t.Fatalf("restore lost instructions or template selections: %+v", iv)
	}
}

func TestPromptTemplateNamesAndProjectConflicts(t *testing.T) {
	h := sandbox(t)
	for _, name := range []string{"", "a/b", "a b", "a\u200bb", "../a", strings.Repeat("审", 61)} {
		if _, err := SavePrompt(PromptChange{Name: name, Text: "body", Agents: []string{}}); err == nil {
			t.Fatalf("accepted invalid name %q", name)
		}
	}
	ok(t)(SavePrompt(PromptChange{Name: "Ｒｅｖｉｅｗ", Text: "body", Agents: []string{}}))
	if _, err := SavePrompt(PromptChange{Name: "review", Text: "body", Agents: []string{}}); err == nil {
		t.Fatal("accepted NFKC-equivalent name")
	}
	project := filepath.Join(h, "work")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	ok(t)(AddProject(project))
	write(t, filepath.Join(project, ".claude/commands/audit-login.md"), "User command")
	res, err := SavePrompt(PromptChange{Name: "audit-login", Text: "body", Agents: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 {
		t.Fatal("registered project conflict was ignored")
	}
	if _, err := os.Stat(filepath.Join(h, ".claude/commands/audit-login.md")); !os.IsNotExist(err) {
		t.Fatal("project conflict deployed globally")
	}
}

func TestPromptTemplateNewConflictRetiresOnlyUneditedDeployment(t *testing.T) {
	h := promptCheckedPiFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "audit-login", Text: "original", Agents: []string{"claude", "pi"}}))
	write(t, filepath.Join(h, ".claude/skills/audit-login/SKILL.md"), "---\nname: audit-login\ndescription: User skill\n---\nUser text")
	res, err := Sync()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 {
		t.Fatal("new conflict was not reported")
	}
	if _, err := os.Stat(filepath.Join(h, ".claude/commands/audit-login.md")); !os.IsNotExist(err) {
		t.Fatal("conflicting managed command was left active")
	}
	path := filepath.Join(h, ".pi/agent/prompts/audit-login.md")
	write(t, path, "User-edited command")
	v, _ := ReadPrompts()
	res, err = RemovePrompt(v.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 || read(t, path) != "User-edited command" {
		t.Fatal("external edit was not protected")
	}
}

func TestPromptTemplateDeletedFileIsNotShownEnabled(t *testing.T) {
	h := promptCheckedPiFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "audit-login", Text: "body", Agents: []string{"pi"}}))
	if err := os.Remove(filepath.Join(h, ".pi/agent/prompts/audit-login.md")); err != nil {
		t.Fatal(err)
	}
	v, _ := ReadPrompts()
	for _, s := range v.Items[0].States {
		if s.Agent == "pi" && s.Status == "enabled" {
			t.Fatal("missing deployment still shown enabled")
		}
	}
}

func TestPromptTemplateRejectsInvalidTargetBeforeWriting(t *testing.T) {
	sandbox(t)
	if _, err := SavePrompt(PromptChange{Name: "audit-login", Text: "body", Agents: []string{"../../claude"}}); err == nil {
		t.Fatal("invalid target was accepted")
	}
	v, _ := ReadPrompts()
	if len(v.Items) != 0 {
		t.Fatal("invalid request changed the library")
	}
}

func TestPromptTemplateDoesNotClobberConcurrentUserCommand(t *testing.T) {
	h := sandbox(t)
	path := filepath.Join(h, ".claude/commands/audit-login.md")
	var result *Result
	err := edit.Filtered(func(p string, b []byte) []byte {
		if strings.Contains(filepath.ToSlash(p), "/.claude/commands/") {
			write(t, path, "Concurrent user command")
		}
		return b
	}, func() error {
		var err error
		result, err = SavePrompt(PromptChange{Name: "audit-login", Text: "Template", Agents: []string{"claude"}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, path) != "Concurrent user command" || len(result.Problems) == 0 {
		t.Fatalf("template overwrote a command created during deployment: body=%q result=%+v", read(t, path), result)
	}
}

func TestPromptTemplateAdaptersAreLimitedToInstalledHarnesses(t *testing.T) {
	sandbox(t)
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range v.Targets {
		if !slices.Contains([]string{"claude", "pi", "codex", "codex-desktop", "dsh"}, target.ID) {
			t.Fatalf("uninstalled harness adapter exposed: %s", target.ID)
		}
	}
}

func TestPromptTemplateUnknownDynamicCommandsBlockOnlyThatTarget(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".pi/agent/settings.json"), `{"extensions":["./custom.ts"]}`)
	res, err := SavePrompt(PromptChange{Name: "audit-login", Text: "Template", Agents: []string{"claude", "pi"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 {
		t.Fatal("unverified extension commands were treated as no conflict")
	}
	if _, err := os.Stat(filepath.Join(h, ".pi/agent/prompts/audit-login.md")); !os.IsNotExist(err) {
		t.Fatal("unverified target was deployed")
	}
	if !strings.Contains(read(t, filepath.Join(h, ".claude/commands/audit-login.md")), "Template") {
		t.Fatal("unknown source blocked the other target")
	}
}

func TestPromptTemplateProtectsObservedBuiltinAliases(t *testing.T) {
	promptCheckedPiFixture(t)
	for _, c := range []struct{ agent, name string }{{"claude", "loop"}, {"claude", "proactive"}, {"claude", "insights"}, {"claude", "checkup"}, {"pi", "thinking"}, {"pi", "clone"}, {"pi", "trust"}, {"pi", "import"}} {
		states, err := CheckPrompt(PromptChange{Name: c.name, Text: "body"})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, s := range states {
			if s.Agent == c.agent {
				found = s.Status == "conflict"
			}
		}
		if !found {
			t.Errorf("%s builtin or alias /%s was not protected", c.agent, c.name)
		}
	}
}

func TestPromptTemplateCorruptMetadataDoesNotReadOutsideLibrary(t *testing.T) {
	sandbox(t)
	write(t, path(), `{"prompts":[{"id":"../escape","name":"审查","agents":[]}]}`)
	if _, err := ReadPrompts(); err == nil {
		t.Fatal("invalid prompt path was accepted from damaged metadata")
	}
}

func TestPromptTemplateProtectsSkillFolderAlias(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".claude/skills/folder-alias/SKILL.md"), "---\nname: metadata-name\ndescription: User skill\n---\nUser instructions")
	res, err := SavePrompt(PromptChange{Name: "folder-alias", Text: "Template", Agents: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 {
		t.Fatal("skill folder alias was not protected")
	}
}

func TestPromptTemplatePiUsesActualSkillNamespaceAndExplicitPromptFiles(t *testing.T) {
	h, _ := promptPiFixture(t)
	write(t, filepath.Join(h, ".pi/agent/skills/audit-login/SKILL.md"), "---\nname: audit-login\ndescription: User skill\n---\nInstructions")
	if s := piPromptState(t, PromptChange{Name: "audit-login"}); s.Status != "available" {
		t.Fatal(s)
	}
	ok(t)(SavePrompt(PromptChange{Name: "audit-login", Text: "Template", Agents: []string{"pi"}}))
	custom := filepath.Join(h, "custom-prompts", "audit-custom.md")
	write(t, custom, "User prompt")
	b, _ := json.Marshal(map[string]any{"prompts": []string{custom}})
	write(t, filepath.Join(h, ".pi/agent/settings.json"), string(b))
	res, err := SavePrompt(PromptChange{Name: "audit-custom", Text: "Template", Agents: []string{"pi"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 {
		t.Fatal("explicit prompt file conflict was not found")
	}
}

func TestPromptTemplateSyncBacksUpEveryDesktopSkill(t *testing.T) {
	promptCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "audit-one"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"audit-one", "audit-two", "audit-three"} {
		ok(t)(SavePrompt(PromptChange{Name: name, Text: "original " + name, Agents: []string{"codex-desktop"}}))
	}
	v, _ := ReadPrompts()
	for _, p := range v.Items {
		write(t, promptPath(p.ID), "updated "+p.Name)
	}
	res := ok(t)(Sync())
	var count int
	if err := filepath.WalkDir(res.Backup, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "SKILL.md" {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("backup kept %d of 3 Desktop skill bodies", count)
	}
	empty := []CarriedPrompt{}
	res = ok(t)(Put(&Bundle{Prompts: &empty}))
	count = 0
	if err := filepath.WalkDir(res.Backup, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "openai.yaml" {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("backup kept %d of 3 Desktop invocation policies", count)
	}
}

func TestPromptTemplateCaseOnlyRenameKeepsTheNewCommand(t *testing.T) {
	h := promptCheckedPiFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "AuditLogin", Text: "Template", Agents: []string{"pi"}}))
	v, _ := ReadPrompts()
	ok(t)(SavePrompt(PromptChange{ID: v.Items[0].ID, Name: "auditlogin", Text: "Template", Agents: []string{"pi"}}))
	es, err := os.ReadDir(filepath.Join(h, ".pi/agent/prompts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 || es[0].Name() != "auditlogin.md" {
		t.Fatalf("case-only rename left the wrong command: %v", es)
	}
}

func TestPromptTemplateInvalidUTF8BodyIsNotDeployedOrCarried(t *testing.T) {
	h := sandbox(t)
	ok(t)(SavePrompt(PromptChange{Name: "audit-login", Text: "Original template", Agents: []string{"claude"}}))
	v, _ := ReadPrompts()
	if err := os.WriteFile(promptPath(v.Items[0].ID), []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Sync()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 || !strings.Contains(read(t, filepath.Join(h, ".claude/commands/audit-login.md")), "Original template") {
		t.Fatal("invalid UTF-8 was propagated to the harness")
	}
	if _, err := Collect(); err == nil {
		t.Fatal("backup carried an invalid template as valid text")
	}
}

func TestPromptTemplateRemovalReleasesEditedDesktopSkill(t *testing.T) {
	promptCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "audit-login"}); err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "audit-login", Text: "Original", Agents: []string{"codex-desktop"}}))
	v, _ := ReadPrompts()
	path := filepath.Join(codexDir(), "skills/audit-login/SKILL.md")
	write(t, path, "---\nname: audit-login\ndescription: User skill\n---\nUser edit")
	res, err := RemovePrompt(v.Items[0].ID)
	if err != nil || len(res.Problems) == 0 {
		t.Fatalf("external edit not reported: %v %+v", err, res)
	}
	view, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range view.FoundSkills {
		if s.Name == "audit-login" {
			found = true
		}
	}
	if !found {
		t.Fatal("removed template still hides the retained user's skill")
	}
}
