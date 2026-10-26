package library

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yetone/magpie/internal/edit"
	"golang.org/x/text/unicode/norm"
)

// Prompt is an explicitly invoked text, independent of the active instructions.
// Its body is kept in library/prompts/<id>.md.
type Prompt struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Agents []string `json:"agents"`
}

type PromptView struct {
	Prompt
	Text   string        `json:"text"`
	States []PromptState `json:"states"`
	Error  string        `json:"error,omitempty"`
}

// PromptsView is the prompt template section of the Library page.
type PromptsView struct {
	Enabled bool           `json:"enabled"`
	Items   []PromptView   `json:"items"`
	Targets []PromptTarget `json:"targets"`
	Checks  []PromptState  `json:"checks"`
}

func (l *Library) promptsView() *PromptsView {
	r := &promptReader{Library: l}
	v := &PromptsView{Enabled: l.promptsEnabled(), Items: []PromptView{}, Targets: promptTargets(), Checks: []PromptState{}}
	for _, target := range v.Targets {
		v.Checks = append(v.Checks, r.promptHarnessState(target))
	}
	for _, p := range l.Prompts {
		pv := PromptView{Prompt: *p, States: []PromptState{}}
		pv.Agents = append([]string{}, p.Agents...)
		body, err := os.ReadFile(promptPath(p.ID))
		if err == nil {
			err = checkPromptText(string(body))
		}
		if err != nil {
			pv.Error = err.Error()
		} else {
			pv.Text = string(body)
		}
		for _, t := range v.Targets {
			state := r.promptState(t, p, pv.Text)
			if err != nil {
				state.Status, state.Reason = "unverified", "Could not read template text"
				state.Deployed = false
			}
			pv.States = append(pv.States, state)
		}
		v.Items = append(v.Items, pv)
	}
	return v
}

func (l *Library) promptsEnabled() bool {
	if l.PromptsEnabled != nil {
		return *l.PromptsEnabled
	}
	return len(l.Prompts) > 0
}

// SetPromptsEnabledContext changes deployment without discarding templates or
// their selected targets. Only an explicit activation may start Harnesses.
func SetPromptsEnabledContext(ctx context.Context, enabled bool) (*Result, error) {
	mu.Lock()
	l, err := load()
	activate := err == nil && enabled && !l.promptsEnabled()
	if err == nil {
		l.PromptsEnabled = &enabled
		err = l.save()
	}
	mu.Unlock()
	if err != nil {
		return nil, err
	}
	if activate {
		for _, target := range promptTargets() {
			if !target.Detected {
				continue
			}
			switch target.ID {
			case "pi":
				l.checkPiCommands(ctx, target)
			case "codex", "dsh":
				l.checkNativePromptCommands(ctx, target)
			}
		}
	}
	// Read the current switch again: a close made while verification was
	// running must not be undone by the earlier activation finishing.
	res, err := changePrompts(func(current *Library) error {
		// Activation releases the lock for native checks, but its initial
		// index change and later deployment changes share one backup.
		current.kept = l.kept
		return nil
	})
	if activate && err == nil {
		view, readErr := ReadPrompts()
		if readErr != nil {
			return res, readErr
		}
		if view.Enabled {
			for _, state := range view.Checks {
				if state.Status == "unverified" {
					res.fail(state.Agent, "prompts", fmt.Errorf("%s: %s", state.Reason, state.Source))
				}
			}
		}
	}
	return res, err
}

// promptHarnessState describes inventory evidence without inventing a template
// name. Template-specific collisions and file ownership are checked separately.
func (l *Library) promptHarnessState(t PromptTarget) PromptState {
	return (&promptReader{Library: l}).promptHarnessState(t)
}

func (r *promptReader) promptHarnessState(t PromptTarget) PromptState {
	l := r.Library
	s := PromptState{Agent: t.ID, Status: "available"}
	if !t.Detected {
		s.Status, s.Reason = "unsupported", "Not installed here"
		return s
	}
	if !l.promptsEnabled() {
		s.Status, s.Reason = "disabled", "Prompt templates are turned off"
		return s
	}
	var err error
	switch t.ID {
	case "pi":
		_, err = r.piCommandConflict(t, nil)
		s.CheckedAt = r.pi[t.dir].check.checkedAt
	case "codex", "dsh":
		c, e := r.nativePromptEvidence(t)
		err, s.CheckedAt = e, c.checkedAt
	}
	if err != nil {
		s.Status, s.Reason, s.Source = "unverified", "Could not check command sources", err.Error()
		if e, ok := err.(*promptRuntimeError); ok {
			s.Reason, s.Source = e.reason, e.detail
		}
	}
	return s
}

func promptLastChecked(t PromptTarget) int64 {
	if t.ID == "pi" {
		piCommandChecks.Lock()
		defer piCommandChecks.Unlock()
		return piCommandChecks.m[t.dir].checkedAt
	}
	nativePromptChecks.Lock()
	defer nativePromptChecks.Unlock()
	return nativePromptChecks.m[nativePromptCacheKey(t)].checkedAt
}

// PromptChange saves the complete form. A nil Agents keeps an existing
// selection, or selects compatible detected targets for a new template.
type PromptChange struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Text   string   `json:"text"`
	Agents []string `json:"agents"`
	// Passive is used by save preflight: it must not start Harnesses or load extensions/plugins.
	Passive bool `json:"passive,omitempty"`
	// Target limits an explicit check to one Harness; an empty value checks all.
	Target string `json:"target,omitempty"`
	// Force refreshes successful evidence instead of reusing this process's cache.
	Force bool `json:"force,omitempty"`
}

func promptPath(id string) string  { return filepath.Join(Dir(), "prompts", id+".md") }
func promptKey(name string) string { return strings.ToLower(norm.NFKC.String(name)) }

// Keep the same Codex selection as skill management. Older clients and bundles
// may still name Desktop; ownership stays under its original id until sync can
// safely retire or transfer it, without rewriting hashes or claiming acceptance.
func promptAgents(ids []string) []string {
	if ids == nil {
		return nil
	}
	out := append([]string{}, ids...)
	for i, id := range out {
		if id == "codex-desktop" {
			out[i] = "codex"
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (l *Library) putPromptText(id, text string) error {
	path := promptPath(id)
	if b, err := os.ReadFile(path); err == nil && string(b) == text {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if l.kept == nil {
		l.kept = newBackups()
	}
	if err := l.kept.keep("library", path); err != nil {
		return err
	}
	return edit.WriteAtomic(path, []byte(text))
}

func checkPromptName(name string) error {
	if len(name) == 0 || len(name) > 60 {
		return fmt.Errorf("prompt name must contain 1–60 characters")
	}
	for i, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		if r != '-' || i == 0 || i == len(name)-1 || name[i-1] == '-' {
			return fmt.Errorf("prompt name: use lowercase letters and digits separated by single hyphens")
		}
	}
	return nil
}

// Stored names keep their original validation so an older template never
// prevents reading, exporting, deselecting or deleting the rest of the library.
func checkStoredPromptName(name string) error {
	if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > 60 {
		return fmt.Errorf("prompt name must contain 1–60 characters")
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '-' && r != '_' {
			return fmt.Errorf("prompt name: use Chinese or English letters, digits, - and _")
		}
	}
	return nil
}

func checkPromptText(text string) error {
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" || strings.ContainsRune(text, 0) {
		return fmt.Errorf("prompt text is empty or invalid UTF-8")
	}
	return nil
}

func checkPromptRecords(ps []*Prompt) error {
	ids, names := map[string]bool{}, map[string]bool{}
	for _, p := range ps {
		if p == nil {
			return fmt.Errorf("empty prompt metadata")
		}
		if err := checkName("prompt id", p.ID); err != nil {
			return err
		}
		if err := checkStoredPromptName(p.Name); err != nil {
			return err
		}
		if ids[p.ID] || names[promptKey(p.Name)] {
			return fmt.Errorf("duplicate prompt %s", p.Name)
		}
		ids[p.ID], names[promptKey(p.Name)] = true, true
		for _, id := range p.Agents {
			if err := checkAgent(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *Library) prompt(id string) *Prompt {
	for _, p := range l.Prompts {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (l *Library) checkPrompt(c PromptChange) error {
	if err := checkPromptName(c.Name); err != nil {
		return err
	}
	for _, id := range c.Agents {
		if err := checkAgent(id); err != nil {
			return err
		}
		if _, ok := promptTargetByID(promptTargets(), id); !ok {
			return fmt.Errorf("native templates are unavailable for target %s", id)
		}
	}
	if c.ID != "" && l.prompt(c.ID) == nil {
		return fmt.Errorf("no prompt called %s", c.ID)
	}
	for _, p := range l.Prompts {
		if p.ID != c.ID && promptKey(p.Name) == promptKey(c.Name) {
			return fmt.Errorf("the library already has a prompt called %s", p.Name)
		}
	}
	return nil
}

// ReadPrompts reads only templates, without scanning the other Library parts.
func ReadPrompts() (*PromptsView, error) {
	mu.Lock()
	defer mu.Unlock()
	l, err := load()
	if err != nil {
		return nil, err
	}
	return l.promptsView(), nil
}

// CheckPrompt explicitly checks availability. It does not deploy files, but
// native runtime checks load extensions/plugins whose startup may have side effects.
func CheckPrompt(c PromptChange) ([]PromptState, error) {
	return CheckPromptContext(context.Background(), c)
}

// CheckPromptContext lets the HTTP request cancel an explicit runtime check.
func CheckPromptContext(ctx context.Context, c PromptChange) ([]PromptState, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Agents = promptAgents(c.Agents)
	targets := promptTargets()
	if c.Target != "" {
		t, ok := promptTargetByID(targets, c.Target)
		if !ok {
			return nil, fmt.Errorf("native templates are unavailable for target %s", c.Target)
		}
		targets = []PromptTarget{t}
	}
	mu.Lock()
	l, err := load()
	global := c.Name == "" && c.ID == "" && c.Text == ""
	if err == nil && !global {
		err = l.checkPrompt(c)
	}
	mu.Unlock()
	if err != nil {
		return nil, err
	}
	if !c.Passive {
		for _, target := range targets {
			if target.Detected {
				switch target.ID {
				case "pi":
					if !c.Force {
						if _, e := l.piCommandConflict(target, nil); e == nil {
							continue
						}
					}
					l.checkPiCommands(ctx, target)
				case "codex", "dsh":
					if !c.Force {
						if _, e := l.nativePromptEvidence(target); e == nil {
							continue
						}
					}
					l.checkNativePromptCommands(ctx, target)
				}
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	l, err = load()
	if err != nil {
		return nil, err
	}
	if !global {
		if err := l.checkPrompt(c); err != nil {
			return nil, err
		}
	}
	out := []PromptState{}
	p := &Prompt{ID: c.ID, Name: c.Name}
	for _, target := range targets {
		var state PromptState
		if global {
			state = l.promptHarnessState(target)
		} else {
			state = l.promptState(target, p, c.Text)
		}
		state.CheckedAt = promptLastChecked(target)
		out = append(out, state)
	}
	return out, nil
}

func changePrompts(f func(*Library) error) (*Result, error) {
	mu.Lock()
	defer mu.Unlock()
	l, err := load()
	if err != nil {
		return nil, err
	}
	if err := f(l); err != nil {
		return nil, err
	}
	if err := l.save(); err != nil {
		return nil, err
	}
	res := &Result{Changed: []string{}, Problems: []Problem{}}
	b := l.kept
	if b == nil {
		b = newBackups()
	}
	l.syncPrompts(b, res)
	res.Backup = b.dir
	if b.dir != "" {
		pruneBackups()
	}
	return res, l.save()
}

// SavePrompt keeps a template independently of the active instruction set.
func SavePrompt(c PromptChange) (*Result, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Agents = promptAgents(c.Agents)
	if err := checkPromptText(c.Text); err != nil {
		return nil, err
	}
	return changePrompts(func(l *Library) error {
		if err := l.checkPrompt(c); err != nil {
			return err
		}
		p := l.prompt(c.ID)
		if p == nil {
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				return err
			}
			p = &Prompt{ID: "p" + hex.EncodeToString(id[:]), Agents: []string{}}
			if c.Agents == nil {
				for _, t := range promptTargets() {
					if s := l.promptState(t, &Prompt{ID: p.ID, Name: c.Name}, c.Text); promptUsable(s) {
						p.Agents = append(p.Agents, t.ID)
					}
				}
			}
		}
		if err := l.putPromptText(p.ID, strings.ReplaceAll(c.Text, "\r\n", "\n")); err != nil {
			return err
		}
		if c.Agents != nil {
			p.Agents = slices.Compact(slices.Sorted(slices.Values(c.Agents)))
		}
		p.Name = c.Name
		if l.prompt(p.ID) == nil {
			l.Prompts = append(l.Prompts, p)
		}
		return nil
	})
}

// SetPromptAgents updates only a template's targets, using its latest name and
// body under the library lock. It never starts Pi or rewrites the body. Existing
// selections survive failed checks; only explicit deselection retires them.
func SetPromptAgents(id string, agents []string) (*Result, error) {
	if agents == nil {
		return nil, fmt.Errorf("prompt targets must be a list")
	}
	agents = promptAgents(agents)
	var blocked []Problem
	res, err := changePrompts(func(l *Library) error {
		p := l.prompt(id)
		if p == nil {
			return fmt.Errorf("no prompt called %s", id)
		}
		targets := promptTargets()
		for _, agent := range agents {
			if _, ok := promptTargetByID(targets, agent); !ok && !slices.Contains(p.Agents, agent) {
				return fmt.Errorf("unknown prompt target: %s", agent)
			}
		}
		body, bodyErr := os.ReadFile(promptPath(id))
		if bodyErr == nil {
			bodyErr = checkPromptText(string(body))
		}
		next := []string{}
		for _, agent := range slices.Compact(slices.Sorted(slices.Values(agents))) {
			if slices.Contains(p.Agents, agent) {
				next = append(next, agent)
				continue
			}
			target, _ := promptTargetByID(targets, agent)
			state := l.promptState(target, p, string(body))
			if bodyErr != nil {
				state.Status, state.Reason = "unverified", bodyErr.Error()
			}
			if !promptUsable(state) {
				blocked = append(blocked, Problem{Agent: agent, What: "prompt:" + p.Name, Error: strings.TrimSpace(state.Reason + " " + state.Source)})
				continue
			}
			next = append(next, agent)
		}
		p.Agents = next
		return nil
	})
	if res != nil {
		res.Problems = append(res.Problems, blocked...)
	}
	return res, err
}

// SetPromptHarnessContext changes one Harness's selection for current templates.
// Other Harness selections and template bodies remain unchanged.
func SetPromptHarnessContext(ctx context.Context, target string, enabled bool) (*Result, error) {
	if _, ok := promptTargetByID(promptTargets(), target); !ok {
		return nil, fmt.Errorf("unknown prompt target: %s", target)
	}
	mu.Lock()
	l, err := load()
	var ids []string
	if err == nil {
		if !l.promptsEnabled() {
			err = fmt.Errorf("Prompt templates are turned off")
		} else {
			for _, p := range l.Prompts {
				ids = append(ids, p.ID)
			}
		}
	}
	mu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &Result{Changed: []string{}, Problems: []Problem{}}, nil
	}
	if enabled {
		if _, err := CheckPromptContext(ctx, PromptChange{Target: target}); err != nil {
			return nil, err
		}
	}
	var blocked []Problem
	res, err := changePrompts(func(l *Library) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !l.promptsEnabled() {
			return fmt.Errorf("Prompt templates are turned off")
		}
		t, _ := promptTargetByID(promptTargets(), target)
		r := &promptReader{Library: l}
		for _, p := range l.Prompts {
			// A template created while the native check runs is outside this operation.
			if !slices.Contains(ids, p.ID) {
				continue
			}
			if !enabled {
				p.Agents = slices.DeleteFunc(p.Agents, func(id string) bool { return id == target })
				continue
			}
			if slices.Contains(p.Agents, target) {
				continue // Sync retries existing selections and reports protected files.
			}
			body, bodyErr := os.ReadFile(promptPath(p.ID))
			if bodyErr == nil {
				bodyErr = checkPromptText(string(body))
			}
			state := r.promptState(t, p, string(body))
			if bodyErr != nil {
				state.Status, state.Reason, state.Source = "unverified", "Could not read template text", ""
			}
			if !promptUsable(state) {
				blocked = append(blocked, Problem{Agent: target, What: "prompt:" + p.Name, Error: strings.TrimSpace(state.Reason + " " + state.Source)})
				continue
			}
			p.Agents = promptAgents(append(p.Agents, target))
		}
		return nil
	})
	if res != nil {
		res.Problems = append(res.Problems, blocked...)
	}
	return res, err
}

// RemovePrompt removes a template from the library.
func RemovePrompt(id string) (*Result, error) {
	return changePrompts(func(l *Library) error {
		p := l.prompt(id)
		if p == nil {
			return fmt.Errorf("no prompt called %s", id)
		}
		if l.kept == nil {
			l.kept = newBackups()
		}
		if err := l.kept.keep("library", promptPath(id)); err != nil {
			return err
		}
		if err := os.Remove(promptPath(id)); err != nil && !os.IsNotExist(err) {
			return err
		}
		l.Prompts = slices.DeleteFunc(l.Prompts, func(p *Prompt) bool { return p.ID == id })
		return nil
	})
}
