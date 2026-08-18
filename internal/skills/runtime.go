package skills

import (
	"fmt"
	"sort"
	"strings"
)

func RuntimeFor(snapshot Snapshot) (Runtime, error) {
	runtime := Runtime{Mode: ModeInline, Context: ContextFull}
	var allowed map[string]struct{}
	models := make(map[string]struct{})
	for _, activation := range snapshot.Activations {
		skill, ok := snapshot.Catalog.Skills[activation.Name]
		if !ok {
			return Runtime{}, fmt.Errorf("activated skill %q is no longer available", activation.Name)
		}
		runtime.ActivePrompts = append(runtime.ActivePrompts, "# Skill: "+skill.Name+"\n\n"+strings.ReplaceAll(skill.Body, "{{args}}", activation.Args))
		if skill.Model != "" {
			models[skill.Model] = struct{}{}
		}
		if len(skill.Tools) > 0 {
			current := make(map[string]struct{}, len(skill.Tools))
			for _, tool := range skill.Tools {
				current[tool] = struct{}{}
			}
			if allowed == nil {
				allowed = current
			} else {
				for tool := range allowed {
					if _, ok := current[tool]; !ok {
						delete(allowed, tool)
					}
				}
			}
		}
	}
	if len(models) > 1 {
		return Runtime{}, fmt.Errorf("activated skills specify conflicting models")
	}
	for model := range models {
		runtime.Model = model
	}
	if allowed != nil {
		for tool := range allowed {
			runtime.AllowedTools = append(runtime.AllowedTools, tool)
		}
		sort.Strings(runtime.AllowedTools)
	}
	return runtime, nil
}

func DirectoryPrompt(directory []Metadata) []string {
	if len(directory) == 0 {
		return nil
	}
	lines := []string{"可以使用以下 Skill；当用户请求匹配时，调用 load_skill 加载对应流程："}
	for _, metadata := range directory {
		lines = append(lines, "- "+metadata.Name+"："+metadata.Description)
	}
	return lines
}
