package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeSubAgentHost struct{ input AgentInput }

func (f *fakeSubAgentHost) DispatchSubAgent(_ context.Context, input AgentInput) (Result, error) {
	f.input = input
	return Success("agent", map[string]any{"task_id": "subagent-1"}), nil
}

func TestAgentToolDispatchesStableSchema(t *testing.T) {
	tool := NewAgentTool()
	meta := tool.Metadata()
	if meta.Name != "agent" || meta.Safety != SafetySideEffect {
		t.Fatalf("metadata=%+v", meta)
	}
	properties := meta.Schema["properties"].(map[string]any)
	for _, name := range []string{"prompt", "description", "subagent_type", "model", "run_in_background", "name", "isolation"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("missing property %s", name)
		}
	}
	forkDescription := properties["subagent_type"].(map[string]any)["description"].(string)
	if !strings.Contains(forkDescription, "fork") || !strings.Contains(forkDescription, "Omit") {
		t.Fatalf("subagent_type description does not document fork compatibility: %q", forkDescription)
	}
	host := &fakeSubAgentHost{}
	input, _ := json.Marshal(map[string]any{"prompt": "inspect", "description": "inspect files", "subagent_type": "Explore"})
	result := tool.Execute(WithSubAgentHost(context.Background(), host), input)
	if !result.Success || host.input.SubagentType != "Explore" {
		t.Fatalf("result=%+v input=%+v", result, host.input)
	}
}

func TestAgentToolRejectsUnsupportedIsolation(t *testing.T) {
	result := NewAgentTool().Execute(context.Background(), []byte(`{"prompt":"x","description":"y","isolation":"worktree"}`))
	if result.Success || result.Error == nil || result.Error.Type != ErrorValidation {
		t.Fatalf("result=%+v", result)
	}
}
