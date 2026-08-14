package command

import (
	"errors"
	"sort"
	"strings"
)

type Registry struct {
	commands []Command
	lookup   map[string]int
}

func NewRegistry(commands ...Command) (*Registry, error) {
	r := &Registry{commands: append([]Command(nil), commands...), lookup: make(map[string]int)}
	for i, command := range r.commands {
		command.Name = normalize(command.Name)
		if command.Name == "" || command.Handler == nil {
			return nil, errors.New("command name and handler are required")
		}
		r.commands[i].Name = command.Name
		for _, name := range append([]string{command.Name}, command.Aliases...) {
			key := normalize(name)
			if key == "" {
				return nil, errors.New("command alias is empty")
			}
			if _, exists := r.lookup[key]; exists {
				return nil, errors.New("duplicate command identifier: " + key)
			}
			r.lookup[key] = i
		}
	}
	return r, nil
}

func (r *Registry) Find(name string) (Command, bool) {
	if r == nil {
		return Command{}, false
	}
	i, ok := r.lookup[normalize(name)]
	if !ok {
		return Command{}, false
	}
	return r.commands[i], true
}

func (r *Registry) Visible() []Command {
	if r == nil {
		return nil
	}
	visible := make([]Command, 0, len(r.commands))
	for _, command := range r.commands {
		if !command.Hidden {
			visible = append(visible, command)
		}
	}
	return visible
}

func (r *Registry) Complete(prefix string) []string {
	prefix = strings.TrimPrefix(normalize(prefix), "/")
	seen := make(map[string]struct{})
	var matches []string
	for _, command := range r.Visible() {
		for _, name := range append([]string{command.Name}, command.Aliases...) {
			name = normalize(name)
			if strings.HasPrefix(name, prefix) {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					matches = append(matches, "/"+name)
				}
			}
		}
	}
	sort.Strings(matches)
	return matches
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "/")))
}
