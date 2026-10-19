package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yetone/magpie/internal/proc"
)

type piCommandSnapshot struct {
	bin, key string
	dirs     []string
	dynamic  bool
}

// Snapshot only observable command sources, not sessions or telemetry. Stamps
// follow the skill cache's path/type/size/mtime contract; they are not a claim
// that extension code cannot consult other files, the network or the clock.
func (l *Library) piCommandSnapshot(t PromptTarget) (piCommandSnapshot, error) {
	s := piCommandSnapshot{bin: proc.FindTool("pi")}
	if s.bin == "" {
		return s, &promptRuntimeError{"Could not start Pi command verification", t.dir}
	}
	dirs := []string{home()}
	for _, p := range l.Projects {
		dirs = append(dirs, p.Dir)
	}
	for _, dir := range dirs {
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return s, err
		}
		if !slices.ContainsFunc(s.dirs, func(d string) bool { return promptSamePath(d, real) }) {
			s.dirs = append(s.dirs, real)
		}
		if err := piProjectTrusted(t.dir, real); err != nil {
			return s, err
		}
	}
	slices.Sort(s.dirs)
	roots := []string{s.bin, filepath.Join(filepath.Dir(s.bin), "pi-launcher.js"), filepath.Join(t.dir, "install", "current-version"), filepath.Join(t.dir, "settings.json"), filepath.Join(t.dir, "trust.json"), filepath.Join(t.dir, "models.json"), filepath.Join(t.dir, "auth.json"), sharedSkillsDir()}
	// A managed launcher keeps the selected release outside its bin directory.
	if b, err := os.ReadFile(filepath.Join(t.dir, "install", "current-version")); err == nil {
		v := strings.TrimSpace(string(b))
		if !filepath.IsLocal(v) || strings.ContainsAny(v, "/\\") {
			return s, fmt.Errorf("invalid Pi release selector")
		}
		roots = append(roots, filepath.Join(t.dir, "install", "releases", v, "node_modules", "@earendil-works", "pi-coding-agent", "package.json"))
	} else if !os.IsNotExist(err) {
		return s, err
	}
	configs := []string{t.dir}
	for _, dir := range s.dirs {
		configs = append(configs, filepath.Join(dir, ".pi"))
		// Pi discovers shared skills in cwd and its ancestors.
		for _, d := range piSkillAncestors(dir) {
			roots = append(roots, filepath.Join(d, ".agents", "skills"))
		}
	}
	for _, base := range configs {
		var cfg struct {
			Extensions          []string          `json:"extensions"`
			Packages            []json.RawMessage `json:"packages"`
			Prompts             []string          `json:"prompts"`
			Skills              []string          `json:"skills"`
			DefaultProjectTrust string            `json:"defaultProjectTrust"`
		}
		path := filepath.Join(base, "settings.json")
		b, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return s, err
		}
		if len(b) > 0 {
			if err := json.Unmarshal(b, &cfg); err != nil {
				return s, fmt.Errorf("%s: %w", path, err)
			}
		}
		roots = append(roots, path, filepath.Join(base, "extensions"), filepath.Join(base, "prompts"), filepath.Join(base, "skills"))
		es, err := os.ReadDir(filepath.Join(base, "extensions"))
		if err != nil && !os.IsNotExist(err) {
			return s, err
		}
		if len(cfg.Extensions) > 0 || len(cfg.Packages) > 0 || len(es) > 0 {
			s.dynamic = true
		}
		for _, list := range [][]string{cfg.Extensions, cfg.Prompts, cfg.Skills} {
			for _, entry := range list {
				if strings.HasPrefix(entry, "-") || strings.HasPrefix(entry, "!") {
					continue
				}
				entry = strings.TrimPrefix(entry, "+")
				if strings.HasPrefix(entry, "npm:") || strings.HasPrefix(entry, "git:") {
					return s, fmt.Errorf("resource source requires a package declaration: %s", path)
				}
				if strings.HasPrefix(entry, "~/") || strings.HasPrefix(entry, `~\`) {
					entry = filepath.Join(home(), entry[2:])
				}
				if !filepath.IsAbs(entry) {
					entry = filepath.Join(base, entry)
				}
				if i := strings.IndexAny(entry, "*?["); i >= 0 {
					prefix := entry[:i]
					if strings.HasSuffix(prefix, string(filepath.Separator)) {
						entry = filepath.Clean(prefix)
					} else {
						entry = filepath.Dir(prefix)
					}
				} else if _, err := os.Stat(entry); err != nil {
					return s, &promptRuntimeError{"Pi command sources could not be fully read", entry}
				}
				roots = append(roots, entry)
			}
		}
		if len(cfg.Packages) > 0 {
			roots = append(roots, filepath.Join(base, "npm"), filepath.Join(base, "git"))
			for _, raw := range cfg.Packages {
				var src string
				if json.Unmarshal(raw, &src) != nil {
					var o struct {
						Source string `json:"source"`
					}
					if err := json.Unmarshal(raw, &o); err != nil || o.Source == "" {
						return s, fmt.Errorf("invalid Pi package source: %s", path)
					}
					src = o.Source
				}
				if strings.HasPrefix(src, "npm:") || strings.HasPrefix(src, "git:") || strings.HasPrefix(src, "https:") || strings.HasPrefix(src, "git@") {
					// Offline Pi silently skips absent or mismatched packages. Until
					// their exact resolver and version ranges are verified, its empty
					// RPC list is insufficient proof of complete coverage.
					return s, &promptRuntimeError{"Pi package sources need verification", src}
				}
				if strings.HasPrefix(src, "~/") {
					src = filepath.Join(home(), src[2:])
				}
				if !filepath.IsAbs(src) {
					src = filepath.Join(base, src)
				}
				if _, err := os.Stat(src); err != nil {
					return s, &promptRuntimeError{"Pi command sources could not be fully read", src}
				}
				roots = append(roots, src)
			}
		}
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\n", t.dir, s.bin, strings.Join(s.dirs, "\x00"))
	// Environment values are hashed only; credentials never appear in the result.
	env := os.Environ()
	slices.Sort(env)
	fmt.Fprintln(h, strings.Join(env, "\x00"))
	seen := map[string]bool{}
	count := 0
	var walk func(string) error
	walk = func(path string) error {
		path = filepath.Clean(path)
		if seen[path] {
			return nil
		}
		seen[path] = true
		count++
		if count > 50000 {
			return fmt.Errorf("Pi command source scan exceeds 50000 entries")
		}
		for _, a := range l.Applied {
			for _, p := range a.Prompts {
				for owned := range p.Files {
					if promptSamePath(path, owned) {
						return nil
					}
				}
			}
		}
		fi, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			real, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "link:%s:%s\n", path, real)
			return walk(real)
		}
		if fi.IsDir() {
			es, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			for _, e := range es {
				if e.Name() == ".git" {
					continue
				}
				if err := walk(filepath.Join(path, e.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00%d\n", path, fi.Mode(), fi.Size(), fi.ModTime().UnixNano())
		return nil
	}
	slices.Sort(roots)
	for _, path := range roots {
		if err := walk(path); err != nil {
			return s, err
		}
	}
	s.key = hex.EncodeToString(h.Sum(nil))
	return s, nil
}

// RPC has no initial project-trust UI in Pi 0.87.1. A successful command list
// could otherwise omit the project's extensions without any error. Honor the
// existing nearest trust decision/default, never grant trust from magpie.
func piProjectTrusted(agentDir, cwd string) error {
	requires := false
	for _, entry := range []string{"settings.json", "extensions", "skills", "prompts", "themes", "SYSTEM.md", "APPEND_SYSTEM.md"} {
		_, err := os.Stat(filepath.Join(cwd, ".pi", entry))
		if err == nil {
			requires = true
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for _, d := range piSkillAncestors(cwd) {
		p := filepath.Join(d, ".agents", "skills")
		if !promptSamePath(p, sharedSkillsDir()) {
			_, err := os.Stat(p)
			if err == nil {
				requires = true
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}
	if !requires {
		return nil
	}
	var trust map[string]*bool
	b, err := os.ReadFile(filepath.Join(agentDir, "trust.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &trust); err != nil {
			return err
		}
	}
	allowed := false
	found := false
	for d := cwd; ; d = filepath.Dir(d) {
		for p, decision := range trust {
			if decision != nil && promptSamePath(p, d) {
				allowed, found = *decision, true
				break
			}
		}
		if found || filepath.Dir(d) == d {
			break
		}
	}
	if !found {
		var cfg struct {
			DefaultProjectTrust string `json:"defaultProjectTrust"`
		}
		b, err := os.ReadFile(filepath.Join(agentDir, "settings.json"))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if len(b) > 0 {
			if err := json.Unmarshal(b, &cfg); err != nil {
				return err
			}
		}
		allowed = cfg.DefaultProjectTrust == "always"
	}
	if !allowed {
		return &promptRuntimeError{"Pi project resources need existing trust", cwd}
	}
	return nil
}

func piSkillAncestors(cwd string) []string {
	var out []string
	stop := ""
	if proc.Isolated() {
		// Never inspect the developer's ancestor skills from a unit test.
		stop, _ = filepath.EvalSymlinks(home())
		if rel, err := filepath.Rel(stop, cwd); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			stop = cwd
		}
	}
	for d := cwd; ; d = filepath.Dir(d) {
		out = append(out, d)
		if filepath.Dir(d) == d || d == stop {
			return out
		}
	}
}
