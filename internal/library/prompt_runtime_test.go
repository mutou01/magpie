package library

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A real child process speaking Pi 0.87.1's line-delimited RPC protocol.
// It rejects any attempt to send a prompt rather than enumerate commands.
func fakePromptPi() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		os.WriteFile(os.Getenv("MAGPIE_FAKE_PI")+".version", []byte("started"), 0600)
		v, _ := os.ReadFile(filepath.Join(os.Getenv("USERPROFILE"), "pi-version"))
		if len(v) == 0 {
			v = []byte("0.87.1")
		}
		fmt.Println(string(v))
		return
	}
	cwd, _ := os.Getwd()
	f, _ := os.OpenFile(os.Getenv("MAGPIE_FAKE_PI"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if f != nil {
		fmt.Fprintln(f, cwd)
		f.Close()
	}
	mode, _ := os.ReadFile(filepath.Join(os.Getenv("USERPROFILE"), "pi-mode"))
	switch string(mode) {
	case "partial":
		fmt.Fprintln(os.Stderr, "Failed to load extension fixture.ts")
	case "invalid":
		fmt.Println("not a protocol response")
		return
	case "oversized":
		fmt.Println(strings.Repeat("x", 2<<20))
		return
	case "interaction":
		fmt.Println(`{"type":"extension_ui_request","id":"trust","method":"confirm"}`)
	case "extension-error":
		fmt.Println(`{"type":"extension_error","error":"session_start failed"}`)
	case "hold":
		os.WriteFile(filepath.Join(os.Getenv("USERPROFILE"), "pi-pid"), []byte(fmt.Sprint(os.Getpid())), 0600)
		for {
			if _, err := os.Stat(filepath.Join(os.Getenv("USERPROFILE"), "pi-release")); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		var req struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if json.Unmarshal(s.Bytes(), &req) != nil || req.Type != "get_commands" {
			os.Exit(9)
		}
		var commands []map[string]any
		b, _ := os.ReadFile(filepath.Join(cwd, "pi-commands.json"))
		if len(b) == 0 {
			b = []byte(`[]`)
		}
		if json.Unmarshal(b, &commands) != nil {
			os.Exit(8)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"id": req.ID, "type": "response", "command": "get_commands", "success": true, "data": map[string]any{"commands": commands}})
	}
}

func piPromptState(t *testing.T, c PromptChange) PromptState {
	t.Helper()
	states, err := CheckPrompt(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Agent == "pi" {
			return s
		}
	}
	t.Fatal("Pi missing from availability check")
	return PromptState{}
}

func TestPromptTemplatePiRuntimeRejectsUnverifiedVersionAndUntrustedProject(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		h, _ := promptPiFixture(t)
		write(t, filepath.Join(h, "pi-version"), "9.0.0")
		if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "unverified" {
			t.Fatalf("unverified Pi version accepted: %+v", s)
		}
	})
	t.Run("project trust", func(t *testing.T) {
		h, _ := promptPiFixture(t)
		project := filepath.Join(h, "work")
		write(t, filepath.Join(project, ".pi/extensions/project.ts"), "export default pi => {}")
		ok(t)(AddProject(project))
		if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "unverified" {
			t.Fatalf("silently skipped project extension accepted: %+v", s)
		}
		if read(t, filepath.Join(h, ".pi/agent/trust.json")) != "" {
			t.Fatal("check changed project trust")
		}
	})
}

func TestPromptTemplatePiRuntimeUsesRegisteredNamesAndFreshChecks(t *testing.T) {
	h, log := promptPiFixture(t)
	write(t, filepath.Join(h, "pi-commands.json"), `[{"name":"skill:d","source":"skill"}]`)
	if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "available" {
		t.Fatalf("skill namespace is not /d: %+v", s)
	}
	before := read(t, log)
	write(t, filepath.Join(h, "pi-commands.json"), `[{"name":"d","source":"extension","sourceInfo":{"path":"fixture-extension.ts"}}]`)
	if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "conflict" || !strings.Contains(s.Source, "fixture-extension.ts") {
		t.Fatalf("manual check reused stale commands: %+v", s)
	}
	if read(t, log) == before {
		t.Fatal("repeated explicit check did not start Pi")
	}
}

func promptPiFixture(t *testing.T) (string, string) {
	t.Helper()
	h := sandbox(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	name := "pi"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(h, "bin", name), b, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(h, "pi-runs")
	t.Setenv("MAGPIE_FAKE_PI", log)
	write(t, filepath.Join(h, ".pi/agent/extensions/observer.ts"), `export default pi => {}`)
	return h, log
}

func TestPromptTemplatePiRuntimeChecksCommandsAndReusesResults(t *testing.T) {
	_, log := promptPiFixture(t)
	states, err := CheckPrompt(PromptChange{Name: "d"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Agent == "pi" && s.Status != "available" {
			t.Fatalf("extension without /d should be available: %+v", s)
		}
	}
	if read(t, log) == "" {
		t.Fatal("explicit check did not ask native Pi for commands")
	}
	before := read(t, log)
	ok(t)(SavePrompt(PromptChange{Name: "审查", Text: "body", Agents: []string{"pi"}}))
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range v.Items[0].States {
		if s.Agent == "pi" && s.Status != "enabled" {
			t.Fatalf("save should reuse command list for another name: %+v", s)
		}
	}
	ok(t)(Sync())
	if read(t, log) != before {
		t.Fatal("save, read or normal sync started Pi")
	}
}

func TestPromptTemplatePiRuntimeManagedCommandsDoNotSurviveRenameInCache(t *testing.T) {
	h, log := promptPiFixture(t)
	if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "available" {
		t.Fatal(s)
	}
	ok(t)(SavePrompt(PromptChange{Name: "d", Text: "body", Agents: []string{"pi"}}))
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	id := v.Items[0].ID
	commands, _ := json.Marshal([]map[string]any{{"name": "d", "source": "prompt", "sourceInfo": map[string]string{"path": filepath.Join(h, ".pi/agent/prompts/d.md")}}})
	write(t, filepath.Join(h, "pi-commands.json"), string(commands))
	if s := piPromptState(t, PromptChange{ID: id, Name: "d", Text: "body"}); s.Status != "enabled" {
		t.Fatal(s)
	}
	before := read(t, log)
	ok(t)(SavePrompt(PromptChange{ID: id, Name: "renamed", Text: "body", Agents: []string{"pi"}}))
	if s := piPromptState(t, PromptChange{Name: "d", Passive: true}); s.Status != "available" {
		t.Fatalf("deleted managed command remained in cache: %+v", s)
	}
	if read(t, log) != before {
		t.Fatal("rename or passive check started Pi")
	}
}

func TestPromptTemplatePiRuntimeFailuresInvalidateEarlierSuccess(t *testing.T) {
	for _, mode := range []string{"partial", "invalid", "oversized", "interaction", "extension-error"} {
		t.Run(mode, func(t *testing.T) {
			h, log := promptPiFixture(t)
			if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "available" {
				t.Fatal(s)
			}
			write(t, filepath.Join(h, "pi-mode"), mode)
			if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "unverified" {
				t.Fatalf("partial/failed runtime accepted: %+v", s)
			}
			before := read(t, log)
			if s := piPromptState(t, PromptChange{Name: "d", Passive: true}); s.Status != "unverified" {
				t.Fatalf("failed check fell back to earlier success: %+v", s)
			}
			if read(t, log) != before {
				t.Fatal("passive check started Pi")
			}
		})
	}
}

func TestPromptTemplatePiRuntimeDoesNotBlockEditsAndRejectsChangedSources(t *testing.T) {
	h, log := promptPiFixture(t)
	write(t, filepath.Join(h, "pi-mode"), "hold")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan []PromptState, 1)
	go func() { s, _ := CheckPromptContext(ctx, PromptChange{Name: "d"}); done <- s }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("check process outlived cancellation")
		}
	})
	deadline := time.After(10 * time.Second)
	for read(t, log) == "" {
		select {
		case <-deadline:
			t.Fatal("Pi did not start")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	saved := make(chan error, 1)
	go func() {
		_, err := SavePrompt(PromptChange{Name: "another", Text: "text", Agents: []string{"claude"}})
		saved <- err
	}()
	select {
	case err := <-saved:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runtime check held Library lock")
	}
	write(t, filepath.Join(h, ".pi/agent/extensions/observer.ts"), "changed during startup")
	write(t, filepath.Join(h, "pi-release"), "yes")
	select {
	case states := <-done:
		done <- nil // cleanup has a completed result to consume
		for _, s := range states {
			if s.Agent == "pi" && s.Status != "unverified" {
				t.Fatalf("changed sources accepted: %+v", s)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("released runtime check did not finish")
	}
}

func TestPromptTemplatePiRuntimePassiveSourceChangePreservesDeployment(t *testing.T) {
	h, log := promptPiFixture(t)
	if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "available" {
		t.Fatal(s)
	}
	ok(t)(SavePrompt(PromptChange{Name: "d", Text: "old", Agents: []string{"pi"}}))
	v, err := ReadPrompts()
	if err != nil {
		t.Fatal(err)
	}
	before := read(t, log)
	write(t, filepath.Join(h, ".pi/agent/extensions/new.ts"), "export default pi => {}")
	if s := piPromptState(t, PromptChange{ID: v.Items[0].ID, Name: "d", Passive: true}); s.Status != "unverified" {
		t.Fatal(s)
	}
	_, err = Sync()
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(h, ".pi/agent/prompts/d.md")) != "old\n\n$ARGUMENTS\n" {
		t.Fatal("unknown result removed an existing deployment")
	}
	if read(t, log) != before {
		t.Fatal("source change silently started Pi")
	}
}

func TestPromptTemplatePiRuntimeMissingSourcesStayUnknown(t *testing.T) {
	for _, settings := range []string{`{"extensions":["./missing.ts"]}`, `{"packages":["npm:missing-fixture"]}`, `{"packages":["./missing-package"]}`} {
		t.Run(settings, func(t *testing.T) {
			h, _ := promptPiFixture(t)
			write(t, filepath.Join(h, ".pi/agent/settings.json"), settings)
			if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "unverified" {
				t.Fatalf("offline Pi may silently skip missing sources: %+v", s)
			}
		})
	}
}

func TestPromptTemplatePiRuntimeReadFailureDiscardsCache(t *testing.T) {
	h, _ := promptPiFixture(t)
	if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "available" {
		t.Fatal(s)
	}
	path := filepath.Join(h, ".pi/agent/settings.json")
	write(t, path, "invalid JSON")
	if s := piPromptState(t, PromptChange{Name: "d", Passive: true}); s.Status != "unverified" {
		t.Fatal(s)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if s := piPromptState(t, PromptChange{Name: "d", Passive: true}); s.Status != "unverified" {
		t.Fatalf("read failure must discard previous success: %+v", s)
	}
}

func TestPromptTemplatePiRuntimeNewestCheckWins(t *testing.T) {
	h, log := promptPiFixture(t)
	write(t, filepath.Join(h, "pi-mode"), "hold")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { CheckPromptContext(ctx, PromptChange{Name: "d"}); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.After(10 * time.Second)
	for read(t, log) == "" {
		select {
		case <-deadline:
			t.Fatal("first check did not start")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	write(t, filepath.Join(h, "pi-mode"), "")
	if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "available" {
		t.Fatal(s)
	}
	write(t, filepath.Join(h, "pi-commands.json"), `[{"name":"d","source":"extension"}]`)
	write(t, filepath.Join(h, "pi-release"), "yes")
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("older check did not finish")
	}
	if s := piPromptState(t, PromptChange{Name: "d", Passive: true}); s.Status != "available" {
		t.Fatalf("older command result overwrote latest check: %+v", s)
	}
}

func TestPromptTemplatePiRuntimeHonorsExistingProjectTrust(t *testing.T) {
	h, log := promptPiFixture(t)
	project := filepath.Join(h, "work")
	write(t, filepath.Join(project, ".pi/extensions/project.ts"), "export default pi => {}")
	write(t, filepath.Join(project, "pi-commands.json"), `[{"name":"d","source":"extension","sourceInfo":{"path":"project-extension.ts"}}]`)
	ok(t)(AddProject(project))
	trust, _ := json.Marshal(map[string]bool{h: true})
	trustPath := filepath.Join(h, ".pi/agent/trust.json")
	write(t, trustPath, string(trust))
	if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "conflict" || s.Source != "project-extension.ts" {
		t.Fatalf("trusted project's command was omitted: %+v", s)
	}
	if got := len(strings.Split(strings.TrimSpace(read(t, log)), "\n")); got != 2 {
		t.Fatalf("expected two distinct working directories, got %d", got)
	}
	if read(t, trustPath) != string(trust) {
		t.Fatal("runtime check changed existing trust")
	}
}

func TestPromptTemplatePiRuntimeTimeoutStaysUnknown(t *testing.T) {
	h, _ := promptPiFixture(t)
	write(t, filepath.Join(h, "pi-mode"), "hold")
	previous := piCheckBudget
	piCheckBudget = 3 * time.Second
	t.Cleanup(func() { piCheckBudget = previous })
	if s := piPromptState(t, PromptChange{Name: "d"}); s.Status != "unverified" || !strings.Contains(s.Reason, "timed out") {
		t.Fatalf("timeout must not accept an empty command list: %+v", s)
	}
}

func TestPromptTemplatePiRuntimeAlreadyCancelledDoesNotStart(t *testing.T) {
	_, log := promptPiFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CheckPromptContext(ctx, PromptChange{Name: "d"}); err != nil {
		t.Fatal(err)
	}
	if read(t, log+".version") != "" || read(t, log) != "" {
		t.Fatal("already cancelled request started Pi")
	}
}

func TestPromptTemplatePiRuntimeCancellationPrecedesProcessLaunch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A nonexistent binary makes attempting a launch observably different from
	// respecting cancellation first, without relying on child scheduling.
	for _, check := range []func() error{
		func() error { _, err := piCommandVersion(ctx, "missing-pi-fixture", t.TempDir()); return err },
		func() error {
			_, err := readPiCommands(ctx, "missing-pi-fixture", t.TempDir(), t.TempDir())
			return err
		},
	} {
		var err *promptRuntimeError
		if !errors.As(check(), &err) || !strings.Contains(err.reason, "cancelled") {
			t.Fatalf("cancelled probe attempted process launch: %v", err)
		}
	}
}
