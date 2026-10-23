package library

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/proc"
	"gopkg.in/yaml.v3"
)

// This is an independent client, not a plugin and never installed in DSH. The
// original CLI and its original remote descriptors provide all native calls.
//
//go:embed prompt_dsh_probe.mjs
var dshPromptProbe string

func dshPromptEntry(dir string) string {
	for _, base := range []string{filepath.Join(dir, "profiles", "node_modules", "@deepseek-ai", "dsh"), filepath.Join(dir, "node_modules", "@deepseek-ai", "dsh")} {
		b, err := os.ReadFile(filepath.Join(base, "package.json"))
		if err != nil {
			continue
		}
		var pkg struct {
			Name string          `json:"name"`
			Bin  json.RawMessage `json:"bin"`
		}
		if json.Unmarshal(b, &pkg) != nil || pkg.Name != "@deepseek-ai/dsh" {
			continue
		}
		var bin string
		if json.Unmarshal(pkg.Bin, &bin) != nil {
			var bins map[string]string
			if json.Unmarshal(pkg.Bin, &bins) != nil {
				continue
			}
			bin = bins["dsh"]
		}
		if !filepath.IsLocal(bin) {
			continue
		}
		entry := filepath.Join(base, bin)
		if fi, err := os.Stat(entry); err == nil && fi.Mode().IsRegular() {
			return entry
		}
	}
	return ""
}

// Refuse executable YAML and explicit storage/resource redirection before
// starting the copied profile. Unknown mappings must not write original data.
func dshPromptCopyProfile(dir, temp string) error {
	if _, err := os.Stat(filepath.Join(dir, "settings.yaml")); err == nil {
		return fmt.Errorf("legacy settings import requires an isolated resource mapping: %s", filepath.Join(dir, "settings.yaml"))
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, rel := range []string{"profiles/web/package.json", "profiles/web/cordis.patch.yml", "cordis.patch.yml"} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) && rel != "profiles/web/package.json" {
			continue
		}
		if err != nil {
			return err
		}
		if strings.HasSuffix(rel, ".yml") {
			var doc yaml.Node
			if err := yaml.Unmarshal(b, &doc); err != nil {
				return err
			}
			var check func(*yaml.Node) error
			check = func(n *yaml.Node) error {
				if n.Kind == yaml.AliasNode {
					return fmt.Errorf("executable or aliased profile mapping requires verification: %s", path)
				}
				switch n.Tag {
				case "", "!!map", "!!seq", "!!str", "!!null", "!!bool", "!!int", "!!float":
				default:
					return fmt.Errorf("dynamic profile mapping requires verification: %s", path)
				}
				if n.Kind == yaml.ScalarNode && (filepath.IsAbs(n.Value) || strings.HasPrefix(n.Value, "file:") || strings.HasPrefix(n.Value, "~/") || strings.HasPrefix(n.Value, `~\`)) {
					return fmt.Errorf("absolute profile resource requires isolated mapping: %s", path)
				}
				if n.Kind == yaml.MappingNode {
					for i := 0; i+1 < len(n.Content); i += 2 {
						key := strings.ToLower(n.Content[i].Value)
						if strings.Contains(key, "path") || strings.Contains(key, "dir") || strings.Contains(key, "home") || strings.Contains(key, "file") || strings.Contains(key, "storage") || strings.Contains(key, "database") {
							return fmt.Errorf("profile resource mapping cannot be isolated: %s (%s)", path, n.Content[i].Value)
						}
					}
				}
				for _, child := range n.Content {
					if err := check(child); err != nil {
						return err
					}
				}
				return nil
			}
			if err := check(&doc); err != nil {
				return err
			}
		}
		to := filepath.Join(temp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(to), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(to, b, 0600); err != nil {
			return err
		}
	}
	return nil
}

func readDSHPromptCommands(ctx context.Context, t PromptTarget, s nativePromptSnapshot) ([]nativePromptCommand, []string, error) {
	bad := func(reason, detail string) ([]nativePromptCommand, []string, error) {
		return nil, nil, &promptRuntimeError{reason, detail}
	}
	node := proc.FindTool("node")
	if node == "" {
		return bad("Could not find the native command-check entry point", "Node.js for DeepSeek Harness")
	}
	temp, err := os.MkdirTemp("", "magpie-dsh-prompt-check-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(temp)
	if err := dshPromptCopyProfile(t.dir, temp); err != nil {
		return bad("DeepSeek Harness configuration cannot be safely mapped", err.Error())
	}
	// Preserve real skill roots while keeping settings and state temporary.
	patch, _ := yaml.Marshal([]map[string]any{{"id": "skill-filesystem", "config": map[string]any{"dshHome": t.dir, "agentsHome": filepath.Dir(sharedSkillsDir())}}})
	pf, err := os.OpenFile(filepath.Join(temp, "profiles/web/cordis.patch.yml"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, err
	}
	_, err = pf.Write(append([]byte("\n"), patch...))
	closeErr := pf.Close()
	if err != nil {
		return nil, nil, err
	}
	if closeErr != nil {
		return nil, nil, closeErr
	}
	// Link executable dependencies only. Configuration, credentials, sessions
	// and storage are physical temporary files, never links to the user's home.
	deps := filepath.Join(t.dir, "profiles", "node_modules")
	if _, err := os.Stat(deps); err != nil {
		return nil, nil, err
	}
	if err := os.Symlink(deps, filepath.Join(temp, "profiles", "node_modules")); err != nil {
		return bad("DeepSeek Harness configuration cannot be safely mapped", "Could not link native dependencies")
	}
	helper := filepath.Join(temp, "probe.mjs")
	if err := os.WriteFile(helper, []byte(dshPromptProbe), 0600); err != nil {
		return nil, nil, err
	}
	request, _ := json.Marshal(map[string]any{"entry": s.bin, "home": temp, "sourceHome": t.dir, "agentsHome": filepath.Dir(sharedSkillsDir()), "cwds": s.dirs})
	cmd := proc.Command(node, helper)
	cmd.Dir = temp
	cmd.Stdin = strings.NewReader(string(request))
	cmd.WaitDelay = time.Second
	out, stderr := &boundedPiOutput{}, &boundedPiOutput{}
	cmd.Stdout, cmd.Stderr = out, stderr
	tree, err := proc.StartTree(cmd)
	if err != nil {
		return bad("Could not start native skill verification", "DeepSeek Harness")
	}
	done := make(chan error, 1)
	go func() { done <- tree.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		tree.Kill()
		<-done
		stage := "DeepSeek Harness"
		for _, line := range strings.Split(out.b.String(), "\n") {
			var p struct {
				Stage string `json:"stage"`
			}
			if json.Unmarshal([]byte(line), &p) == nil && p.Stage != "" {
				stage = p.Stage
			}
		}
		return bad("Native command verification timed out or was cancelled", stage)
	}
	if err != nil || out.tooLarge || stderr.failed() {
		return bad("DeepSeek Harness native inventory could not be read", "Native startup, authorization or protocol failed")
	}
	var result struct {
		Commands []nativePromptCommand `json:"commands"`
		Complete bool                  `json:"complete"`
		Reason   string                `json:"reason"`
		Sources  []string              `json:"sources"`
	}
	lines := strings.Split(strings.TrimSpace(out.b.String()), "\n")
	if json.Unmarshal([]byte(lines[len(lines)-1]), &result) != nil {
		return bad("Native skill verification returned invalid output", "DeepSeek Harness")
	}
	// There is deliberately no inference from an empty list, active plugins or
	// a clean log. The current native endpoint omits provider completeness.
	if !result.Complete {
		return bad("DeepSeek Harness skill source completeness could not be verified", result.Reason)
	}
	return result.Commands, result.Sources, nil
}
