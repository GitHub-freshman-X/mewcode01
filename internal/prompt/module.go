package prompt

import (
	"sort"
	"strings"
)

type ModuleKey string

const (
	ModuleIdentity    ModuleKey = "identity"
	ModuleSystemRules ModuleKey = "system_rules"
	ModuleTaskMode    ModuleKey = "task_mode"
	ModuleAction      ModuleKey = "action"
	ModuleToolUse     ModuleKey = "tool_use"
	ModuleTone        ModuleKey = "tone"
	ModuleOutput      ModuleKey = "output"
	ModuleEnvironment ModuleKey = "environment"
	ModuleCustom      ModuleKey = "custom"
	ModuleSkills      ModuleKey = "skills"
	ModuleMemory      ModuleKey = "memory"
)

type Module struct {
	Key      ModuleKey
	Title    string
	Priority int
	Content  string
	Stable   bool
	Optional bool
}

func renderModules(modules []Module) (string, []Module) {
	filtered := make([]Module, 0, len(modules))
	for _, module := range modules {
		module.Content = strings.TrimSpace(module.Content)
		if module.Optional && module.Content == "" {
			continue
		}
		filtered = append(filtered, module)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Priority < filtered[j].Priority
	})
	parts := make([]string, 0, len(filtered))
	for _, module := range filtered {
		title := strings.TrimSpace(module.Title)
		if title == "" {
			parts = append(parts, module.Content)
			continue
		}
		parts = append(parts, title+"\n"+module.Content)
	}
	return strings.Join(parts, "\n\n"), filtered
}
