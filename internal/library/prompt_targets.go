package library

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/yetone/magpie/internal/agent"
)

// PromptTarget describes native invocation, separately from the agent's
// always-loaded instructions. Desktop and CLI can share physical files.
type PromptTarget struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Icon         string `json:"icon"`
	Detected     bool   `json:"detected"`
	Reload       string `json:"reload"`
	Shared       bool   `json:"shared,omitempty"`
	Verification string `json:"verification"`
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
// code.claude.com/docs/en/interactive-mode; pi core/slash-commands.ts;
// learn.chatgpt.com/docs/reference/slash-commands. Only locally installed
// harness families are implemented in this first release.
var promptBuiltins = map[string]string{
	"claude":        "help clear compact config context cost doctor exit export feedback hooks init login logout mcp memory model permissions plan plugin plugins rename resume review rewind status tasks terminal theme usage vim add-dir agents skills deep-research dataviz update-config verify debug code-review simplify batch fewer-permission-prompts loop claude-api workflow-authoring run run-skill-generator auto-mode-setup autocompact color output-style effort fast heapdump __remote-workflow workflow-launch-exec reload-plugins reload-skills security-review insights recap goal list-agents team-onboarding checkup proactive reset new settings name stats peers",
	"pi":            "help model models settings scoped-models export import share bug copy name session fork tree thinking clone trust login logout new compact resume reload hotkeys changelog quit skills llama",
	"codex-desktop": "approve archive chat cloud cloud-environment compact fast feedback fork goal ide-context init local mcp memories model new pet pin plan project reasoning rename review side status usage worktree 计划",
}

// Reuse app installation evidence rather than a CLI or leftover config folder.
var promptDesktopInstalled = func() bool {
	for _, a := range agent.All() {
		if a.ID == "codex" {
			return a.HasApp()
		}
	}
	return false
}

func promptTargets() []PromptTarget {
	byID := map[string]*agent.Agent{}
	for _, a := range agent.All() {
		if a.WSL == "" && a.Plugin == "" {
			byID[a.ID] = a
		}
	}
	var out []PromptTarget
	for _, id := range []string{"claude", "pi", "codex-desktop"} {
		t := PromptTarget{ID: id, family: id, Verification: "pending", Reload: "Restart the harness to load updated commands"}
		if id == "codex-desktop" {
			t.Name, t.Icon, t.dir, t.Shared, t.Detected = "Codex Desktop", "codex", codexDir(), true, promptDesktopInstalled()
			t.Reload = "Skills refresh automatically; restart Codex Desktop if the command is missing"
		} else if a := byID[id]; a != nil {
			t.Name, t.Icon, t.dir, t.Detected = a.Name, a.Icon, a.Dir, a.Detected() && !a.CLIMissing()
			if id == "claude" {
				t.dir = claudeDir()
			}
		}
		switch id {
		case "pi":
			t.Reload = "Run /reload to load updated commands"
		}
		out = append(out, t)
	}
	return out
}

func (t PromptTarget) folder() string {
	switch t.ID {
	case "codex-desktop":
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
	if t.ID == "codex-desktop" {
		dir := filepath.Join(t.folder(), name)
		return map[string][]byte{
			filepath.Join(dir, "SKILL.md"):              []byte("---\nname: " + quoted(name) + "\ndescription: " + quoted("Explicitly invoke /"+name+" to apply these instructions to the accompanying request.") + "\n---\n\n" + text + "\n"),
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
	i := slices.IndexFunc(all, func(t PromptTarget) bool { return t.ID == id })
	if i < 0 {
		return PromptTarget{}, false
	}
	return all[i], true
}
