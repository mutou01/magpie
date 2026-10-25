package library

// Native inventories are process-local evidence. Neither page reads nor save /
// sync may start a harness. A successful CLI probe does not certify Desktop UX.
import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/proc"
)

type nativePromptCommand struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Enabled *bool  `json:"enabled,omitempty"`
	managed bool
}

type nativePromptCheck struct {
	key         string
	roots       []string
	commands    []nativePromptCommand
	notices     []codexPromptNotice
	dshWarnings []string
	err         error
	checkedAt   int64
}

var nativePromptChecks = struct {
	sync.Mutex
	m    map[string]nativePromptCheck
	seq  map[string]uint64
	next uint64
}{m: map[string]nativePromptCheck{}, seq: map[string]uint64{}}

type nativePromptSnapshot struct {
	bin, key    string
	dirs, roots []string
}

func nativePromptCacheKey(t PromptTarget) string { return t.ID + "\x00" + t.dir }

func (l *Library) nativePromptSnapshot(t PromptTarget, extra []string) (nativePromptSnapshot, error) {
	return l.nativePromptSnapshotMode(t, extra, t.ID == "codex")
}

func (l *Library) nativePromptSnapshotMode(t PromptTarget, extra []string, light bool) (nativePromptSnapshot, error) {
	s := nativePromptSnapshot{bin: proc.FindTool(t.ID)}
	if t.ID == "dsh" {
		s.bin = dshPromptEntry(t.dir)
	}
	if s.bin == "" {
		return s, &promptRuntimeError{"Could not find the native command-check entry point", t.Name}
	}
	s.roots = append([]string{s.bin, filepath.Join(t.dir, "config.toml"), filepath.Join(t.dir, "skills"), filepath.Join(t.dir, "plugins"), sharedSkillsDir()}, extra...)
	if t.ID == "codex" {
		catalog, err := codexPromptCatalog(t.dir)
		if err != nil {
			return s, err
		}
		if catalog != "" {
			s.roots = append(s.roots, catalog)
		}
	}
	// npm shims and their native package are separate resources. Hash their
	// actual installed contents; a hash is never used as a compatibility gate.
	for _, dir := range []string{filepath.Dir(s.bin), filepath.Dir(filepath.Dir(s.bin))} {
		if t.ID == "codex" {
			s.roots = append(s.roots, filepath.Join(dir, "node_modules", "@openai", "codex"), filepath.Join(dir, "node_modules", "@openai", "codex-win32-x64"))
		}
	}
	if t.ID == "dsh" {
		s.roots = append(s.roots, filepath.Join(t.dir, "profiles/web/package.json"), filepath.Join(t.dir, "profiles/web/compatibility.json"), filepath.Join(t.dir, "profiles/web/cordis.patch.yml"), filepath.Join(t.dir, "cordis.patch.yml"), filepath.Join(t.dir, "settings.yaml"), filepath.Join(t.dir, ".env"), filepath.Dir(s.bin))
	}
	dirs := []string{home()}
	for _, p := range l.Projects {
		dirs = append(dirs, p.Dir)
	}
	for _, dir := range dirs {
		real, err := promptSourcePath(dir, nil, 0)
		if err != nil {
			return s, err
		}
		if !slices.ContainsFunc(s.dirs, func(d string) bool { return promptSamePath(d, real) }) {
			s.dirs = append(s.dirs, real)
		}
		for _, ancestor := range piSkillAncestors(real) {
			s.roots = append(s.roots, filepath.Join(ancestor, ".agents", "skills"), filepath.Join(ancestor, ".codex", "config.toml"))
		}
		if t.ID == "dsh" {
			s.roots = append(s.roots, filepath.Join(real, ".dsh", "skills"), filepath.Join(real, ".dsh", "cordis.patch.yml"), filepath.Join(real, ".env"))
		}
	}
	slices.Sort(s.dirs)
	key, err := l.promptResourceKeyMode(t, s.bin, s.dirs, s.roots, light)
	s.key = key
	return s, err
}

// Resolve every component freshly. Go's EvalSymlinks does not follow Windows
// mount-point junctions, and the Library's page memo cannot certify new evidence.
func promptSourcePath(path string, link func(string, string), depth int) (string, error) {
	if depth > 40 {
		return "", fmt.Errorf("command source link cycle: %s", path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	vol := filepath.VolumeName(path)
	cur := vol + string(filepath.Separator)
	parts := strings.FieldsFunc(path[len(vol):], func(r rune) bool { return r < 128 && os.IsPathSeparator(uint8(r)) })
	for i, part := range parts {
		next := filepath.Join(cur, part)
		fi, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		if linkEntry(fi) {
			to, err := os.Readlink(next)
			if err != nil {
				return "", err
			} // Unknown reparse points remain blocked.
			if !filepath.IsAbs(to) {
				to = filepath.Join(filepath.Dir(next), to)
			}
			to = filepath.Clean(to)
			if link != nil {
				link(next, to)
			}
			real, err := promptSourcePath(to, link, depth+1)
			if err != nil {
				// A missing optional resource differs from a broken installed
				// link. Do not let callers omit the latter as os.IsNotExist.
				return "", fmt.Errorf("unreadable command source link %s: %w", next, err)
			}
			return promptSourcePath(filepath.Join(append([]string{real}, parts[i+1:]...)...), link, depth+1)
		}
		cur = next
	}
	return cur, nil
}

// Hash bytes, not merely mtime: replacing a same-sized source while preserving
// timestamps must invalidate evidence too. Follow links and bound the traversal.
func (l *Library) promptResourceKey(t PromptTarget, bin string, dirs, roots []string) (string, error) {
	return l.promptResourceKeyMode(t, bin, dirs, roots, false)
}

// Light evidence watches declarations and source identities, not bundled binary
// assets. Deep byte fingerprints remain the active DSH and Pi checks' contract.
func (l *Library) promptResourceKeyMode(t PromptTarget, bin string, dirs, roots []string, light bool) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\n", t.dir, bin, strings.Join(dirs, "\x00"))
	env := os.Environ()
	slices.Sort(env)
	fmt.Fprintln(h, strings.Join(env, "\x00"))
	seen := map[string]bool{}
	active := map[string]bool{}
	rootDirs := map[string]bool{}
	recordLink := func(path, target string) { fmt.Fprintf(h, "link:%s:%s\n", path, target) }
	total := int64(0)
	type ownedFile struct{ path, hash string }
	ownedByName := map[string][]ownedFile{}
	for _, applied := range l.Applied {
		for _, prompt := range applied.Prompts {
			if prompt != nil {
				for path, hash := range prompt.Files {
					name := strings.ToLower(filepath.Base(path))
					ownedByName[name] = append(ownedByName[name], ownedFile{path, hash})
				}
			}
		}
	}
	type sourceFile struct {
		path string
		mode os.FileMode
		size int64
		sum  []byte
		err  error
	}
	var files []sourceFile
	var walk func(string, os.FileInfo) error
	walk = func(path string, fi os.FileInfo) error {
		path = filepath.Clean(path)
		if active[path] {
			return fmt.Errorf("command source directory cycle: %s", path)
		}
		if seen[path] {
			return nil
		}
		seen[path] = true
		active[path] = true
		defer delete(active, path)
		if len(seen) > 50000 {
			return fmt.Errorf("command source scan exceeds 50000 entries")
		}
		for _, owned := range ownedByName[strings.ToLower(filepath.Base(path))] {
			if promptSamePath(path, owned.path) {
				b, err := os.ReadFile(path)
				if os.IsNotExist(err) {
					return nil
				}
				if err != nil {
					return err
				}
				if promptHash(b) == owned.hash {
					return nil
				}
			}
		}
		if fi == nil {
			var err error
			fi, err = os.Lstat(path)
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		if linkEntry(fi) {
			real, err := promptSourcePath(path, recordLink, 0)
			if err != nil {
				return err
			}
			return walk(real, nil)
		}
		if fi.IsDir() {
			if light && l.isPromptSkill(path) {
				// Managed members are checked together by promptState; their
				// deployment must not invalidate unrelated inventory evidence.
				return nil
			}
			if light {
				// Codex discovers skills from declaration files. Empty folders
				// left after retiring our files do not change that inventory.
				if t.ID != "codex" && !rootDirs[path] {
					fmt.Fprintf(h, "dir:%s\n", path)
				}
				if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
					if err := walk(filepath.Join(path, "SKILL.md"), nil); err != nil {
						return err
					}
					return walk(filepath.Join(path, "agents", "openai.yaml"), nil)
				} else if !os.IsNotExist(err) {
					return err
				}
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.Name() == ".git" {
					continue
				}
				info, err := e.Info()
				if err != nil {
					return err
				}
				if err := walk(filepath.Join(path, e.Name()), info); err != nil {
					return err
				}
			}
			return nil
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("non-file command source: %s", path)
		}
		if light {
			name, ext := filepath.Base(path), filepath.Ext(path)
			declaration := name == "SKILL.md" || ext == ".toml" || ext == ".yaml" || ext == ".yml" || ext == ".json" || name == ".env"
			if !declaration {
				fmt.Fprintf(h, "file:%s:%d:%d:%d\n", path, fi.Mode(), fi.Size(), fi.ModTime().UnixNano())
				return nil
			}
		}
		total += fi.Size()
		// A bundled browser plugin alone can exceed 512 MiB. Keep full-byte
		// evidence, with a bounded budget covering it and the native executable.
		if total > 1<<30 {
			return fmt.Errorf("command sources exceed 1 GiB")
		}
		files = append(files, sourceFile{path: path, mode: fi.Mode(), size: fi.Size()})
		return nil
	}
	roots = slices.Clone(roots)
	slices.Sort(roots)
	for _, root := range roots {
		if light {
			// A previously absent discovery root containing only our own
			// deployment is equivalent to its empty state before deployment.
			fmt.Fprintf(h, "root:%s\n", root)
		}
		real, err := promptSourcePath(root, recordLink, 0)
		// Native package resolution records absent shadowing manifests too.
		// A broken candidate is still absent; its link chain above has already
		// entered the stamp. Required modules are checked by the native graph.
		if os.IsNotExist(err) || filepath.Base(root) == "package.json" && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		rootDirs[filepath.Clean(real)] = true
		info, err := os.Lstat(real)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := walk(real, info); err != nil {
			return "", err
		}
	}
	// Independent file reads can proceed together. In particular, Windows
	// installations contain many small native modules; serial opens can make
	// a content-based check exceed its probe budget. Keep ordering in the key.
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(64, len(files)) {
		workers.Go(func() {
			for i := range jobs {
				f, err := os.Open(files[i].path)
				if err != nil {
					files[i].err = err
					continue
				}
				digest := sha256.New()
				_, files[i].err = io.Copy(digest, io.LimitReader(f, files[i].size+1))
				if err := f.Close(); files[i].err == nil {
					files[i].err = err
				}
				files[i].sum = digest.Sum(nil)
			}
		})
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	for _, f := range files {
		if f.err != nil {
			return "", f.err
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00%x\n", f.path, f.mode, f.size, f.sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (l *Library) nativePromptEvidence(t PromptTarget) (nativePromptCheck, error) {
	k := nativePromptCacheKey(t)
	nativePromptChecks.Lock()
	c, ok := nativePromptChecks.m[k]
	nativePromptChecks.Unlock()
	if !ok {
		return c, &promptRuntimeError{"Check native command availability again", t.Name}
	}
	s, err := l.nativePromptSnapshotMode(t, c.roots, true)
	if err != nil {
		nativePromptChecks.Lock()
		delete(nativePromptChecks.m, k)
		nativePromptChecks.Unlock()
		return c, err
	}
	if !ok || c.key != s.key {
		nativePromptChecks.Lock()
		delete(nativePromptChecks.m, k)
		nativePromptChecks.Unlock()
		return c, &promptRuntimeError{"Check native command availability again", t.Name}
	}
	if c.err != nil {
		return c, c.err
	}
	return c, nil
}

func (l *Library) nativePromptConflict(t PromptTarget, p *Prompt) (string, []string, error) {
	c, err := l.nativePromptEvidence(t)
	if err != nil {
		return "", nil, err
	}
	for _, cmd := range c.commands {
		if promptKey(cmd.Name) != promptKey(p.Name) {
			continue
		}
		if cmd.Enabled != nil && !*cmd.Enabled {
			files, _ := t.files(p, "")
			for path := range files {
				if promptSamePath(path, cmd.Path) {
					return "", nil, &promptRuntimeError{"This skill is disabled in Codex configuration", cmd.Path}
				}
			}
		}
		if cmd.managed || cmd.Path != "" && l.promptOwns(t.ID, p.ID, cmd.Path) {
			for _, notice := range c.notices {
				if notice.SkillPath != "" && promptSamePath(notice.SkillPath, cmd.Path) {
					return "", nil, &promptRuntimeError{"Native skill metadata could not be loaded; invocation policy is unverified", notice.Source}
				}
			}
			continue
		}
		if cmd.Path != "" {
			return cmd.Path, nil, nil
		}
		return t.Name + " /" + cmd.Name, nil, nil
	}
	return "", c.dshWarnings, nil
}

func (l *Library) checkNativePromptCommands(ctx context.Context, t PromptTarget) {
	k := nativePromptCacheKey(t)
	nativePromptChecks.Lock()
	nativePromptChecks.next++
	seq := nativePromptChecks.next
	nativePromptChecks.seq[k] = seq
	delete(nativePromptChecks.m, k)
	nativePromptChecks.Unlock()
	s, err := l.nativePromptSnapshot(t, nil)
	if err != nil {
		nativePromptChecks.Lock()
		if nativePromptChecks.seq[k] == seq {
			nativePromptChecks.m[k] = nativePromptCheck{err: err, checkedAt: time.Now().UnixMilli()}
		}
		nativePromptChecks.Unlock()
		return
	}
	c := nativePromptCheck{key: s.key}
	if t.ID == "dsh" {
		// This budget includes two isolated native boots and a full content
		// fingerprint of the active dependency graph between them.
		step, cancel := context.WithTimeout(ctx, 90*time.Second)
		var first, current dshPromptInventory
		first, c.err = readDSHPromptInventory(step, t, s)
		c.roots = first.Sources
		if c.err == nil {
			s, c.err = l.nativePromptSnapshot(t, c.roots)
			c.key = s.key
			if c.err == nil {
				current, c.err = readDSHPromptInventory(step, t, s)
				c.commands, c.dshWarnings = current.Commands, current.Warnings
				before, _ := json.Marshal(first)
				after, _ := json.Marshal(current)
				if c.err == nil && string(before) != string(after) {
					c.err = &promptRuntimeError{"Native command sources changed during verification", t.Name}
				}
			}
		}
		cancel()
	} else {
		// One native enumeration is the recent inventory evidence. Record the
		// returned sources for later invalidation; this does not certify a future
		// invocation or require a second app-server boot.
		step, cancel := context.WithTimeout(ctx, 30*time.Second)
		var first codexPromptInventory
		first, c.err = readCodexPromptSkills(step, s.bin, t.dir, s.dirs)
		if c.err == nil {
			c.commands, c.notices = first.Commands, first.Notices
			for _, cmd := range first.Commands {
				c.roots = append(c.roots, filepath.Dir(cmd.Path))
			}
			s, c.err = l.nativePromptSnapshot(t, c.roots)
			c.key = s.key
		}
		cancel()
	}
	if c.err == nil {
		for i := range c.commands {
			for _, a := range l.Applied {
				for pid := range a.Prompts {
					if c.commands[i].Path != "" && l.promptOwns(t.ID, pid, c.commands[i].Path) {
						c.commands[i].managed = true
					}
				}
			}
		}
	}
	after, err := l.nativePromptSnapshot(t, c.roots)
	if err != nil || after.key != c.key {
		c.err = &promptRuntimeError{"Native command sources changed during verification", t.Name}
	}
	if t.ID == "dsh" {
		// Keep full-byte checks around both native enumerations. Subsequent
		// reads reuse that result while declarations and source identities match.
		passive, err := l.nativePromptSnapshotMode(t, c.roots, true)
		c.key = passive.key
		if err != nil && c.err == nil {
			c.err = err
		}
	}
	if ctx.Err() != nil {
		c.err = &promptRuntimeError{"Native command verification was cancelled", t.Name}
	}
	c.checkedAt = time.Now().UnixMilli()
	nativePromptChecks.Lock()
	if nativePromptChecks.seq[k] == seq {
		nativePromptChecks.m[k] = c
	}
	nativePromptChecks.Unlock()
}

// readCodexPromptSkills uses only initialize + skills/list, never thread/start,
// turn/start or a user prompt. Each cwd must be represented with explicit lists.
func readCodexPromptSkills(ctx context.Context, bin, dir string, cwds []string) (codexPromptInventory, error) {
	bad := func(reason string) (codexPromptInventory, error) {
		return codexPromptInventory{}, &promptRuntimeError{reason, "Codex CLI"}
	}
	cmd := proc.Command(bin, "app-server")
	cmd.Dir = cwds[0]
	cmd.Env = append(os.Environ(), "CODEX_HOME="+dir, "LOG_FORMAT=json")
	cmd.WaitDelay = time.Second
	in, err := cmd.StdinPipe()
	if err != nil {
		return bad("Could not start native skill verification")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return bad("Could not start native skill verification")
	}
	stderr := &boundedPiOutput{}
	cmd.Stderr = stderr
	tree, err := proc.StartTree(cmd)
	if err != nil {
		in.Close()
		return bad("Could not start native skill verification")
	}
	done := make(chan struct{})
	go func() { tree.Wait(); close(done) }()
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			in.Close()
			tree.Kill()
			out.Close()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
			}
		})
	}
	defer stop()
	type answer struct {
		commands []nativePromptCommand
		warnings []string
		err      error
	}
	answers := make(chan answer, 1)
	go func() {
		finish := func(reason string) { _, err := bad(reason); answers <- answer{err: err} }
		write := func(v any) bool { return json.NewEncoder(in).Encode(v) == nil }
		if !write(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "magpie-prompt-check", "title": "Magpie prompt check", "version": "1"}}}) {
			finish("Native skill verification returned invalid output")
			return
		}
		scan := bufio.NewScanner(io.LimitReader(out, (1<<20)+1))
		scan.Buffer(make([]byte, 4096), 1<<20)
		total := 0
		initialized := false
		var warnings []string
		for scan.Scan() {
			total += len(scan.Bytes()) + 1
			if total > 1<<20 {
				finish("Native skill verification returned invalid output")
				return
			}
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(scan.Bytes(), &msg) != nil || len(msg.Error) > 0 && string(msg.Error) != "null" {
				finish("Native skill verification returned invalid output")
				return
			}
			if msg.Method == "skills/changed" {
				finish("Native command sources changed during verification")
				return
			}
			if msg.Method == "configWarning" && len(msg.ID) == 0 && len(msg.Result) == 0 && len(msg.Error) == 0 {
				var warning struct {
					Summary *string         `json:"summary"`
					Details json.RawMessage `json:"details"`
					Path    json.RawMessage `json:"path"`
					Range   json.RawMessage `json:"range"`
				}
				decoder := json.NewDecoder(strings.NewReader(string(msg.Params)))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&warning) == nil && warning.Summary != nil {
					empty := func(value json.RawMessage) bool { return len(value) == 0 || string(value) == "null" }
					if empty(warning.Details) && empty(warning.Path) && empty(warning.Range) && codexIgnoredPromptSetting(*warning.Summary, dir) {
						warnings = append(warnings, *warning.Summary)
						continue
					}
					// Unknown warnings remain blocking; arbitrary summaries or
					// details may contain credentials and are never displayed.
					finish("Codex configuration contains warnings; native verification is incomplete")
					return
				}
			}
			// Native app-server sends remote connection status independently of
			// skill loading. Only its notification form (no id) is harmless.
			// A request using the same method must never certify an inventory.
			if msg.Method == "remoteControl/status/changed" && len(msg.ID) == 0 && len(msg.Result) == 0 && len(msg.Error) == 0 {
				var status struct {
					Status         *string `json:"status"`
					ServerName     *string `json:"serverName"`
					InstallationID *string `json:"installationId"`
					EnvironmentID  *string `json:"environmentId"`
				}
				if json.Unmarshal(msg.Params, &status) == nil && status.Status != nil && status.ServerName != nil && status.InstallationID != nil && slices.Contains([]string{"disabled", "connecting", "connected", "errored"}, *status.Status) {
					continue
				}
			}
			// An authorization request or unsupported notification cannot be
			// silently skipped and followed by an apparently complete inventory.
			if msg.Method != "" {
				finish("Native skill verification returned invalid output")
				return
			}
			var id int
			if len(msg.ID) == 0 || string(msg.ID) == "null" || json.Unmarshal(msg.ID, &id) != nil {
				finish("Native skill verification returned invalid output")
				return
			}
			if id == 1 && !initialized {
				var init map[string]any
				if json.Unmarshal(msg.Result, &init) != nil || init == nil {
					finish("Native skill verification returned invalid output")
					return
				}
				initialized = true
				if !write(map[string]any{"method": "initialized"}) || !write(map[string]any{"id": 2, "method": "skills/list", "params": map[string]any{"cwds": cwds, "forceReload": true}}) {
					finish("Native skill verification returned invalid output")
					return
				}
				continue
			}
			if id != 2 {
				finish("Native skill verification returned invalid output")
				return
			}
			if !initialized {
				finish("Native skill verification returned invalid output")
				return
			}
			var result struct {
				Data *[]struct {
					Cwd    string                 `json:"cwd"`
					Skills *[]nativePromptCommand `json:"skills"`
					Errors *[]json.RawMessage     `json:"errors"`
				} `json:"data"`
			}
			if json.Unmarshal(msg.Result, &result) != nil || result.Data == nil || len(*result.Data) != len(cwds) {
				finish("Native skill sources could not be fully loaded")
				return
			}
			seen := map[int]bool{}
			var commands []nativePromptCommand
			for _, row := range *result.Data {
				i := slices.IndexFunc(cwds, func(cwd string) bool { return promptSamePath(cwd, row.Cwd) })
				if i < 0 || seen[i] || row.Skills == nil || row.Errors == nil || len(*row.Errors) > 0 {
					finish("Native skill sources could not be fully loaded")
					return
				}
				seen[i] = true
				for _, skill := range *row.Skills {
					if skill.Name == "" || !filepath.IsAbs(skill.Path) || skill.Enabled == nil {
						finish("Native skill sources could not be fully loaded")
						return
					}
					commands = append(commands, skill)
				}
			}
			slices.SortFunc(commands, func(a, b nativePromptCommand) int { return strings.Compare(a.Name+"\x00"+a.Path, b.Name+"\x00"+b.Path) })
			answers <- answer{commands: commands, warnings: warnings}
			return
		}
		finish("Native skill sources could not be fully loaded")
	}()
	select {
	case a := <-answers:
		stop()
		if a.err != nil {
			return codexPromptInventory{}, a.err
		}
		inventory := codexPromptInventory{Commands: a.commands}
		if len(a.warnings) != 0 {
			inventory.Notices = append(inventory.Notices, codexPromptNotice{Source: filepath.Join(dir, "config.toml") + ": disable_response_storage"})
		}
		if reason := codexPromptDiagnostics(stderr, a.warnings, &inventory); reason != "" {
			return bad(reason)
		}
		return inventory, nil
	case <-ctx.Done():
		return bad("Native command verification timed out or was cancelled")
	}
}
