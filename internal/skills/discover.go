package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

var reservedCommandNames = map[string]struct{}{
	"help": {}, "exit": {}, "compact": {}, "clear": {}, "plan": {}, "do": {},
	"session": {}, "memory": {}, "status": {}, "skills": {},
}

type DiscoverOptions struct {
	ProjectDir string
	UserDir    string
	ToolNames  []string
}

func Discover(options DiscoverOptions) (Catalog, error) {
	knownTools := make(map[string]struct{}, len(options.ToolNames))
	for _, name := range options.ToolNames {
		knownTools[name] = struct{}{}
	}
	catalog := Catalog{Skills: make(map[string]Skill)}
	if err := mergeSkills(&catalog, Builtins(), knownTools, false); err != nil {
		return Catalog{}, err
	}
	for _, root := range []string{options.UserDir, options.ProjectDir} {
		if strings.TrimSpace(root) == "" {
			continue
		}
		skills, err := discoverDirectory(root)
		if err != nil {
			return Catalog{}, err
		}
		if err := mergeSkills(&catalog, skills, knownTools, true); err != nil {
			return Catalog{}, err
		}
	}
	return catalog, nil
}

func mergeSkills(catalog *Catalog, candidates []Skill, knownTools map[string]struct{}, allowOverride bool) error {
	seen := make(map[string]struct{}, len(candidates))
	for _, skill := range candidates {
		metadata, err := ValidateMetadata(skill.Metadata)
		if err != nil {
			return fmt.Errorf("skill %s: %w", skill.Path, err)
		}
		if strings.TrimSpace(skill.Body) == "" {
			return fmt.Errorf("skill %s: body is required", skill.Path)
		}
		if _, reserved := reservedCommandNames[metadata.Name]; reserved {
			return fmt.Errorf("skill %s: name %q conflicts with built-in command", skill.Path, metadata.Name)
		}
		if _, duplicate := seen[metadata.Name]; duplicate {
			return fmt.Errorf("skill %s: duplicate skill name %q", skill.Path, metadata.Name)
		}
		seen[metadata.Name] = struct{}{}
		for _, tool := range metadata.Tools {
			if _, ok := knownTools[tool]; !ok {
				return fmt.Errorf("skill %s: tools contains unknown tool %q", skill.Path, tool)
			}
		}
		skill.Metadata = metadata
		if _, exists := catalog.Skills[metadata.Name]; exists && !allowOverride {
			return fmt.Errorf("skill %s: duplicate skill name %q", skill.Path, metadata.Name)
		}
		catalog.Skills[metadata.Name] = skill
	}
	return nil
}

func discoverDirectory(root string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan skills directory %s: %w", root, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var skills []Skill
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			entryPath := filepath.Join(path, "SKILL.md")
			if _, err := os.Stat(entryPath); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return nil, fmt.Errorf("skill %s: %w", entryPath, err)
			}
			skill, err := parseSkill(entryPath)
			if err != nil {
				return nil, err
			}
			skills = append(skills, skill)
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") || strings.EqualFold(entry.Name(), "skill.md") {
			continue
		}
		skill, err := parseSkill(path)
		if err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}
	return skills, nil
}

type frontmatter struct {
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Model       string       `yaml:"model"`
	Tools       []string     `yaml:"tools"`
	Mode        Mode         `yaml:"mode"`
	Context     ContextScope `yaml:"context"`
}

func parseSkill(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("read skill %s: %w", path, err)
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return Skill{}, fmt.Errorf("skill %s: frontmatter must start on the first line", path)
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return Skill{}, fmt.Errorf("skill %s: frontmatter closing delimiter is required", path)
	}
	end += 4
	var raw frontmatter
	if err := yaml.Unmarshal([]byte(content[4:end]), &raw); err != nil {
		return Skill{}, fmt.Errorf("skill %s: invalid frontmatter: %w", path, err)
	}
	body := strings.TrimSpace(content[end+5:])
	return Skill{Metadata: Metadata{Name: raw.Name, Description: raw.Description, Model: raw.Model, Tools: raw.Tools, Mode: raw.Mode, Context: raw.Context}, Path: path, Body: body}, nil
}
