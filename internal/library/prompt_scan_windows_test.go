package library

import (
	"fmt"
	"path/filepath"
	"testing"
	unsafePointer "unsafe"

	"golang.org/x/sys/windows"
)

// Observe actual OS file reads around the public save, without replacing a
// production filesystem function. The wide 1-vs-8 fixture gap distinguishes
// rereading 256 unrelated command bodies per template from bounded per-phase
// source discovery; it does not assert elapsed time on a loaded machine.
func TestPromptTemplateSaveDoesNotRescanAllSourcesForEveryTemplate(t *testing.T) {
	getIO := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessIoCounters")
	type counters struct{ reads, writes, other, readBytes, writeBytes, otherBytes uint64 }
	reads := func(t *testing.T) uint64 {
		t.Helper()
		var c counters
		ok, _, err := getIO.Call(^uintptr(0), uintptr(unsafePointer.Pointer(&c)))
		if ok == 0 {
			t.Fatal("cannot observe process file reads", err)
		}
		return c.reads
	}
	measure := func(t *testing.T, count int) uint64 {
		t.Helper()
		home := promptEnabledSandbox(t)
		for i := 0; i < count; i++ {
			ok(t)(SavePrompt(PromptChange{Name: fmt.Sprintf("scan-template-%d", i), Text: "Initial body", Agents: []string{"claude"}}))
		}
		view, err := ReadPrompts()
		if err != nil || len(view.Items) != count {
			t.Fatalf("public fixture setup: %v %v", view, err)
		}
		p := view.Items[0]
		for i := 0; i < 256; i++ {
			write(t, filepath.Join(home, ".claude", "commands", fmt.Sprintf("foreign-source-%d.md", i)), "Unrelated user command\n")
		}
		before := reads(t)
		result, err := SavePrompt(PromptChange{ID: p.ID, Name: p.Name, Text: "Edited body", Agents: []string{"claude"}})
		delta := reads(t) - before
		if err != nil || len(result.Problems) != 0 {
			t.Fatalf("timed public save must succeed: %+v %v", result, err)
		}
		if got := read(t, filepath.Join(home, ".claude", "commands", p.Name+".md")); got == "" {
			t.Fatal("public save did not retain the deployment")
		}
		return delta
	}
	var one, eight uint64
	t.Run("one-template", func(t *testing.T) { one = measure(t, 1) })
	t.Run("eight-templates", func(t *testing.T) { eight = measure(t, 8) })
	if one == 0 || eight == 0 {
		t.Fatal("both public saves must have observable file reads")
	}
	t.Logf("public save file read operations: one=%d eight=%d", one, eight)
	if eight >= 2*one {
		t.Fatalf("saving one edit must not reread all 256 unrelated sources per template: one=%d eight=%d", one, eight)
	}
}
