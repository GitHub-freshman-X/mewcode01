package tools

import (
	"context"
	"encoding/json"
)

type WriteFileTool struct{ workspace *Workspace }

func NewWriteFileTool(workspace *Workspace) *WriteFileTool {
	return &WriteFileTool{workspace: workspace}
}

func (t *WriteFileTool) Metadata() Metadata {
	return Metadata{
		Name:        "write_file",
		Description: "Write UTF-8 text content to a file inside the current workspace, creating parent directories when needed.",
		Safety:      SafetySideEffect,
		Permission:  PermissionMetadata{Target: PermissionTargetPath, PathParams: []string{"path"}},
		Schema: Schema{"type": "object", "required": []any{"path", "content"}, "properties": map[string]any{
			"path":    map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
		}},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, input json.RawMessage) Result {
	args, err := Validate(t.Metadata().Schema, input)
	if err != nil {
		return Failure(t.Metadata().Name, err.Type, err.Message, err.Details)
	}
	rel, bytes, terr := t.workspace.WriteText(args["path"].(string), args["content"].(string))
	if terr != nil {
		return Failure(t.Metadata().Name, terr.Type, terr.Message, terr.Details)
	}
	return Success(t.Metadata().Name, map[string]any{"path": rel, "bytes_written": bytes})
}
