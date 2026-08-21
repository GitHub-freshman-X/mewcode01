package subagent

import (
	"fmt"
	"sort"
)

// All definition-based children must not recursively create agents.
var AllAgentDisallowedTools = map[string]struct{}{"agent": {}}

// Background tasks may use base workspace tools but cannot delegate work again.
var AsyncAgentAllowedTools = map[string]struct{}{
	"read_file": {}, "write_file": {}, "edit_file": {}, "run_command": {},
	"find_files": {}, "search_code": {}, "load_skill": {},
}

func FilterTools(known []string, definition Definition, background bool) ([]string, error) {
	available := make(map[string]struct{}, len(known))
	for _, name := range known {
		available[name] = struct{}{}
	}
	for _, name := range definition.Tools {
		if _, ok := available[name]; !ok {
			return nil, fmt.Errorf("agent definition %s: tools contains unknown tool %q", definition.Path, name)
		}
	}
	selected := make(map[string]struct{}, len(available))
	for name := range available {
		selected[name] = struct{}{}
	}
	for name := range AllAgentDisallowedTools {
		delete(selected, name)
	}
	for _, name := range definition.DisallowedTools {
		delete(selected, name)
	}
	if len(definition.Tools) > 0 {
		for name := range selected {
			if !contains(definition.Tools, name) {
				delete(selected, name)
			}
		}
	}
	if background {
		for name := range selected {
			if _, ok := AsyncAgentAllowedTools[name]; !ok {
				delete(selected, name)
			}
		}
	}
	out := make([]string, 0, len(selected))
	for name := range selected {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
