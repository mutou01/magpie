package library

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type codexPromptNotice struct {
	Source    string
	SkillPath string // Empty for the ignored user configuration setting.
}

type codexPromptInventory struct {
	Commands []nativePromptCommand
	Notices  []codexPromptNotice
}

// Codex loads a configured catalog at startup, relative to its configuration
// directory. Missing or changed catalog bytes invalidate the same evidence.
func codexPromptCatalog(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var cfg struct {
		Catalog string `toml:"model_catalog_json"`
	}
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return "", err
	}
	if cfg.Catalog == "" || filepath.IsAbs(cfg.Catalog) {
		return cfg.Catalog, nil
	}
	return filepath.Join(dir, cfg.Catalog), nil
}

// Native startup reports ignored settings as one summary, with up to three
// details. Only this complete, single known retired key is independent of
// skill naming, loading and policy. Additional or truncated warnings block.
func codexIgnoredPromptSetting(summary, dir string) bool {
	const prefix = "Codex is ignoring 1 unrecognized configuration setting. Check for typos or deprecated settings.\n  user ("
	const suffix = "): `disable_response_storage` is ignored."
	if !strings.HasPrefix(summary, prefix) || !strings.HasSuffix(summary, suffix) {
		return false
	}
	path := strings.TrimSuffix(strings.TrimPrefix(summary, prefix), suffix)
	return !strings.ContainsAny(path, "\r\n") && filepath.IsAbs(path) && promptSamePath(path, filepath.Join(dir, "config.toml"))
}

// The native loader parses each skill's metadata independently. A malformed
// policy defaults that skill to implicit invocation; it is never evidence of
// an explicit-only policy. Associate the diagnostic with its complete native
// inventory entry, preserving both the occupied name and its warning.
func codexPromptMetadataNotice(message string, commands []nativePromptCommand) (codexPromptNotice, bool) {
	path, detail, found := strings.Cut(strings.TrimPrefix(message, "ignoring "), ": invalid openai.yaml: ")
	if !strings.HasPrefix(message, "ignoring ") || !found || detail == "" {
		return codexPromptNotice{}, false
	}
	u, err := url.Parse(path)
	if err != nil || u.Scheme != "file" || u.Host != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return codexPromptNotice{}, false
	}
	path = filepath.FromSlash(u.Path)
	// A Windows file URI has a slash before the drive letter.
	if len(path) > 2 && (path[0] == '/' || path[0] == '\\') && filepath.VolumeName(path[1:]) != "" {
		path = path[1:]
	}
	if !filepath.IsAbs(path) {
		return codexPromptNotice{}, false
	}
	real, err := promptSourcePath(path, nil, 0)
	if err != nil {
		return codexPromptNotice{}, false
	}
	for _, command := range commands {
		metadata := filepath.Join(filepath.Dir(command.Path), "agents", "openai.yaml")
		candidate, err := promptSourcePath(metadata, nil, 0)
		if err == nil && promptSamePath(candidate, real) {
			return codexPromptNotice{Source: metadata, SkillPath: command.Path}, true
		}
	}
	return codexPromptNotice{}, false
}

func codexPromptDiagnostics(stderr *boundedPiOutput, warnings []string, inventory *codexPromptInventory) string {
	stderr.Lock()
	defer stderr.Unlock()
	if stderr.tooLarge {
		return "Native skill sources could not be fully loaded"
	}
	for _, line := range strings.Split(stderr.b.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var diagnostic struct {
			Level  string `json:"level"`
			Target string `json:"target"`
			Fields struct {
				Message string `json:"message"`
			} `json:"fields"`
		}
		if json.Unmarshal([]byte(line), &diagnostic) != nil {
			return "Native skill sources could not be fully loaded"
		}
		if diagnostic.Level == "ERROR" && diagnostic.Target == "codex_app_server" && slices.Contains(warnings, diagnostic.Fields.Message) {
			continue
		}
		if diagnostic.Level == "WARN" && diagnostic.Target == "codex_skills_extension::loader::metadata" {
			if notice, ok := codexPromptMetadataNotice(diagnostic.Fields.Message, inventory.Commands); ok {
				inventory.Notices = append(inventory.Notices, notice)
				continue
			}
			return "Native skill metadata could not be loaded; invocation policy is unverified"
		}
		return "Native skill sources could not be fully loaded"
	}
	slices.SortFunc(inventory.Notices, func(a, b codexPromptNotice) int {
		return strings.Compare(a.Source+"\x00"+a.SkillPath, b.Source+"\x00"+b.SkillPath)
	})
	inventory.Notices = slices.Compact(inventory.Notices)
	return ""
}

func codexPromptWarning(t PromptTarget) (string, string) {
	nativePromptChecks.Lock()
	c := nativePromptChecks.m[nativePromptCacheKey(t)]
	nativePromptChecks.Unlock()
	if t.ID != "codex" || c.err != nil || len(c.notices) == 0 {
		return "", ""
	}
	var sources []string
	for _, notice := range c.notices {
		sources = append(sources, notice.Source)
	}
	return "Codex ignored unrelated configuration or skill metadata; this template is unaffected", strings.Join(sources, "; ")
}
