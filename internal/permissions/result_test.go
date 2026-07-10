package permissions

import (
	"encoding/json"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

func TestDeniedToolResultEncodesPermissionError(t *testing.T) {
	call := provider.ToolCall{ID: "call-1", Name: "write_file", Arguments: []byte(`{"path":"secret.txt"}`)}
	decision := Decision{
		Action:  ActionDeny,
		Stage:   StageRule,
		Reason:  "local rule matched",
		Request: Request{Tool: "write_file", MatchTarget: "secret.txt"},
		Rule:    &Rule{Scope: ScopeLocal, Effect: EffectDeny, Key: "write_file(secret.txt)"},
	}
	result := DeniedToolResult(call, decision)
	if result.CallID != "call-1" || result.Name != "write_file" || !result.IsError {
		t.Fatalf("unexpected provider result: %#v", result)
	}
	var payload tools.Result
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("content is not tools.Result JSON: %v\n%s", err, result.Content)
	}
	if payload.Success || payload.Error == nil || payload.Error.Type != tools.ErrorPermission {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload.Error.Details["stage"] != string(StageRule) || payload.Error.Details["scope"] != string(ScopeLocal) {
		t.Fatalf("unexpected details: %#v", payload.Error.Details)
	}
}
