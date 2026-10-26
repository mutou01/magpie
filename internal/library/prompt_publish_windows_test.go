//go:build windows

package library

import (
	"path/filepath"
	"testing"
)

func TestPromptTemplatesPublishWindowsNativePaths(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{`C:\提示词\command.md`, `\\?\C:\提示词\command.md`},
		{`\\server\share\提示词\command.md`, `\\?\UNC\server\share\提示词\command.md`},
		{`\\?\C:\提示词\command.md`, `\\?\C:\提示词\command.md`},
		{`\\?\UNC\server\share\command.md`, `\\?\UNC\server\share\command.md`},
		{`\\.\C:\提示词\command.md`, `\\.\C:\提示词\command.md`},
	} {
		got, err := promptPublishPath(tc.path)
		if err != nil || got != tc.want {
			t.Errorf("native path %q: got %q, want %q, error %v", tc.path, got, tc.want, err)
		}
	}
	abs, err := filepath.Abs(filepath.Join("relative", "command.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := promptPublishPath(filepath.Join("relative", "command.md"))
	if err != nil || got != `\\?\`+abs {
		t.Fatalf("relative native path: %q %v", got, err)
	}
}
