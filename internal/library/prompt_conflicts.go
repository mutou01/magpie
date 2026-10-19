package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

func (l *Library) promptOwns(id, pid, path string) bool {
	return l.promptOwnedHash(id, pid, path) != ""
}

func promptSamePath(a, b string) bool {
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) {
		x, ex := os.Stat(a)
		y, ey := os.Stat(b)
		if ex == nil && ey == nil {
			return os.SameFile(x, y)
		}
	}
	return samePath(a, b)
}

func (l *Library) promptOwnedHash(id, pid, path string) string {
	a := l.Applied[id]
	if a == nil || a.Prompts[pid] == nil {
		return ""
	}
	for p, hash := range a.Prompts[pid].Files {
		if promptSamePath(p, path) {
			return hash
		}
	}
	return ""
}

func (l *Library) promptState(t PromptTarget, p *Prompt, text string) PromptState {
	s := PromptState{Agent: t.ID, Status: "available"}
	if !t.Detected {
		s.Status, s.Reason = "unsupported", "Not installed here"
		return s
	}
	files, err := t.files(p, text)
	if err != nil {
		s.Status, s.Reason = "unsupported", err.Error()
		return s
	}
	allPresent := true
	if t.ID == "codex-desktop" {
		path, err := promptCodexDisabled(t, files)
		if err != nil {
			s.Status, s.Reason, s.Source = "unverified", "Could not check command sources", err.Error()
			return s
		}
		if path != "" {
			s.Status, s.Reason, s.Source = "unsupported", "This skill is disabled in Codex configuration", path
			return s
		}
	}
	for _, name := range strings.Fields(promptBuiltins[t.ID]) {
		if promptKey(name) == promptKey(p.Name) {
			s.Status, s.Reason, s.Source = "conflict", "This name is already a built-in command", t.Name
			return s
		}
	}
	for path := range files {
		if s.Path == "" || filepath.Base(path) == "SKILL.md" {
			s.Path = path
		}
		cur, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			allPresent = false
			continue
		}
		if err != nil {
			s.Status, s.Reason, s.Source = "unverified", "Could not check command sources", path
			return s
		}
		if !l.promptOwns(t.ID, p.ID, path) {
			s.Status, s.Reason, s.Source = "conflict", "This name is already in use", path
			return s
		}
		if promptHash(cur) != l.promptOwnedHash(t.ID, p.ID, path) {
			s.Status, s.Reason, s.Source = "edited", "An externally edited template was kept", path
			return s
		}
	}
	if source, err := l.promptConflict(t, p); err != nil {
		s.Status, s.Reason, s.Source = "unverified", "Could not check command sources", err.Error()
		var runtimeErr *promptRuntimeError
		if errors.As(err, &runtimeErr) {
			s.Reason, s.Source = runtimeErr.reason, runtimeErr.detail
		}
		return s
	} else if source != "" {
		s.Status, s.Reason, s.Source = "conflict", "This name is already in use", source
		return s
	}
	if a := l.Applied[t.ID]; allPresent && a != nil && a.Prompts[p.ID] != nil && len(a.Prompts[p.ID].Files) == len(files) {
		all := true
		for path, b := range files {
			if a.Prompts[p.ID].Files[path] != promptHash(b) {
				all = false
				break
			}
		}
		if all {
			s.Status = "enabled"
		}
	}
	return s
}

type promptSource struct {
	path    string
	skills  bool
	plugins bool
}

func (l *Library) isPromptSkill(dir string) bool {
	for _, a := range l.Applied {
		for _, p := range a.Prompts {
			for path := range p.Files {
				if filepath.Base(path) == "SKILL.md" && promptSamePath(path, filepath.Join(dir, "SKILL.md")) {
					return true
				}
			}
		}
	}
	return false
}

func (l *Library) promptConflict(t PromptTarget, p *Prompt) (string, error) {
	sources := []promptSource{{path: t.folder()}, {path: filepath.Join(t.dir, "skills"), skills: true}, {path: sharedSkillsDir(), skills: true}}
	if t.ID == "codex-desktop" {
		sources[0].skills = true
		sources = append(sources, promptSource{path: filepath.Join(t.dir, "skills", ".system"), skills: true})
	}
	if t.ID == "pi" {
		if hit, err := l.piCommandConflict(t, p); err != nil || hit != "" {
			return hit, err
		}
	}
	extra, err := promptConfiguredSources(t)
	if err != nil {
		return "", err
	}
	sources = append(sources, extra...)
	sources = append(sources, promptSource{path: filepath.Join(t.dir, "plugins"), plugins: true})
	projectFolder := map[string]string{"claude": ".claude", "pi": ".pi", "codex-desktop": ".codex"}[t.ID]
	for _, project := range l.Projects {
		dir := filepath.Join(project.Dir, projectFolder)
		commands := "commands"
		if t.ID == "pi" {
			commands = "prompts"
		}
		sources = append(sources, promptSource{path: filepath.Join(dir, commands)}, promptSource{path: filepath.Join(dir, "skills"), skills: true}, promptSource{path: filepath.Join(project.Dir, ".agents", "skills"), skills: true})
	}
	seen := map[string]bool{}
	for _, source := range sources {
		if seen[source.path] {
			continue
		}
		seen[source.path] = true
		hit, err := l.scanPromptSource(t.ID, p, source)
		if err != nil || hit != "" {
			return hit, err
		}
	}
	return "", nil
}

// scanPromptSource scans only command and skill declarations, never executes a
// harness, plugin or hook. Unreadable or malformed declarations remain unknown.
func (l *Library) scanPromptSource(id string, p *Prompt, source promptSource) (string, error) {
	var hit string
	count := 0
	err := filepath.WalkDir(source.path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == source.path && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		count++
		if count > 20000 {
			return fmt.Errorf("command source too large: %s", source.path)
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return fs.SkipDir
			}
			if id == "pi" && !source.skills && !source.plugins && path != source.path {
				return fs.SkipDir
			}
			return nil
		}
		isSkill := entry.Name() == "SKILL.md"
		// Follow top-level skill symlinks, as the harnesses do.
		if entry.Type()&os.ModeSymlink != 0 && (source.skills || source.plugins) {
			if info, err := os.Stat(path); err != nil {
				return err
			} else if info.IsDir() {
				path = filepath.Join(path, "SKILL.md")
				isSkill = true
			}
		}
		if source.skills && !isSkill {
			return nil
		}
		if source.plugins && !isSkill && !strings.Contains(filepath.ToSlash(path), "/commands/") {
			return nil
		}
		if !isSkill && filepath.Ext(path) != ".md" {
			return nil
		}
		if l.promptOwns(id, p.ID, path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		var alias string
		if isSkill {
			var m meta
			s := strings.ReplaceAll(string(data), "\r\n", "\n")
			front, ok := strings.CutPrefix(s, "---\n")
			if !ok {
				return fmt.Errorf("invalid skill metadata: %s", path)
			}
			front, _, ok = strings.Cut(front, "\n---")
			if !ok {
				return fmt.Errorf("invalid skill metadata: %s", path)
			}
			if err := yaml.Unmarshal([]byte(front), &m); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			name = m.Name
			if name == "" {
				name = filepath.Base(filepath.Dir(path))
			}
			if id == "pi" {
				name = "skill:" + name
			} else {
				alias = filepath.Base(filepath.Dir(path))
			}
		} else {
			rel, err := filepath.Rel(source.path, path)
			if err != nil {
				return err
			}
			name = strings.ReplaceAll(filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel))), "/", ":")
			if id == "pi" {
				name = strings.TrimSuffix(filepath.Base(path), ".md")
			}
		}
		if promptKey(name) == promptKey(p.Name) || alias != "" && promptKey(alias) == promptKey(p.Name) {
			hit = path
			return fs.SkipAll
		}
		return nil
	})
	return hit, err
}

func promptConfiguredSources(t PromptTarget) ([]promptSource, error) {
	var out []promptSource
	readJSON := func(path string, into any) error {
		b, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) || err == nil && len(b) == 0 {
			return nil
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, into); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	}
	if t.ID == "pi" {
		path := filepath.Join(t.dir, "settings.json")
		var cfg struct {
			Extensions []json.RawMessage `json:"extensions"`
			Packages   []json.RawMessage `json:"packages"`
			Prompts    []string          `json:"prompts"`
			Skills     []string          `json:"skills"`
		}
		if err := readJSON(path, &cfg); err != nil {
			return nil, err
		}
		for _, p := range cfg.Prompts {
			if strings.ContainsAny(p, "*?[]") || strings.HasPrefix(p, "!") {
				return nil, fmt.Errorf("resource pattern needs verification: %s", path)
			}
			if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
				p = filepath.Join(home(), p[2:])
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(t.dir, p)
			}
			out = append(out, promptSource{path: p})
		}
		for _, p := range cfg.Skills {
			if strings.ContainsAny(p, "*?[]") || strings.HasPrefix(p, "!") {
				return nil, fmt.Errorf("resource pattern needs verification: %s", path)
			}
			if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
				p = filepath.Join(home(), p[2:])
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(t.dir, p)
			}
			out = append(out, promptSource{path: p, skills: true})
		}
	}
	for _, file := range []string{"installed_plugins.json", "installed-plugins.json"} {
		path := filepath.Join(t.dir, "plugins", file)
		var registry any
		if err := readJSON(path, &registry); err != nil {
			return nil, err
		}
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				for key, item := range x {
					if key == "installPath" || key == "install_path" {
						if p, ok := item.(string); ok && filepath.IsAbs(p) {
							out = append(out, promptSource{path: p, plugins: true})
						}
					} else {
						walk(item)
					}
				}
			case []any:
				for _, item := range x {
					walk(item)
				}
			}
		}
		walk(registry)
	}
	return out, nil
}

func promptCodexDisabled(t PromptTarget, files map[string][]byte) (string, error) {
	path := filepath.Join(t.dir, "config.toml")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var cfg struct {
		Skills struct {
			Config []struct {
				Path    string `toml:"path"`
				Enabled *bool  `toml:"enabled"`
			} `toml:"config"`
		} `toml:"skills"`
	}
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	for _, c := range cfg.Skills.Config {
		for f := range files {
			if c.Enabled != nil && !*c.Enabled && samePath(c.Path, f) {
				return path, nil
			}
		}
	}
	return "", nil
}

func promptUsable(s PromptState) bool {
	return slices.Contains([]string{"available", "enabled"}, s.Status)
}
