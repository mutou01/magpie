package library

// Pi 0.87.1: dist/modes/rpc/rpc-mode.js get_commands returns extension
// invocationName, prompt names and skill:<name>. Built-ins remain a separate
// catalog. This protocol is not MCP and loading extensions is not a sandbox.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/proc"
)

type promptRuntimeError struct{ reason, detail string }

func (e *promptRuntimeError) Error() string { return e.reason + ": " + e.detail }

type piCommand struct {
	managed    bool   // owned when captured; a rename must not leave a stale conflict
	Name       string `json:"name"`
	Source     string `json:"source"`
	SourceInfo struct {
		Path string `json:"path"`
	} `json:"sourceInfo"`
}

type piCommandCheck struct {
	key      string
	commands []piCommand
	err      error
}

var piCommandChecks = struct {
	sync.Mutex
	m    map[string]piCommandCheck
	seq  map[string]uint64
	next uint64
}{m: map[string]piCommandCheck{}, seq: map[string]uint64{}}
var piCheckBudget = 30 * time.Second

func (l *Library) piCommandConflict(t PromptTarget, p *Prompt) (string, error) {
	s, err := l.piCommandSnapshot(t)
	if err != nil {
		piCommandChecks.Lock()
		delete(piCommandChecks.m, t.dir)
		piCommandChecks.Unlock()
		return "", err
	}
	if !s.dynamic {
		return "", nil
	}
	piCommandChecks.Lock()
	c, ok := piCommandChecks.m[t.dir]
	piCommandChecks.Unlock()
	if !ok || c.key != s.key {
		return "", &promptRuntimeError{"Check Pi command availability again", t.dir}
	}
	if c.err != nil {
		return "", c.err
	}
	for _, cmd := range c.commands {
		if cmd.managed {
			continue
		}
		if promptKey(cmd.Name) != promptKey(p.Name) {
			continue
		}
		path := cmd.SourceInfo.Path
		if cmd.Source == "prompt" && path != "" && l.promptOwns("pi", p.ID, path) {
			continue
		}
		if path == "" {
			path = "/" + cmd.Name
		}
		return path, nil
	}
	return "", nil
}

func (l *Library) checkPiCommands(ctx context.Context, t PromptTarget) {
	piCommandChecks.Lock()
	piCommandChecks.next++
	seq := piCommandChecks.next
	piCommandChecks.seq[t.dir] = seq
	delete(piCommandChecks.m, t.dir)
	piCommandChecks.Unlock()
	s, err := l.piCommandSnapshot(t)
	if err != nil || !s.dynamic {
		return
	}
	c := piCommandCheck{key: s.key}
	for _, cwd := range s.dirs {
		step, cancel := context.WithTimeout(ctx, piCheckBudget)
		version, err := piCommandVersion(step, s.bin, cwd)
		var commands []piCommand
		if err == nil && version != "0.87.1" {
			err = &promptRuntimeError{"Pi version awaits command verification", version}
		}
		if err == nil {
			commands, err = readPiCommands(step, s.bin, t.dir, cwd)
		}
		cancel()
		if err != nil {
			c.err = err
			break
		}
		// Keep the complete native list, but remember which prompt entries
		// belonged to magpie. Their files are deliberately excluded from the
		// source stamp, so renaming/removing one must not retain its old name.
		if a := l.Applied["pi"]; a != nil {
			for i := range commands {
				if commands[i].Source == "prompt" {
					for pid := range a.Prompts {
						if l.promptOwns("pi", pid, commands[i].SourceInfo.Path) {
							commands[i].managed = true
							break
						}
					}
				}
			}
		}
		c.commands = append(c.commands, commands...)
	}
	// Never publish a result for resources that changed during startup. Readers
	// also compare against their current Library/project snapshot before use.
	after, err := l.piCommandSnapshot(t)
	if err != nil || after.key != s.key {
		c.err = &promptRuntimeError{"Pi command sources changed during verification", t.dir}
	}
	if ctx.Err() != nil {
		c.err = &promptRuntimeError{"Pi command verification was cancelled", t.dir}
	}
	piCommandChecks.Lock()
	if piCommandChecks.seq[t.dir] == seq {
		piCommandChecks.m[t.dir] = c
	}
	piCommandChecks.Unlock()
}

func piCommandVersion(ctx context.Context, bin, cwd string) (string, error) {
	if ctx.Err() != nil {
		return "", &promptRuntimeError{"Pi command verification timed out or was cancelled", cwd}
	}
	cmd := proc.Command(bin, "--version")
	cmd.Dir = cwd
	cmd.WaitDelay = time.Second
	out, errs := &boundedPiOutput{}, &boundedPiOutput{}
	cmd.Stdout, cmd.Stderr = out, errs
	tree, err := proc.StartTree(cmd)
	if err != nil {
		return "", &promptRuntimeError{"Could not start Pi command verification", cwd}
	}
	done := make(chan error, 1)
	go func() { done <- tree.Wait() }()
	select {
	case err := <-done:
		if err != nil || errs.failed() || out.tooLarge {
			return "", &promptRuntimeError{"Pi version awaits command verification", cwd}
		}
		return strings.TrimSpace(out.b.String()), nil
	case <-ctx.Done():
		tree.Kill()
		<-done
		return "", &promptRuntimeError{"Pi command verification timed out or was cancelled", cwd}
	}
}

// boundedPiOutput is shared by stdout/stderr. A noisy extension cannot allocate
// unbounded buffers, and diagnostics on stderr invalidate a partial command list.
type boundedPiOutput struct {
	sync.Mutex
	b        strings.Builder
	tooLarge bool
}

func (w *boundedPiOutput) Write(b []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	if w.b.Len()+len(b) > 1<<20 {
		w.tooLarge = true
		return 0, fmt.Errorf("Pi output exceeds 1 MiB")
	}
	return w.b.Write(b)
}
func (w *boundedPiOutput) failed() bool {
	w.Lock()
	defer w.Unlock()
	return w.tooLarge || strings.TrimSpace(w.b.String()) != ""
}

func readPiCommands(ctx context.Context, bin, dir, cwd string) ([]piCommand, error) {
	bad := func(reason string) ([]piCommand, error) { return nil, &promptRuntimeError{reason, cwd} }
	if ctx.Err() != nil {
		return bad("Pi command verification timed out or was cancelled")
	}
	cmd := proc.Command(bin, "--mode", "rpc", "--no-session", "--offline")
	cmd.Dir = cwd
	cmd.WaitDelay = time.Second
	// Pin the config that the target describes; never inherit a relative Pi home.
	cmd.Env = append(os.Environ(), "PI_CODING_AGENT_DIR="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return bad("Could not start Pi command verification")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return bad("Could not start Pi command verification")
	}
	stderr := &boundedPiOutput{}
	cmd.Stderr = stderr
	tree, err := proc.StartTree(cmd)
	if err != nil {
		stdin.Close()
		return bad("Could not start Pi command verification")
	}
	exited := make(chan struct{})
	go func() { tree.Wait(); close(exited) }()
	var once sync.Once
	stopped := false
	stop := func() {
		once.Do(func() {
			stdin.Close()
			tree.Kill()
			stdout.Close()
			select {
			case <-exited:
				stopped = true
			case <-time.After(3 * time.Second):
			}
		})
	}
	defer stop()
	type answer struct {
		commands []piCommand
		reason   string
	}
	answers := make(chan answer, 1)
	go func() {
		finish := func(a answer) { answers <- a }
		scanner := bufio.NewScanner(io.LimitReader(stdout, (1<<20)+1))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		total := 0
		for scanner.Scan() {
			total += len(scanner.Bytes()) + 1
			if total > 1<<20 {
				finish(answer{reason: "Pi command verification returned invalid output"})
				return
			}
			var msg struct {
				ID      string `json:"id"`
				Type    string `json:"type"`
				Command string `json:"command"`
				Success bool   `json:"success"`
				Data    struct {
					Commands *[]piCommand `json:"commands"`
				} `json:"data"`
			}
			if json.Unmarshal(scanner.Bytes(), &msg) != nil {
				finish(answer{reason: "Pi command verification returned invalid output"})
				return
			}
			if msg.Type == "extension_ui_request" {
				finish(answer{reason: "Pi command verification needs interaction"})
				return
			}
			if msg.Type == "extension_error" {
				finish(answer{reason: "Pi extensions could not be fully loaded"})
				return
			}
			if msg.Type != "response" || msg.ID != "magpie-commands" {
				continue
			}
			if msg.Command != "get_commands" || !msg.Success || msg.Data.Commands == nil {
				finish(answer{reason: "Pi command verification returned invalid output"})
				return
			}
			for _, c := range *msg.Data.Commands {
				if c.Name == "" || !strings.Contains("|extension|prompt|skill|", "|"+c.Source+"|") {
					finish(answer{reason: "Pi command verification returned invalid output"})
					return
				}
			}
			finish(answer{commands: *msg.Data.Commands})
			return
		}
		finish(answer{reason: "Pi extensions could not be fully loaded"})
	}()
	// Do not send user text, prompt, reload or extension UI responses.
	writeDone := make(chan error, 1)
	go func() {
		_, err := io.WriteString(stdin, "{\"id\":\"magpie-commands\",\"type\":\"get_commands\"}\n")
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			return bad("Pi extensions could not be fully loaded")
		}
	case <-ctx.Done():
		return bad("Pi command verification timed out or was cancelled")
	}
	select {
	case a := <-answers:
		if a.reason != "" {
			return bad(a.reason)
		}
		// Wait for stderr's copier too: a command response can arrive before
		// the startup diagnostic on the other pipe has been consumed.
		stop()
		if !stopped || stderr.failed() {
			return bad("Pi extensions could not be fully loaded")
		}
		return a.commands, nil
	case <-ctx.Done():
		return bad("Pi command verification timed out or was cancelled")
	}
}
