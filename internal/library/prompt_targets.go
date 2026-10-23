package library

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/yetone/magpie/internal/agent"
)

// PromptTarget describes native invocation, separately from the agent's
// always-loaded instructions. Codex uses one target for CLI/Desktop shared files.
type PromptTarget struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Icon         string `json:"icon"`
	Detected     bool   `json:"detected"`
	Reload       string `json:"reload"`
	Shared       bool   `json:"shared,omitempty"`
	Verification string `json:"verification"`
	Invocation   string `json:"invocation"`
	dir          string
	family       string
}

type PromptState struct {
	Agent  string `json:"agent"`
	Status string `json:"status"` // available, enabled, conflict, unsupported, unverified, edited
	Reason string `json:"reason,omitempty"`
	Source string `json:"source,omitempty"`
	Path   string `json:"path,omitempty"`
}

// These catalogs deliberately include conditional built-ins and aliases. They
// reserve names rather than claiming to enumerate a running session. Sources:
// code.claude.com/docs/en/interactive-mode;
// learn.chatgpt.com/docs/reference/slash-commands. Only locally installed
// harness families are implemented in this first release.
var promptBuiltins = map[string]string{
	"claude": "help clear compact config context cost doctor exit export feedback hooks init login logout mcp memory model permissions plan plugin plugins rename resume review rewind status tasks terminal theme usage vim add-dir agents skills deep-research dataviz update-config verify debug code-review simplify batch fewer-permission-prompts loop claude-api workflow-authoring run run-skill-generator auto-mode-setup autocompact color output-style effort fast heapdump __remote-workflow workflow-launch-exec reload-plugins reload-skills security-review insights recap goal list-agents team-onboarding checkup proactive reset new settings name stats peers",
	"codex":  "approve archive chat cloud cloud-environment compact fast feedback fork goal ide-context init local mcp memories model new pet pin plan project reasoning rename review side status usage worktree 计划",
}

func promptTargets() []PromptTarget {
	byID := map[string]*agent.Agent{}
	for _, a := range agent.All() {
		if a.WSL == "" && a.Plugin == "" {
			byID[a.ID] = a
		}
	}
	var out []PromptTarget
	for _, id := range []string{"claude", "pi", "codex", "dsh"} {
		t := PromptTarget{ID: id, family: id, Invocation: "/<name>", Verification: "pending", Reload: "Restart the harness to load updated commands"}
		if a := byID[id]; a != nil {
			t.Name, t.Icon, t.dir, t.Detected = a.Name, a.Icon, a.Dir, a.Detected() && !a.CLIMissing()
			if id == "claude" {
				t.dir = claudeDir()
			}
		}
		switch id {
		case "codex":
			t.Name, t.Icon, t.dir, t.Shared = "Codex", "codex-color", codexDir(), true
			// Detection follows the Agent page, including shared-app evidence.
			// Native inventory verification separately requires the CLI entry.
			t.Invocation = "/skills · $<name>"
		case "dsh":
			t.Invocation = "/<name>"
		case "pi":
			t.Reload = "Run /reload to load updated commands"
		}
		out = append(out, t)
	}
	return out
}

func (t PromptTarget) folder() string {
	switch t.ID {
	case "codex-desktop", "codex", "dsh":
		return filepath.Join(t.dir, "skills")
	case "pi":
		return filepath.Join(t.dir, "prompts")
	default:
		return filepath.Join(t.dir, "commands")
	}
}

func (t PromptTarget) files(p *Prompt, text string) (map[string][]byte, error) {
	name, quoted := p.Name, func(s string) string { b, _ := json.Marshal(s); return string(b) }
	if t.family == "" || !filepath.IsAbs(t.dir) {
		return nil, fmt.Errorf("native commands are not verified for this harness")
	}
	if !filepath.IsLocal(name + ".md") {
		return nil, fmt.Errorf("This command name is not a valid file name")
	}
	if t.ID == "dsh" {
		if !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(name) {
			return nil, fmt.Errorf("DeepSeek Harness names require lowercase ASCII letters, digits and single hyphens")
		}
		return map[string][]byte{filepath.Join(t.folder(), name, "SKILL.md"): []byte("---\nname: " + quoted(name) + "\ndescription: " + quoted("Explicitly invoke /"+name+" to apply these instructions to the accompanying request.") + "\ndisable-model-invocation: true\nuser-invocable: true\n---\n\n" + text + "\n")}, nil
	}
	if t.ID == "codex-desktop" || t.ID == "codex" {
		dir := filepath.Join(t.folder(), name)
		return map[string][]byte{
			filepath.Join(dir, "SKILL.md"):              []byte("---\nname: " + quoted(name) + "\ndescription: " + quoted("Explicitly invoke this skill to apply these instructions to the accompanying request.") + "\n---\n\n" + text + "\n"),
			filepath.Join(dir, "agents", "openai.yaml"): []byte("policy:\n  allow_implicit_invocation: false\n"),
		}, nil
	}
	body := text + "\n\n$ARGUMENTS\n"
	if t.ID != "pi" {
		body = "---\ndescription: " + quoted("Prompt: "+name) + "\n---\n\n" + body
	}
	return map[string][]byte{filepath.Join(t.folder(), name+".md"): []byte(body)}, nil
}

func promptTargetByID(all []PromptTarget, id string) (PromptTarget, bool) {
	if id == "codex-desktop" { // compatibility with older clients and ownership records
		id = "codex"
	}
	i := slices.IndexFunc(all, func(t PromptTarget) bool { return t.ID == id })
	if i < 0 {
		return PromptTarget{}, false
	}
	return all[i], true
}
