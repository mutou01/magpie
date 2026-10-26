package library

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/edit"
)

func TestPromptTemplatesPublishPreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "command.md")
	write(t, path, "User command")
	if err := writePromptFile(path, []byte("Managed command"), ""); err == nil {
		t.Fatal("initial publication accepted an existing unowned file")
	}
	if read(t, path) != "User command" {
		t.Fatal("initial publication replaced an existing unowned file")
	}
}

func TestPromptTemplatesPublishLongPathAndEdit(t *testing.T) {
	dir := t.TempDir()
	for len(dir) < 300 {
		dir = filepath.Join(dir, "提示词-publication")
	}
	paths := []string{filepath.Join(dir, "command.md")}
	if runtime.GOOS == "windows" {
		paths = append(paths, `\\?\`+filepath.Join(dir, "extended.md"))
	}
	for _, path := range paths {
		if err := writePromptFile(path, []byte("Initial body"), ""); err != nil {
			t.Fatalf("long-path initial publication: %v", err)
		}
		if err := writePromptFile(path, []byte("Edited body"), promptHash([]byte("Initial body"))); err != nil {
			t.Fatalf("long-path owned edit: %v", err)
		}
		if read(t, path) != "Edited body" {
			t.Fatal("long-path publication or owned edit lost contents")
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".prompt-") {
				t.Fatal("successful long-path publication left a staging file")
			}
		}
	}
}

func TestPromptTemplatesPublishPreservesFileCreatedAfterCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "command.md")
	intervened := false
	err := edit.Filtered(func(stage string, body []byte) []byte {
		if promptSamePath(filepath.Dir(stage), dir) && !promptSamePath(stage, path) {
			write(t, path, "Intervening user command")
			intervened = true
		}
		return body
	}, func() error { return writePromptFile(path, []byte("Managed command"), "") })
	if !intervened || err == nil {
		t.Fatalf("file created after initial conflict check was not refused: intervened=%v error=%v", intervened, err)
	}
	if read(t, path) != "Intervening user command" {
		t.Fatal("publication replaced a file created after its initial check")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "command.md" {
		t.Fatalf("failed publication left a staging file: %v %v", entries, err)
	}
}

func TestPromptTemplatesConcurrentPublishHasOneWinner(t *testing.T) {
	dir := t.TempDir()
	for round := 0; round < 32; round++ {
		path := filepath.Join(dir, "command.md")
		start := make(chan struct{})
		type result struct {
			body []byte
			err  error
		}
		results := make(chan result, 2)
		for _, text := range []string{"First publisher", "Second publisher"} {
			go func(text string) {
				<-start
				body := []byte(text)
				results <- result{body, writePromptFile(path, body, "")}
			}(text)
		}
		close(start)
		first, second := <-results, <-results
		if (first.err == nil) == (second.err == nil) {
			t.Fatalf("round %d expected exactly one successful publisher: %v / %v", round, first.err, second.err)
		}
		winner := first
		if winner.err != nil {
			winner = second
		}
		body, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(body, winner.body) {
			t.Fatalf("round %d losing publication changed winning contents: %v", round, err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}
