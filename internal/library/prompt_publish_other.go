//go:build !windows

package library

import "os"

func publishPromptFile(stage, path string) error {
	return os.Link(stage, path)
}
