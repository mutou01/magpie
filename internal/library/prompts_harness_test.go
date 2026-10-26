package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPromptTemplatesHarnessTargetOnlyCacheAndDisable(t *testing.T) {
	home, starts := promptEnabledCodexFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Body", Agents: []string{}}))
	res, err := SetPromptHarnessContext(context.Background(), "codex", true)
	if err != nil || len(res.Problems) != 0 {
		t.Fatalf("enable: %+v %v", res, err)
	}
	before := read(t, starts)
	if before == "" {
		t.Fatal("enabling did not verify the requested target")
	}
	piStarts := os.Getenv("MAGPIE_FAKE_PI")
	if read(t, piStarts) != "" || read(t, piStarts+".version") != "" {
		t.Fatal("Codex enable started unrelated Pi")
	}
	ok(t)(SetPromptHarnessContext(context.Background(), "codex", true))
	if read(t, starts) != before {
		t.Fatal("repeated enable failed to reuse valid evidence")
	}
	ok(t)(SetPromptHarnessContext(context.Background(), "codex", false))
	if read(t, starts) != before {
		t.Fatal("disable started Codex")
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "audit-code", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("disable kept unchanged owned body", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "audit-code", "agents", "openai.yaml")); !os.IsNotExist(err) {
		t.Fatal("disable kept unchanged owned policy", err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "later-template", Text: "Later body", Agents: []string{}}))
	view, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range view.Items {
		if len(p.Agents) != 0 {
			t.Fatalf("batch introduced a future default: %+v", p)
		}
	}
}

func TestPromptTemplatesHarnessEmptyOffAndCancellation(t *testing.T) {
	_, starts := promptEnabledCodexFixture(t)
	for _, enabled := range []bool{true, false} {
		ok(t)(SetPromptHarnessContext(context.Background(), "codex", enabled))
	}
	if read(t, starts) != "" {
		t.Fatal("empty batch started a Harness")
	}
	ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Body", Agents: []string{}}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SetPromptHarnessContext(ctx, "codex", true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled operation accepted: %v", err)
	}
	ok(t)(SetPromptsEnabledContext(context.Background(), false))
	for _, enabled := range []bool{true, false} {
		if _, err := SetPromptHarnessContext(context.Background(), "codex", enabled); err == nil {
			t.Fatal("batch accepted while global switch is off")
		}
	}
	if read(t, starts) != "" {
		t.Fatal("rejected batch started a Harness")
	}
	view, err := ReadPrompts()
	if err != nil || len(view.Items) != 1 || len(view.Items[0].Agents) != 0 {
		t.Fatalf("rejected batch changed selection: %+v %v", view, err)
	}
}

func TestPromptTemplatesHarnessConcurrentCreateAndLatestBody(t *testing.T) {
	home, _ := promptEnabledPiFixture(t)
	ok(t)(SavePrompt(PromptChange{Name: "before-check", Text: "Old body", Agents: []string{}}))
	view, _ := ReadPrompts()
	id := view.Items[0].ID
	write(t, filepath.Join(home, "pi-mode"), "hold")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := SetPromptHarnessContext(ctx, "pi", true); done <- err }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("batch process did not end")
		}
	}()
	deadline := time.After(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(home, "pi-pid")); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("native check did not reach held fixture")
		case <-time.After(10 * time.Millisecond):
		}
	}
	ok(t)(SavePrompt(PromptChange{ID: id, Name: "after-check", Text: "Latest body", Agents: []string{}}))
	ok(t)(SavePrompt(PromptChange{Name: "created-during-check", Text: "New body", Agents: []string{}}))
	write(t, filepath.Join(home, "pi-release"), "yes")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
		// Leave a completion token for deferred cleanup's unconditional join.
		done <- nil
	case <-time.After(10 * time.Second):
		t.Fatal("batch did not complete after release")
	}
	view, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range view.Items {
		if p.ID == id {
			if p.Name != "after-check" || p.Text != "Latest body" || !slices.Equal(p.Agents, []string{"pi"}) {
				t.Fatalf("batch used stale form: %+v", p)
			}
		} else if len(p.Agents) != 0 {
			t.Fatalf("new template incorrectly joined in-flight batch: %+v", p)
		}
	}
	if !strings.Contains(read(t, filepath.Join(home, ".pi", "agent", "prompts", "after-check.md")), "Latest body") {
		t.Fatal("latest body not deployed")
	}
	if _, err := os.Stat(filepath.Join(home, ".pi", "agent", "prompts", "before-check.md")); !os.IsNotExist(err) {
		t.Fatal("stale name deployed", err)
	}
}

func TestPromptTemplatesHarnessCodexExternalEditProtectsWholeGroup(t *testing.T) {
	for _, edited := range []string{"SKILL.md", "agents/openai.yaml"} {
		t.Run(edited, func(t *testing.T) {
			home, starts := promptEnabledCodexFixture(t)
			ok(t)(SavePrompt(PromptChange{Name: "audit-code", Text: "Body", Agents: []string{}}))
			ok(t)(SetPromptHarnessContext(context.Background(), "codex", true))
			root := filepath.Join(home, ".codex", "skills", "audit-code")
			path := filepath.Join(root, filepath.FromSlash(edited))
			write(t, path, read(t, path)+"\n# External edit\n")
			body, policy := read(t, filepath.Join(root, "SKILL.md")), read(t, filepath.Join(root, "agents", "openai.yaml"))
			before := read(t, starts)
			res, err := SetPromptHarnessContext(context.Background(), "codex", false)
			if err != nil || len(res.Problems) == 0 {
				t.Fatalf("external edit not reported: %+v %v", res, err)
			}
			if read(t, filepath.Join(root, "SKILL.md")) != body || read(t, filepath.Join(root, "agents", "openai.yaml")) != policy {
				t.Fatal("batch disable modified externally edited file group")
			}
			if read(t, starts) != before {
				t.Fatal("disable started native verification")
			}
			view, err := ReadPrompts()
			if err != nil || len(view.Items[0].Agents) != 0 {
				t.Fatalf("explicit disable did not persist selection removal: %+v %v", view, err)
			}
			res, err = SetPromptHarnessContext(context.Background(), "codex", true)
			if err != nil || len(res.Problems) == 0 {
				t.Fatalf("enable did not report protected edit: %+v %v", res, err)
			}
			if read(t, filepath.Join(root, "SKILL.md")) != body || read(t, filepath.Join(root, "agents", "openai.yaml")) != policy {
				t.Fatal("batch re-enable overwrote externally edited file group")
			}
		})
	}
}
