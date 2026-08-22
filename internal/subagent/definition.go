package subagent

import (
	"embed"
	"fmt"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"go.yaml.in/yaml/v4"
)

type Source string

const (
	SourceProject Source = "project"
	SourceUser    Source = "user"
	SourceBuiltin Source = "builtin"
	SourcePlugin  Source = "plugin"
)

type CreationMode string

const (
	CreationDefinition CreationMode = "definition"
	CreationFork       CreationMode = "fork"
)

type Definition struct {
	Name            string
	Description     string
	Tools           []string
	DisallowedTools []string
	Model           string
	MaxTurns        int
	PermissionMode  permissions.Mode
	SystemPrompt    string
	Path            string
	Source          Source
}

type Registry struct{ definitions map[string]Definition }

func NewRegistry(definitions []Definition) (*Registry, error) {
	r := &Registry{definitions: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		if err := r.Add(definition); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Add(definition Definition) error {
	if err := ValidateDefinition(definition); err != nil {
		return err
	}
	if r.definitions == nil {
		r.definitions = make(map[string]Definition)
	}
	definition.Tools = append([]string(nil), definition.Tools...)
	definition.DisallowedTools = append([]string(nil), definition.DisallowedTools...)
	r.definitions[definition.Name] = definition
	return nil
}

func (r *Registry) Get(name string) (Definition, bool) {
	definition, ok := r.definitions[strings.TrimSpace(name)]
	definition.Tools = append([]string(nil), definition.Tools...)
	definition.DisallowedTools = append([]string(nil), definition.DisallowedTools...)
	return definition, ok
}

func (r *Registry) All() []Definition {
	out := make([]Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		out = append(out, definition)
	}
	return out
}

func ValidateDefinition(definition Definition) error {
	if strings.TrimSpace(definition.Name) == "" {
		return fmt.Errorf("agent definition %s: name is required", definition.Path)
	}
	if strings.EqualFold(strings.TrimSpace(definition.Name), "fork") {
		return fmt.Errorf("agent definition %s: name %q is reserved", definition.Path, definition.Name)
	}
	if strings.TrimSpace(definition.Description) == "" {
		return fmt.Errorf("agent definition %s: description is required", definition.Path)
	}
	if strings.TrimSpace(definition.SystemPrompt) == "" {
		return fmt.Errorf("agent definition %s: body is required", definition.Path)
	}
	switch definition.Model {
	case "", "inherit", "haiku", "sonnet", "opus":
	default:
		return fmt.Errorf("agent definition %s: unsupported model %q", definition.Path, definition.Model)
	}
	if definition.MaxTurns < 0 {
		return fmt.Errorf("agent definition %s: maxTurns must not be negative", definition.Path)
	}
	switch definition.PermissionMode {
	case "", permissions.ModeStrict, permissions.ModeDefault, permissions.ModeRelaxed:
	default:
		return fmt.Errorf("agent definition %s: unsupported permissionMode %q", definition.Path, definition.PermissionMode)
	}
	return nil
}

type frontmatter struct {
	Name            string           `yaml:"name"`
	Description     string           `yaml:"description"`
	Tools           []string         `yaml:"tools"`
	DisallowedTools []string         `yaml:"disallowedTools"`
	Model           string           `yaml:"model"`
	MaxTurns        int              `yaml:"maxTurns"`
	PermissionMode  permissions.Mode `yaml:"permissionMode"`
}

func ParseDefinition(path, content string, source Source) (Definition, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return Definition{}, fmt.Errorf("agent definition %s: frontmatter must start on the first line", path)
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return Definition{}, fmt.Errorf("agent definition %s: frontmatter closing delimiter is required", path)
	}
	end += 4
	var raw frontmatter
	if err := yaml.Unmarshal([]byte(content[4:end]), &raw); err != nil {
		return Definition{}, fmt.Errorf("agent definition %s: invalid frontmatter: %w", path, err)
	}
	definition := Definition{Name: strings.TrimSpace(raw.Name), Description: strings.TrimSpace(raw.Description), Tools: raw.Tools, DisallowedTools: raw.DisallowedTools, Model: strings.TrimSpace(raw.Model), MaxTurns: raw.MaxTurns, PermissionMode: raw.PermissionMode, SystemPrompt: strings.TrimSpace(content[end+5:]), Path: path, Source: source}
	if err := ValidateDefinition(definition); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

//go:embed builtins/*.md
var builtinFiles embed.FS

func Builtins(enableVerification bool) ([]Definition, error) {
	names := []string{"explore.md", "plan.md", "general-purpose.md"}
	if enableVerification {
		names = append(names, "verification.md")
	}
	out := make([]Definition, 0, len(names))
	for _, name := range names {
		path := "builtins/" + name
		data, err := builtinFiles.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read builtin agent %s: %w", name, err)
		}
		definition, err := ParseDefinition(path, string(data), SourceBuiltin)
		if err != nil {
			return nil, err
		}
		out = append(out, definition)
	}
	return out, nil
}
