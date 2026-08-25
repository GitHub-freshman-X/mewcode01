package tools

import (
	"errors"
	"fmt"
	"sort"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func NewDefaultRegistry(root string) (*Registry, error) {
	workspace, err := NewWorkspace(root)
	if err != nil {
		return nil, err
	}
	return NewDefaultRegistryForWorkspace(workspace)
}

// NewDefaultRegistryForWorkspace creates fresh tool instances bound to the
// supplied workspace. It is used for worktrees so no tool retains the main
// workspace as an implicit current directory.
func NewDefaultRegistryForWorkspace(workspace *Workspace) (*Registry, error) {
	if workspace == nil {
		return nil, errors.New("workspace is nil")
	}
	r := NewRegistry()
	for _, tool := range []Tool{
		NewReadFileTool(workspace),
		NewWriteFileTool(workspace),
		NewEditFileTool(workspace),
		NewRunCommandTool(workspace),
		NewFindFilesTool(workspace),
		NewSearchCodeTool(workspace),
	} {
		if err := r.Register(tool); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// RebindWorkspace returns a registry whose local filesystem and command tools
// use workspace. Tools that are not workspace-bound (for example MCP tools)
// are preserved from the original registry.
func (r *Registry) RebindWorkspace(workspace *Workspace) (*Registry, error) {
	bound, err := NewDefaultRegistryForWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	for name, tool := range r.tools {
		if _, local := bound.tools[name]; local {
			continue
		}
		bound.tools[name] = tool
	}
	return bound, nil
}

func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return errors.New("tool is nil")
	}
	meta := tool.Metadata()
	if meta.Name == "" {
		return errors.New("tool name is empty")
	}
	if meta.Description == "" {
		return fmt.Errorf("tool %q description is empty", meta.Name)
	}
	if typ, _ := meta.Schema["type"].(string); typ != "object" {
		return fmt.Errorf("tool %q schema must be an object", meta.Name)
	}
	if _, exists := r.tools[meta.Name]; exists {
		return fmt.Errorf("tool %q is already registered", meta.Name)
	}
	r.tools[meta.Name] = tool
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	if r == nil {
		return nil, false
	}
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Subset returns a registry containing allowed names. An empty allowed list
// preserves all tools; required names are always retained.
func (r *Registry) Subset(allowed, required []string) (*Registry, error) {
	if r == nil {
		return nil, errors.New("tool registry is nil")
	}
	selected := make(map[string]struct{}, len(allowed)+len(required))
	if len(allowed) == 0 {
		for name := range r.tools {
			selected[name] = struct{}{}
		}
	} else {
		for _, name := range allowed {
			selected[name] = struct{}{}
		}
	}
	for _, name := range required {
		selected[name] = struct{}{}
	}
	filtered := NewRegistry()
	for name := range selected {
		tool, ok := r.tools[name]
		if !ok {
			return nil, fmt.Errorf("tool %q is not registered", name)
		}
		filtered.tools[name] = tool
	}
	return filtered, nil
}

func (r *Registry) FilterBySafety(safety Safety) *Registry {
	filtered := NewRegistry()
	want := NormalizeSafety(safety)
	if r == nil {
		return filtered
	}
	for name, tool := range r.tools {
		if NormalizeSafety(tool.Metadata().Safety) == want {
			filtered.tools[name] = tool
		}
	}
	return filtered
}

func (r *Registry) List() []Tool {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}

func (r *Registry) Definitions() []provider.ToolDefinition {
	tools := r.List()
	defs := make([]provider.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		meta := tool.Metadata()
		defs = append(defs, provider.ToolDefinition{Name: meta.Name, Description: meta.Description, Schema: map[string]any(meta.Schema)})
	}
	return defs
}
