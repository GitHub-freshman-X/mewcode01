package skills

import (
	"fmt"
	"regexp"
	"strings"
)

type Mode string

const (
	ModeInline Mode = "inline"
	ModeFork   Mode = "fork"
)

type ContextScope string

const (
	ContextFull   ContextScope = "full"
	ContextRecent ContextScope = "recent"
	ContextNone   ContextScope = "none"
)

type Metadata struct {
	Name        string
	Description string
	Model       string
	Tools       []string
	Mode        Mode
	Context     ContextScope
}

type Skill struct {
	Metadata
	Path string
	Body string
}

type Catalog struct {
	Skills map[string]Skill
}

type Activation struct {
	Name string
	Args string
}

type Snapshot struct {
	Catalog     Catalog
	Activations []Activation
}

type Runtime struct {
	ActivePrompts []string
	AllowedTools  []string
	Mode          Mode
	Context       ContextScope
	Model         string
}

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func normalizeMetadata(metadata Metadata) Metadata {
	metadata.Name = strings.TrimSpace(metadata.Name)
	metadata.Description = strings.TrimSpace(metadata.Description)
	metadata.Model = strings.TrimSpace(metadata.Model)
	if metadata.Mode == "" {
		metadata.Mode = ModeInline
	}
	if metadata.Context == "" {
		metadata.Context = ContextFull
	}
	metadata.Tools = append([]string(nil), metadata.Tools...)
	return metadata
}

func ValidateMetadata(metadata Metadata) (Metadata, error) {
	metadata = normalizeMetadata(metadata)
	if !skillNamePattern.MatchString(metadata.Name) {
		return Metadata{}, fmt.Errorf("name %q must contain lowercase letters, digits, and hyphens only", metadata.Name)
	}
	if metadata.Description == "" {
		return Metadata{}, fmt.Errorf("description is required")
	}
	if metadata.Mode != ModeInline && metadata.Mode != ModeFork {
		return Metadata{}, fmt.Errorf("mode %q must be inline or fork", metadata.Mode)
	}
	if metadata.Context != ContextFull && metadata.Context != ContextRecent && metadata.Context != ContextNone {
		return Metadata{}, fmt.Errorf("context %q must be full, recent, or none", metadata.Context)
	}
	seen := make(map[string]struct{}, len(metadata.Tools))
	for i, name := range metadata.Tools {
		name = strings.TrimSpace(name)
		if name == "" {
			return Metadata{}, fmt.Errorf("tools[%d] is empty", i)
		}
		if _, ok := seen[name]; ok {
			return Metadata{}, fmt.Errorf("tools contains duplicate %q", name)
		}
		seen[name] = struct{}{}
		metadata.Tools[i] = name
	}
	return metadata, nil
}

func cloneCatalog(catalog Catalog) Catalog {
	copy := Catalog{Skills: make(map[string]Skill, len(catalog.Skills))}
	for name, skill := range catalog.Skills {
		skill.Tools = append([]string(nil), skill.Tools...)
		copy.Skills[name] = skill
	}
	return copy
}
