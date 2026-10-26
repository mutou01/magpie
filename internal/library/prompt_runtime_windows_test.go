package library

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPromptTemplatePiRuntimeCancellationEndsWindowsProcess(t *testing.T) {
	h, _ := promptEnabledPiFixture(t)
	write(t, filepath.Join(h, "pi-mode"), "hold")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan []PromptState, 1)
	go func() { s, _ := CheckPromptContext(ctx, PromptChange{Name: "d"}); done <- s }()
	deadline := time.After(10 * time.Second)
	var pid uint64
	for pid == 0 {
		pid, _ = strconv.ParseUint(strings.TrimSpace(read(t, filepath.Join(h, "pi-pid"))), 10, 32)
		select {
		case <-deadline:
			t.Fatal("Pi did not start")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	cancel()
	select {
	case states := <-done:
		for _, s := range states {
			if s.Agent == "pi" && s.Status != "unverified" {
				t.Fatal(s)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled check did not finish")
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err == nil {
		defer windows.CloseHandle(handle)
		status, err := windows.WaitForSingleObject(handle, 0)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatalf("Pi process %d survived cancellation: %d, %v", pid, status, err)
		}
	}
}

func TestPromptTemplatesRemovalRetriesUnreadableCodexDeploymentAfterUnlock(t *testing.T) {
	home, _ := promptEnabledCodexFixture(t)
	if _, err := CheckPrompt(PromptChange{Name: "audit-retry", Target: "codex"}); err != nil {
		t.Fatal(err)
	}
	ok(t)(SavePrompt(PromptChange{Name: "audit-retry", Text: "Original body", Agents: []string{"codex"}}))
	view, err := ReadPrompts()
	if err != nil || len(view.Items) != 1 {
		t.Fatalf("fixture missing template: %+v %v", view, err)
	}
	root := filepath.Join(home, ".codex", "skills", "audit-retry")
	bodyPath, policyPath := filepath.Join(root, "SKILL.md"), filepath.Join(root, "agents", "openai.yaml")
	body, policy := read(t, bodyPath), read(t, policyPath)
	path, err := windows.UTF16PtrFromString(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if handle != windows.InvalidHandle {
			windows.CloseHandle(handle)
		}
	}()
	if _, err := os.ReadFile(policyPath); err == nil {
		t.Fatal("fixture must deny policy reads")
	}
	result, err := RemovePrompt(view.Items[0].ID)
	if err != nil || len(result.Problems) == 0 {
		t.Fatalf("temporary unreadability must retain and report the deployment: %+v %v", result, err)
	}
	if read(t, bodyPath) != body {
		t.Fatal("unreadable policy retired the readable body")
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = windows.InvalidHandle
	if read(t, policyPath) != policy {
		t.Fatal("removal changed the retained policy")
	}
	result, err = Sync()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{bodyPath, policyPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("unlock followed by Sync must retire the still-owned group: %s: %v; result=%+v", filepath.Base(path), err, result)
		}
	}
}
