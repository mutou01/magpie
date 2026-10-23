package library

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yetone/magpie/internal/edit"
)

// promptStore is local, including ownership. Unlike library.json, older
// binaries never rewrite it. Bundle still carries only portable templates.
type promptStore struct {
	Version int                                  `json:"version"`
	Prompts []*Prompt                            `json:"prompts"`
	Applied map[string]map[string]*AppliedPrompt `json:"applied"`
}

func promptStorePath() string { return filepath.Join(Dir(), "prompts.json") }

func (l *Library) loadPromptStore() error {
	b, err := os.ReadFile(promptStorePath())
	if os.IsNotExist(err) {
		return nil // migrate library.json on the next successful save
	}
	if err != nil {
		return err
	}
	var s promptStore
	if err := json.Unmarshal(edit.TrimBOM(b), &s); err != nil {
		return fmt.Errorf("%s: %w", promptStorePath(), err)
	}
	if s.Version != 1 || s.Prompts == nil || s.Applied == nil {
		return fmt.Errorf("%s: incomplete or unsupported prompt index", promptStorePath())
	}
	if err := checkPromptRecords(s.Prompts); err != nil {
		return fmt.Errorf("%s: %w", promptStorePath(), err)
	}
	for agent, prompts := range s.Applied {
		if err := checkAgent(agent); err != nil || prompts == nil {
			return fmt.Errorf("%s: invalid prompt ownership for %s", promptStorePath(), agent)
		}
		for id, applied := range prompts {
			if err := checkName("prompt id", id); err != nil || applied == nil || applied.Files == nil {
				return fmt.Errorf("%s: incomplete prompt ownership for %s/%s", promptStorePath(), agent, id)
			}
			for path, hash := range applied.Files {
				b, err := hex.DecodeString(hash)
				if !filepath.IsAbs(path) || err != nil || len(b) != 32 {
					return fmt.Errorf("%s: invalid deployment path or hash for %s/%s", promptStorePath(), agent, id)
				}
			}
		}
	}
	l.Prompts = s.Prompts
	// Empty is authoritative too; never revive stale mirror ownership.
	for _, a := range l.Applied {
		a.Prompts = nil
	}
	for id, prompts := range s.Applied {
		l.applied(id).Prompts = prompts
	}
	return nil
}

func (l *Library) savePromptStore() error {
	s := promptStore{Version: 1, Prompts: l.Prompts, Applied: map[string]map[string]*AppliedPrompt{}}
	if s.Prompts == nil {
		s.Prompts = []*Prompt{}
	}
	for id, a := range l.Applied {
		if len(a.Prompts) > 0 {
			s.Applied[id] = a.Prompts
		}
	}
	old, err := os.ReadFile(promptStorePath())
	if os.IsNotExist(err) && len(s.Prompts) == 0 && len(s.Applied) == 0 {
		return nil // a library that has never had templates needs no index
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if bytes.Equal(edit.TrimBOM(old), b) {
		return nil
	}
	if l.kept == nil {
		l.kept = newBackups()
	}
	if err := l.kept.keep("library", promptStorePath()); err != nil {
		return err
	}
	return edit.WriteAtomic(promptStorePath(), b)
}
