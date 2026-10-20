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
	Items   []PromptView   `json:"items"`
	Targets []PromptTarget `json:"targets"`
}

func (l *Library) promptsView() *PromptsView {
	v := &PromptsView{Items: []PromptView{}, Targets: promptTargets()}
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
			state := l.promptState(t, p, pv.Text)
			if err != nil {
				state.Status, state.Reason = "unverified", "Could not read template text"
			}
			pv.States = append(pv.States, state)
		}
		v.Items = append(v.Items, pv)
	}
	return v
}

// PromptChange saves the complete form. A nil Agents keeps an existing
// selection, or selects compatible detected targets for a new template.
type PromptChange struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Text   string   `json:"text"`
	Agents []string `json:"agents"`
	// Passive is used by a save's preflight: it must not load extensions.
	Passive bool `json:"passive,omitempty"`
}

func promptPath(id string) string  { return filepath.Join(Dir(), "prompts", id+".md") }
func promptKey(name string) string { return strings.ToLower(norm.NFKC.String(name)) }

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
		if err := checkPromptName(p.Name); err != nil {
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
		if promptBuiltins[id] == "" {
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
// Pi's runtime check loads extensions, whose startup code may have side effects.
func CheckPrompt(c PromptChange) ([]PromptState, error) {
	return CheckPromptContext(context.Background(), c)
}

// CheckPromptContext lets the HTTP request cancel an explicit runtime check.
func CheckPromptContext(ctx context.Context, c PromptChange) ([]PromptState, error) {
	c.Name = strings.TrimSpace(c.Name)
	mu.Lock()
	l, err := load()
	if err == nil {
		err = l.checkPrompt(c)
	}
	mu.Unlock()
	if err != nil {
		return nil, err
	}
	if !c.Passive {
		if t, ok := promptTargetByID(promptTargets(), "pi"); ok && t.Detected {
			l.checkPiCommands(ctx, t)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	l, err = load()
	if err != nil {
		return nil, err
	}
	if err := l.checkPrompt(c); err != nil {
		return nil, err
	}
	out := []PromptState{}
	p := &Prompt{ID: c.ID, Name: c.Name}
	for _, target := range promptTargets() {
		out = append(out, l.promptState(target, p, c.Text))
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
