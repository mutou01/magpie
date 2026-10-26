package library

import (
	"fmt"
	"os"
	"slices"
)

// CarriedPrompt contains portable text and choices, never deployment paths.
type CarriedPrompt struct {
	Prompt
	Text string `json:"text"`
}

func checkCarriedPrompts(ps []CarriedPrompt) error {
	ids, names := map[string]bool{}, map[string]bool{}
	for _, p := range ps {
		if err := checkPromptText(p.Text); err != nil {
			return err
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

func (l *Library) collectPrompts() (*[]CarriedPrompt, error) {
	out := []CarriedPrompt{}
	for _, p := range sorted(l.Prompts, func(p *Prompt) string { return p.ID }) {
		body, err := os.ReadFile(promptPath(p.ID))
		if err == nil {
			err = checkPromptText(string(body))
		}
		if err != nil {
			return nil, err
		}
		c := *p
		c.Agents = slices.Sorted(slices.Values(p.Agents))
		if c.Agents == nil {
			c.Agents = []string{}
		}
		out = append(out, CarriedPrompt{Prompt: c, Text: string(body)})
	}
	return &out, nil
}

func (l *Library) putPrompts(ps []CarriedPrompt) error {
	if l.kept == nil {
		l.kept = newBackups()
	}
	for _, p := range l.Prompts {
		if !slices.ContainsFunc(ps, func(c CarriedPrompt) bool { return c.ID == p.ID }) {
			if err := l.kept.keep("library", promptPath(p.ID)); err != nil {
				return err
			}
			if err := os.Remove(promptPath(p.ID)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	out := []*Prompt{}
	for _, c := range ps {
		if err := l.putPromptText(c.ID, c.Text); err != nil {
			return err
		}
		p := c.Prompt
		p.Agents = append([]string{}, c.Agents...)
		out = append(out, &p)
	}
	l.Prompts = out
	return nil
}
