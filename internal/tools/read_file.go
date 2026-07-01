package tools

import (
	"context"
	"encoding/json"
)

type ReadFileTool struct{ workspace *Workspace }

func NewReadFileTool(workspace *Workspace) *ReadFileTool { return &ReadFileTool{workspace: workspace} }

func (t *ReadFileTool) Metadata() Metadata {
	return Metadata{
		Name:        "read_file",
		Description: "Read a UTF-8 text file from the current workspace.",
		Safety:      SafetyReadOnly,
		Schema: Schema{"type": "object", "required": []any{"path"}, "properties": map[string]any{
			"path": map[string]any{"type": "string"},
		}},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, input json.RawMessage) Result {
	args, err := Validate(t.Metadata().Schema, input)
	if err != nil {
		return Failure(t.Metadata().Name, err.Type, err.Message, err.Details)
	}
	path := args["path"].(string)
	rel, content, bytes, truncated, terr := t.workspace.ReadText(path)
	if terr != nil {
		return Failure(t.Metadata().Name, terr.Type, terr.Message, terr.Details)
	}
	return Success(t.Metadata().Name, map[string]any{"path": rel, "bytes": bytes, "content": content, "truncated": truncated})
}
