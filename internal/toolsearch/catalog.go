package toolsearch

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

const SearchToolName = "tool_search"
const CallToolName = "mcp_call"

type Catalog struct{ tools []provider.ToolDefinition }

func New(defs []provider.ToolDefinition) Catalog {
	tools := make([]provider.ToolDefinition, 0)
	for _, def := range defs {
		if def.MCPServer != "" {
			tools = append(tools, def)
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return Catalog{tools: tools}
}

func (c Catalog) Visible(defs []provider.ToolDefinition) []provider.ToolDefinition {
	out := make([]provider.ToolDefinition, 0, len(defs)+2)
	for _, def := range defs {
		if def.MCPServer == "" {
			out = append(out, def)
		}
	}
	out = append(out,
		provider.ToolDefinition{Name: SearchToolName, Description: "Load an MCP tool schema by its exact name from the MCP tool directory. tool_name must exactly equal one listed name; do not pass keywords, natural-language requests, or a select: prefix.", Schema: map[string]any{"type": "object", "properties": map[string]any{"tool_name": map[string]any{"type": "string"}}, "required": []string{"tool_name"}, "additionalProperties": false}},
		provider.ToolDefinition{Name: CallToolName, Description: "Call an MCP tool returned by tool_search.", Schema: map[string]any{"type": "object", "properties": map[string]any{"server": map[string]any{"type": "string"}, "tool": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object"}}, "required": []string{"server", "tool", "arguments"}, "additionalProperties": false}},
	)
	return out
}

func (c Catalog) Names() []string {
	out := make([]string, len(c.tools))
	for i, t := range c.tools {
		out[i] = t.Name
	}
	return out
}
func (c Catalog) Load(name string) (provider.ToolDefinition, error) {
	name = strings.TrimSpace(name)
	for _, tool := range c.tools {
		if tool.Name == name {
			return tool, nil
		}
	}
	return provider.ToolDefinition{}, fmt.Errorf("MCP tool %q is not in the tool directory", name)
}
func (c Catalog) Resolve(server, name string) (provider.ToolDefinition, error) {
	for _, t := range c.tools {
		if t.MCPServer == server && (t.Name == name || t.RemoteName == name) {
			return t, nil
		}
	}
	return provider.ToolDefinition{}, fmt.Errorf("MCP tool %q on server %q was not found", name, server)
}
