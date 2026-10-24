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

func TestPromptTemplateOriginalDSHCancellationEndsNativeProcess(t *testing.T) {
	dir, node := os.Getenv("MAGPIE_VERIFY_DSH_HOME"), os.Getenv("MAGPIE_VERIFY_NODE_BIN")
	if dir == "" || node == "" {
		t.Skip("opt-in original Windows DSH process cancellation")
	}
	h := sandbox(t)
	t.Setenv("PATH", filepath.Join(h, "bin")+string(os.PathListSeparator)+filepath.Dir(node)+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	isolated := t.TempDir()
	write(t, filepath.Join(isolated, "profiles/web/package.json"), `{"dsh":{"profile":{"bundles":["@deepseek-ai/dsh-base","@deepseek-ai/dsh-web-app"]}}}`)
	if err := os.Symlink(filepath.Join(dir, "profiles/node_modules"), filepath.Join(isolated, "profiles/node_modules")); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(isolated, "profiles/web/node_modules/magpie-cancellation-fixture")
	write(t, filepath.Join(plugin, "package.json"), `{"name":"magpie-cancellation-fixture","version":"1.0.0","type":"module","main":"index.mjs"}`)
	write(t, filepath.Join(plugin, "index.mjs"), "import {writeFileSync} from 'node:fs';\nexport async function apply(){writeFileSync(process.env.MAGPIE_DSH_CANCELLATION_PID,String(process.pid));await new Promise(()=>{});}\n")
	write(t, filepath.Join(isolated, "cordis.patch.yml"), "- insert:\n    - id: cancellation-fixture\n      name: magpie-cancellation-fixture\n")
	pidFile := filepath.Join(t.TempDir(), "native.pid")
	t.Setenv("MAGPIE_DSH_CANCELLATION_PID", pidFile)
	t.Setenv("DSH_HOME", isolated)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var states []PromptState
	var checkErr error
	go func() {
		states, checkErr = CheckPromptContext(ctx, PromptChange{Name: "cancelled-template"})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("cancelled native check did not stop during cleanup")
		}
	})
	deadline := time.After(45 * time.Second)
	var pid uint64
	for pid == 0 {
		b, _ := os.ReadFile(pidFile)
		pid, _ = strconv.ParseUint(strings.TrimSpace(string(b)), 10, 32)
		select {
		case <-deadline:
			t.Fatal("original native plugin did not reach its startup barrier")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	// Holding a real process handle proves termination even if Windows would
	// otherwise recycle its PID before the public check returns.
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatalf("original native process exited before cancellation: %v", err)
	}
	defer windows.CloseHandle(handle)
	cancel()
	select {
	case <-done:
		if checkErr != nil {
			t.Fatal(checkErr)
		}
		found := false
		for _, state := range states {
			if state.Agent == "dsh" {
				found = true
				if state.Status != "unverified" {
					t.Fatalf("cancelled native check retained availability: %+v", state)
				}
			}
		}
		if !found {
			t.Fatal("original DSH was not detected")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("cancelled native check did not return")
	}
	status, err := windows.WaitForSingleObject(handle, 0)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("original native process survived cancellation: status=%d, error=%v", status, err)
	}
	states, err = CheckPrompt(PromptChange{Name: "cancelled-template", Passive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Agent == "dsh" && state.Status != "unverified" {
			t.Fatalf("passive read revived cancelled native evidence: %+v", state)
		}
	}
}
