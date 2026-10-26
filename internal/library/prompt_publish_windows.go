//go:build windows

package library

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Avoid the hard-link/delete publication sequence: later replacement of such
// files can fail on Windows. A same-directory move without REPLACE_EXISTING
// publishes the complete file and refuses a name another writer has claimed.
func publishPromptFile(stage, path string) error {
	move := func() error {
		from, err := promptPublishPath(stage)
		if err != nil {
			return err
		}
		to, err := promptPublishPath(path)
		if err != nil {
			return err
		}
		old, err := windows.UTF16PtrFromString(from)
		if err != nil {
			return err
		}
		next, err := windows.UTF16PtrFromString(to)
		if err != nil {
			return err
		}
		return windows.MoveFileEx(old, next, 0)
	}
	if err := move(); err != nil {
		return &os.LinkError{Op: "rename", Old: stage, New: path, Err: err}
	}
	return nil
}

// Native APIs need extended paths to preserve Go's long-path support, including
// UNC paths. Already extended/device paths retain their native interpretation.
func promptPublishPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return path, nil
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + path[2:], nil
	}
	return `\\?\` + path, nil
}
