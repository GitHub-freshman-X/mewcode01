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
	tool, ok := r.tools[name]
	return tool, ok
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
