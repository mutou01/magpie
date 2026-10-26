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
	enabled := l.promptsEnabled()
	for _, t := range all {
		r := &promptReader{Library: l}
		type publication struct {
			prompt *Prompt
			files  map[string][]byte
		}
		var publish []publication
		for _, p := range l.Prompts {
			if !enabled || !slices.Contains(p.Agents, t.ID) || !t.Detected {
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
			if state := r.promptState(t, p, string(body)); !promptUsable(state) {
				res.fail(t.ID, "prompt:"+p.Name, fmt.Errorf("%s: %s", state.Reason, state.Source))
				continue
			}
			publish = append(publish, publication{p, files})
		}
		// Finish shared source reads before publishing. Individual destination
		// files are still read and checked against ownership just before writing.
		for _, item := range publish {
			p, files := item.prompt, item.files
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
				caseRename := false
				if runtime.GOOS == "windows" {
					for alias := range old.Files {
						caseRename = caseRename || (alias != path && strings.EqualFold(alias, path) && filepath.Base(alias) != filepath.Base(path) && promptSamePath(alias, path))
					}
				}
				if err == nil && ownedHash != "" && promptHash(cur) == promptHash(data) && !caseRename {
					old.Files[path] = promptHash(data)
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
	// Publishing can change discovery roots. Retirement gets fresh evidence,
	// collected before any removal, never the pre-publication reader.
	r := &promptReader{Library: l}
	retirement := map[[2]string]PromptState{}
	for id, a := range l.Applied {
		for pid := range a.Prompts {
			p := l.prompt(pid)
			if !enabled || p == nil || !slices.Contains(p.Agents, id) {
				continue
			}
			t, ok := promptTargetByID(all, id)
			body, err := os.ReadFile(promptPath(pid))
			if !ok || err != nil || checkPromptText(string(body)) != nil {
				continue
			}
			retirement[[2]string{id, pid}] = r.promptState(t, p, string(body))
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
			if p := l.prompt(pid); enabled && p != nil && (id == "codex" || id == "codex-desktop") && (slices.Contains(p.Agents, "codex") || slices.Contains(p.Agents, "codex-desktop")) {
				// Without the body we cannot prove which shared files are still
				// needed. Keep the existing ownership, not just the file itself.
				if _, err := os.ReadFile(promptPath(pid)); err != nil {
					continue
				}
			}
			if p := l.prompt(pid); enabled && p != nil && slices.Contains(p.Agents, id) {
				// An unknown source or unsupported target never means remove.
				body, err := os.ReadFile(promptPath(pid))
				if err == nil {
					err = checkPromptText(string(body))
				}
				if err != nil {
					continue
				}
				if t, ok := promptTargetByID(all, id); ok {
					state, checked := retirement[[2]string{id, pid}]
					if !checked {
						continue
					}
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
			if id == "codex" || id == "codex-desktop" {
				// Body and invocation policy form one deployment, including
				// ownership split across legacy CLI/Desktop records. Never
				// retire half of an externally edited skill.
				if edited, err := l.promptCodexRetirementCheck(pid); err != nil {
					res.fail(id, "prompt:"+pid, err)
					if edited && l.prompt(pid) == nil {
						// Deletion releases the retained skill to the user. Drop
						// both legacy records together so neither half is retired.
						for _, target := range []string{"codex", "codex-desktop"} {
							if applied := l.Applied[target]; applied != nil {
								delete(applied.Prompts, pid)
							}
						}
					}
					continue
				}
			}
			for path, hash := range old.Files {
				if slices.ContainsFunc(slices.Collect(maps.Keys(desired)), func(want string) bool { return promptSamePath(path, want) }) {
					continue
				}
				if enabled && l.promptSharedPathNeeded(all, id, pid, path, hash, retirement) {
					delete(old.Files, path)
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

func (l *Library) promptCodexRetirementCheck(pid string) (bool, error) {
	for _, id := range []string{"codex", "codex-desktop"} {
		a := l.Applied[id]
		if a == nil || a.Prompts[pid] == nil {
			continue
		}
		for path, hash := range a.Prompts[pid].Files {
			cur, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return false, err
			}
			if promptHash(cur) != hash {
				return true, fmt.Errorf("externally edited template kept with its invocation policy: %s", path)
			}
		}
	}
	return false, nil
}

// A shared deployment is retired only after its last selected consumer stops
// needing that physical file. This also understands legacy Desktop records.
func (l *Library) promptSharedPathNeeded(all []PromptTarget, id, pid, path, hash string, retirement map[[2]string]PromptState) bool {
	if id != "codex" && id != "codex-desktop" {
		return false
	}
	p := l.prompt(pid)
	if p == nil {
		return false
	}
	body, err := os.ReadFile(promptPath(pid))
	if err != nil {
		return true
	}
	for _, other := range []string{"codex", "codex-desktop"} {
		if other == id || !slices.Contains(p.Agents, other) {
			continue
		}
		t, ok := promptTargetByID(all, other)
		if !ok {
			continue
		}
		files, _ := t.files(p, string(body))
		for want := range files {
			if promptSamePath(path, want) {
				// Transfer legacy ownership even if the remaining consumer's
				// capability check is pending. This records the existing file,
				// not a successful new deployment, and preserves edit protection.
				a := l.applied(other)
				if a.Prompts == nil {
					a.Prompts = map[string]*AppliedPrompt{}
				}
				if a.Prompts[pid] == nil {
					a.Prompts[pid] = &AppliedPrompt{Files: map[string]string{}}
				}
				if a.Prompts[pid].Files == nil {
					a.Prompts[pid].Files = map[string]string{}
				}
				if a.Prompts[pid].Files[want] == "" {
					a.Prompts[pid].Files[want] = hash
				}
				return true
			}
		}
		if a := l.Applied[other]; a != nil && a.Prompts[pid] != nil {
			state, checked := retirement[[2]string{other, pid}]
			if !checked || state.Status == "unverified" || state.Status == "edited" {
				for owned := range a.Prompts[pid].Files {
					if promptSamePath(path, owned) {
						return true
					}
				}
			}
		}
	}
	return false
}

// Publish a flushed staging file without replacing a file created after the
// conflict check. Existing owned files still use the normal atomic editor.
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
	return publishPromptFile(stage, path)
}
