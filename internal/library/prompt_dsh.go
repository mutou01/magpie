package library

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/proc"
	"gopkg.in/yaml.v3"
)

// This is an independent client, not a plugin and never installed in DSH. The
// original CLI and registries provide all native calls.
//
//go:embed prompt_dsh_probe.mjs
var dshPromptProbe string

var dshPromptPackageName = regexp.MustCompile(`(?i)^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)
var dshPromptVariableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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
	for _, rel := range []string{"profiles/web/package.json", "profiles/web/compatibility.json", "profiles/web/cordis.patch.yml", "cordis.patch.yml", ".env"} {
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
			var check func(*yaml.Node, bool) error
			check = func(n *yaml.Node, resource bool) error {
				if n.Kind == yaml.AliasNode {
					return fmt.Errorf("executable or aliased profile mapping requires verification: %s", path)
				}
				switch n.Tag {
				case "", "!!map", "!!seq", "!!str", "!!null", "!!bool", "!!int", "!!float":
				default:
					return fmt.Errorf("dynamic profile mapping requires verification: %s", path)
				}
				if !resource && n.Kind == yaml.ScalarNode && (filepath.IsAbs(n.Value) || strings.HasPrefix(n.Value, "file:") || strings.HasPrefix(n.Value, "~/") || strings.HasPrefix(n.Value, `~\`)) {
					return fmt.Errorf("absolute profile resource requires isolated mapping: %s", path)
				}
				if n.Kind == yaml.MappingNode {
					filesystem := false
					for i := 0; i+1 < len(n.Content); i += 2 {
						if n.Content[i].Value == "name" && n.Content[i+1].Value == "@deepseek-ai/dsh-skill-filesystem" {
							filesystem = true
						}
					}
					for i := 0; i+1 < len(n.Content); i += 2 {
						key := strings.ToLower(n.Content[i].Value)
						readRoot := resource && (key == "dshhome" || key == "agentshome" || key == "customskilldirs" || key == "bundledskilldir")
						if !readRoot && (strings.Contains(key, "path") || strings.Contains(key, "dir") || strings.Contains(key, "home") || strings.Contains(key, "file") || strings.Contains(key, "storage") || strings.Contains(key, "database")) {
							return fmt.Errorf("profile resource mapping cannot be isolated: %s (%s)", path, n.Content[i].Value)
						}
						if err := check(n.Content[i+1], readRoot || filesystem && key == "config"); err != nil {
							return err
						}
					}
					return nil
				}
				for _, child := range n.Content {
					if err := check(child, resource); err != nil {
						return err
					}
				}
				return nil
			}
			if err := check(&doc, false); err != nil {
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
	// Each invoking directory needs its own native launch environment. Reusing
	// a process would promote a previous project's dotenv values to inherited
	// process values in the next context, changing both precedence and trust.
	var commands []nativePromptCommand
	var sources []string
	for _, cwd := range s.dirs {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		found, roots, err := readDSHPromptContext(ctx, t, s, cwd)
		if err != nil {
			return nil, nil, err
		}
		commands = append(commands, found...)
		sources = append(sources, roots...)
	}
	slices.Sort(sources)
	return commands, slices.Compact(sources), nil
}

func readDSHPromptContext(ctx context.Context, t PromptTarget, s nativePromptSnapshot, cwd string) ([]nativePromptCommand, []string, error) {
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
	// The shared installation stays linked. Profile-local packages are copied
	// by the client: a directory link would move their real importers outside
	// the native profile resolver's scope and lose installation peer fallback.
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
	envCwd := cwd
	if promptSamePath(cwd, t.dir) {
		envCwd = temp
	}
	request, _ := json.Marshal(map[string]any{"entry": s.bin, "home": temp, "sourceHome": t.dir, "agentsHome": filepath.Dir(sharedSkillsDir()), "cwds": []string{cwd}, "envCwd": envCwd})
	cmd := proc.Command(node, helper)
	cmd.Dir = temp
	cmd.Stdin = strings.NewReader(string(request))
	cmd.WaitDelay = time.Second
	// Native dependency resolution includes absent shadowing candidates as
	// well as physical sources. Keep a bounded larger inventory envelope.
	out, stderr := &boundedPiOutput{limit: 8 << 20}, &boundedPiOutput{}
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
	if out.tooLarge || stderr.tooLarge {
		return bad("DeepSeek Harness native inventory could not be read", "Native startup, authorization or protocol failed")
	}
	var result struct {
		Commands   []nativePromptCommand `json:"commands"`
		Complete   bool                  `json:"complete"`
		Reason     string                `json:"reason"`
		Sources    []string              `json:"sources"`
		Diagnostic struct {
			Kind       string `json:"kind"`
			Plugin     string `json:"plugin"`
			Dependency string `json:"dependency"`
			Variable   string `json:"variable"`
			Layer      string `json:"layer"`
		} `json:"diagnostic"`
	}
	lines := strings.Split(strings.TrimSpace(out.b.String()), "\n")
	if json.Unmarshal([]byte(lines[len(lines)-1]), &result) != nil {
		return bad("Native skill verification returned invalid output", "DeepSeek Harness")
	}
	// The independent native client must prove provider/source completeness;
	// an empty list, active plugins or a clean log cannot replace that proof.
	if !result.Complete {
		d := result.Diagnostic
		if d.Kind == "dependency" && len(d.Plugin) <= 214 && len(d.Dependency) <= 214 && dshPromptPackageName.MatchString(d.Plugin) && dshPromptPackageName.MatchString(d.Dependency) {
			return bad("DeepSeek Harness plugin dependency could not be loaded", d.Plugin+": "+d.Dependency)
		}
		if d.Kind == "environment" && len(d.Variable) <= 256 && dshPromptVariableName.MatchString(d.Variable) && (d.Layer == "user" || d.Layer == "project") {
			root := cwd
			if d.Layer == "user" {
				root = t.dir
			}
			return bad("DeepSeek Harness environment variable is restricted to the launching process", filepath.Join(root, ".env")+": "+d.Variable)
		}
		return bad("DeepSeek Harness skill source completeness could not be verified", result.Reason)
	}
	if err != nil || stderr.failed() {
		return bad("DeepSeek Harness native inventory could not be read", "Native startup, authorization or protocol failed")
	}
	return result.Commands, result.Sources, nil
}
