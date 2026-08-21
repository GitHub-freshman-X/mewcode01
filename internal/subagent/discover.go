package subagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type DiscoverOptions struct {
	ProjectDir              string
	UserDir                 string
	PluginDefinitions       []Definition
	EnableVerificationAgent bool
}

func Discover(options DiscoverOptions) (*Registry, error) {
	builtins, err := Builtins(options.EnableVerificationAgent)
	if err != nil {
		return nil, err
	}
	registry, err := NewRegistry(nil)
	if err != nil {
		return nil, err
	}
	for _, definitions := range [][]Definition{options.PluginDefinitions, builtins} {
		if err := merge(registry, definitions, false); err != nil {
			return nil, err
		}
	}
	for _, source := range []struct {
		root   string
		source Source
	}{{options.UserDir, SourceUser}, {options.ProjectDir, SourceProject}} {
		definitions, err := discoverDirectory(source.root, source.source)
		if err != nil {
			return nil, err
		}
		if err := merge(registry, definitions, true); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func merge(registry *Registry, definitions []Definition, allowOverride bool) error {
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if err := ValidateDefinition(definition); err != nil {
			return err
		}
		if _, duplicate := seen[definition.Name]; duplicate {
			return fmt.Errorf("agent definition %s: duplicate name %q", definition.Path, definition.Name)
		}
		seen[definition.Name] = struct{}{}
		if _, exists := registry.Get(definition.Name); exists && !allowOverride {
			return fmt.Errorf("agent definition %s: duplicate name %q", definition.Path, definition.Name)
		}
		if err := registry.Add(definition); err != nil {
			return err
		}
	}
	return nil
}

func discoverDirectory(root string, source Source) ([]Definition, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan agent directory %s: %w", root, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read agent definition %s: %w", path, err)
		}
		definition, err := ParseDefinition(path, string(data), source)
		if err != nil {
			return nil, err
		}
		out = append(out, definition)
	}
	return out, nil
}
