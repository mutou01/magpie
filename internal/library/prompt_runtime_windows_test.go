package library

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPromptTemplatePiRuntimeCancellationEndsWindowsProcess(t *testing.T) {
	h, _ := promptPiFixture(t)
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
