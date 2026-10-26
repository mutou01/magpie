package library

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPromptTemplateDeployedFactSurvivesExpiredNativeEvidence(t *testing.T) {
	home, starts := promptEnabledCodexFixture(t)
	states, err := CheckPromptContext(context.Background(), PromptChange{Name: "audit-code", Target: "codex"})
	if err != nil || len(states) != 1 || states[0].Status != "available" {
		t.Fatalf("initial public check: %+v %v", states, err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Saved body", Agents: []string{"codex"}}))
	deployment := filepath.Join(home, ".codex", "skills", "audit-code", "SKILL.md")
	before := read(t, deployment)
	if before == "" {
		t.Fatal("public save did not deploy the skill")
	}
	// A real configuration edit invalidates runtime evidence without changing
	// either managed file. Reading must retain the independent deployment fact,
	// but must not restore native verification or start a process.
	write(t, filepath.Join(home, ".codex", "config.toml"), "# native evidence changed\n")
	processes := read(t, starts)
	view, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Items []struct {
			States []struct {
				Agent    string `json:"agent"`
				Status   string `json:"status"`
				Deployed bool   `json:"deployed"`
			} `json:"states"`
		} `json:"items"`
	}
	if err := json.Unmarshal(b, &contract); err != nil {
		t.Fatal(err)
	}
	if len(contract.Items) != 1 {
		t.Fatalf("saved template missing: %s", b)
	}
	for _, s := range contract.Items[0].States {
		if s.Agent != "codex" {
			continue
		}
		if s.Status != "unverified" {
			t.Fatalf("expired evidence must keep the write gate unverified: %+v", s)
		}
		if read(t, starts) != processes || read(t, deployment) != before {
			t.Fatal("passive read started Codex or changed the saved deployment")
		}
		if !s.Deployed {
			t.Fatalf("intact owned deployment must remain deployed despite expired native evidence: %+v", s)
		}
		return
	}
	t.Fatal("Codex state missing")
}

func TestPromptTemplateDeployedFactRejectsIncompleteOrInactiveFiles(t *testing.T) {
	for _, scenario := range []string{"body-edited", "policy-edited", "policy-missing", "disabled", "deselected", "body-not-synchronized", "codex-config-disabled", "native-disabled"} {
		t.Run(scenario, func(t *testing.T) {
			home, _ := promptEnabledCodexFixture(t)
			_, err := CheckPromptContext(context.Background(), PromptChange{Name: "audit-code", Target: "codex"})
			if err != nil {
				t.Fatal(err)
			}
			ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Saved body", Agents: []string{"codex"}}))
			view, err := ReadPrompts()
			if err != nil || len(view.Items) != 1 {
				t.Fatalf("public save fixture: %+v %v", view, err)
			}
			p := view.Items[0]
			skill := filepath.Join(home, ".codex", "skills", p.Name)
			switch scenario {
			case "body-edited":
				write(t, filepath.Join(skill, "SKILL.md"), "Externally changed body")
			case "policy-edited":
				write(t, filepath.Join(skill, "agents", "openai.yaml"), "policy:\n  allow_implicit_invocation: true\n")
			case "policy-missing":
				if err := os.Remove(filepath.Join(skill, "agents", "openai.yaml")); err != nil {
					t.Fatal(err)
				}
			case "disabled":
				ok(t)(SetPromptsEnabledContext(context.Background(), false))
			case "deselected":
				ok(t)(SetPromptAgents(p.ID, []string{}))
			case "body-not-synchronized":
				write(t, filepath.Join(home, ".codex", "config.toml"), "# invalidate runtime evidence\n")
				result, err := SavePrompt(PromptChange{ID: p.ID, Name: p.Name, Text: "New unsynchronized body", Agents: p.Agents})
				if err != nil || len(result.Problems) == 0 {
					t.Fatalf("expired evidence must report blocked deployment while retaining the saved draft: %+v %v", result, err)
				}
			case "codex-config-disabled":
				write(t, filepath.Join(home, ".codex", "config.toml"), "[[skills.config]]\npath = "+strconv.Quote(filepath.ToSlash(filepath.Join(skill, "SKILL.md")))+"\nenabled = false\n")
			case "native-disabled":
				rows, _ := json.Marshal([]map[string]any{{"name": p.Name, "path": filepath.Join(skill, "SKILL.md"), "enabled": false}})
				write(t, filepath.Join(home, "codex-skills.json"), string(rows))
				if _, err := CheckPromptContext(context.Background(), PromptChange{ID: p.ID, Name: p.Name, Text: p.Text, Target: "codex", Force: true}); err != nil {
					t.Fatal(err)
				}
			}
			view, err = ReadPrompts()
			if err != nil {
				t.Fatal(err)
			}
			for _, state := range view.Items[0].States {
				if state.Agent == "codex" && promptDeployedJSON(t, state) {
					t.Fatalf("%s must not claim a complete active managed deployment: %+v", scenario, state)
				}
			}
		})
	}
}

func TestPromptTemplateDeployedFactRejectsCurrentNameConflict(t *testing.T) {
	home := promptEnabledSandbox(t)
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Saved body", Agents: []string{"claude"}}))
	write(t, filepath.Join(home, ".claude", "skills", "audit-code", "SKILL.md"), "---\nname: audit-code\ndescription: Conflicting user skill\n---\nForeign body\n")
	view, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range view.Items[0].States {
		if s.Agent == "claude" {
			if s.Status != "conflict" || promptDeployedJSON(t, s) {
				t.Fatalf("a current same-name source must override intact owned bytes: %+v", s)
			}
			return
		}
	}
	t.Fatal("Claude state missing")
}

func promptDeployedJSON(t *testing.T, state PromptState) bool {
	t.Helper()
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Deployed bool `json:"deployed"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	return result.Deployed
}
