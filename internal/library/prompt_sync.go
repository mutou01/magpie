package library

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/yetone/magpie/internal/edit"
)

// AppliedPrompt owns individual files, never the surrounding skill directory.
type AppliedPrompt struct {
	Files map[string]string `json:"files"` // physical path → last written hash
}

func promptHash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func (l *Library) syncPrompts(b *backups, res *Result) {
	all := promptTargets()
	for _, t := range all {
		for _, p := range l.Prompts {
			if !slices.Contains(p.Agents, t.ID) || !t.Detected {
				continue
			}
			body, err := os.ReadFile(promptPath(p.ID))
			if err == nil {
				err = checkPromptText(string(body))
			}
			if err != nil {
				res.fail(t.ID, "prompt:"+p.Name, err)
				continue
			}
			files, err := t.files(p, string(body))
			if err != nil {
				res.fail(t.ID, "prompt:"+p.Name, err)
				continue
			}
			if state := l.promptState(t, p, string(body)); !promptUsable(state) {
				res.fail(t.ID, "prompt:"+p.Name, fmt.Errorf("%s: %s", state.Reason, state.Source))
				continue
			}
			a := l.applied(t.ID)
			if a.Prompts == nil {
				a.Prompts = map[string]*AppliedPrompt{}
			}
			old := a.Prompts[p.ID]
			if old == nil {
				old = &AppliedPrompt{Files: map[string]string{}}
				a.Prompts[p.ID] = old
			}
			if old.Files == nil {
				old.Files = map[string]string{}
			}
			paths := slices.Sorted(maps.Keys(files))
			// Publish a skill's explicit-only policy before its discoverable body.
			slices.SortStableFunc(paths, func(a, b string) int {
				if filepath.Base(a) == "SKILL.md" {
					return 1
				}
				if filepath.Base(b) == "SKILL.md" {
					return -1
				}
				return strings.Compare(a, b)
			})
			for _, path := range paths {
				data := files[path]
				ownedHash := l.promptOwnedHash(t.ID, p.ID, path)
				cur, err := os.ReadFile(path)
				if err != nil && !os.IsNotExist(err) {
					res.fail(t.ID, "prompt:"+p.Name, err)
					continue
				}
				if err == nil && (ownedHash == "" || promptHash(cur) != ownedHash) {
					res.fail(t.ID, "prompt:"+p.Name, fmt.Errorf("user-owned or externally edited file: %s", path))
					continue
				}
				if err == nil && old.Files[path] != "" && promptHash(cur) == promptHash(data) {
					continue
				}
				if err := b.keep(t.ID+"-"+p.ID, path); err != nil {
					res.fail(t.ID, "prompt:"+p.Name, err)
					continue
				}
				var renameErr error
				if runtime.GOOS == "windows" {
					for alias := range old.Files {
						if alias != path && strings.EqualFold(alias, path) && filepath.Base(alias) != filepath.Base(path) && promptSamePath(alias, path) {
							renameErr = os.Rename(alias, path)
							break
						}
					}
				}
				if renameErr != nil {
					res.fail(t.ID, "prompt:"+p.Name, renameErr)
					continue
				}
				if err := writePromptFile(path, data, ownedHash); err != nil {
					res.fail(t.ID, "prompt:"+p.Name, err)
					continue
				}
				for alias := range old.Files {
					if alias != path && promptSamePath(alias, path) {
						delete(old.Files, alias)
					}
				}
				old.Files[path] = promptHash(data)
				res.changed(t.ID)
			}
		}
	}
	// Retire only owned, unedited files after a rename, deselection or removal.
	for id, a := range l.Applied {
		for pid, old := range a.Prompts {
			if old == nil {
				delete(a.Prompts, pid)
				continue
			}
			var desired map[string][]byte
			if p := l.prompt(pid); p != nil && slices.Contains(p.Agents, id) {
				// An unknown source or unsupported target never means remove.
				body, err := os.ReadFile(promptPath(pid))
				if err == nil {
					err = checkPromptText(string(body))
				}
				if err != nil {
					continue
				}
				if t, ok := promptTargetByID(all, id); ok {
					state := l.promptState(t, p, string(body))
					if state.Status == "edited" || state.Status == "unverified" {
						continue
					}
					if state.Status != "conflict" {
						desired, _ = t.files(p, string(body))
					}
				} else {
					continue
				}
			}
			for path, hash := range old.Files {
				if slices.ContainsFunc(slices.Collect(maps.Keys(desired)), func(want string) bool { return promptSamePath(path, want) }) {
					continue
				}
				cur, err := os.ReadFile(path)
				if os.IsNotExist(err) {
					delete(old.Files, path)
					continue
				}
				if err != nil {
					res.fail(id, "prompt:"+pid, err)
					continue
				}
				if promptHash(cur) != hash {
					res.fail(id, "prompt:"+pid, fmt.Errorf("externally edited file kept: %s", path))
					delete(old.Files, path) // no longer managed: ordinary skill discovery may find it
					continue
				}
				if err := b.keep(id+"-"+pid, path); err != nil {
					res.fail(id, "prompt:"+pid, err)
					continue
				}
				if err := os.Remove(path); err != nil {
					res.fail(id, "prompt:"+pid, err)
					continue
				}
				delete(old.Files, path)
				res.changed(id)
			}
			if len(old.Files) == 0 {
				delete(a.Prompts, pid)
			}
		}
	}
}

// Link a flushed staging file into a new name without replacing a file created
// after the conflict check. Filesystems without hard links report a problem.
func writePromptFile(path string, data []byte, expected string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("user-owned link or non-file: %s", path)
		}
		cur, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if expected == "" || promptHash(cur) != expected {
			return fmt.Errorf("command changed since it was checked: %s", path)
		}
		return edit.WriteAtomic(path, data)
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".prompt-*.tmp")
	if err != nil {
		return err
	}
	stage := f.Name()
	defer os.Remove(stage)
	if err := f.Close(); err != nil {
		return err
	}
	if err := edit.WriteAtomic(stage, data); err != nil {
		return err
	}
	return os.Link(stage, path)
}
