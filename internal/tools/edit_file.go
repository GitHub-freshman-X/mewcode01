package tools

import (
	"context"
	"encoding/json"
	"strings"
)

type EditFileTool struct{ workspace *Workspace }

func NewEditFileTool(workspace *Workspace) *EditFileTool { return &EditFileTool{workspace: workspace} }

func (t *EditFileTool) Metadata() Metadata {
	return Metadata{
		Name:        "edit_file",
		Description: "Replace one exact text occurrence in a workspace file.",
		Safety:      SafetySideEffect,
		Schema: Schema{"type": "object", "required": []any{"path", "old_text", "new_text"}, "properties": map[string]any{
			"path":     map[string]any{"type": "string"},
			"old_text": map[string]any{"type": "string"},
			"new_text": map[string]any{"type": "string"},
		}},
	}
}

func (t *EditFileTool) Execute(ctx context.Context, input json.RawMessage) Result {
	args, err := Validate(t.Metadata().Schema, input)
	if err != nil {
		return Failure(t.Metadata().Name, err.Type, err.Message, err.Details)
	}
	path := args["path"].(string)
	oldText := args["old_text"].(string)
	newText := args["new_text"].(string)
	if oldText == "" {
		return Failure(t.Metadata().Name, ErrorValidation, "old_text must not be empty", nil)
	}
	rel, content, _, _, terr := t.workspace.ReadText(path)
	if terr != nil {
		return Failure(t.Metadata().Name, terr.Type, terr.Message, terr.Details)
	}
	count := strings.Count(content, oldText)
	if count == 0 {
		return Failure(t.Metadata().Name, ErrorNotFound, "old_text was not found", map[string]any{"path": rel, "matches": count})
	}
	if count > 1 {
		return Failure(t.Metadata().Name, ErrorConflict, "old_text must match exactly once", map[string]any{"path": rel, "matches": count})
	}
	updated := strings.Replace(content, oldText, newText, 1)
	rel, bytes, terr := t.workspace.WriteText(path, updated)
	if terr != nil {
		return Failure(t.Metadata().Name, terr.Type, terr.Message, terr.Details)
	}
	return Success(t.Metadata().Name, map[string]any{"path": rel, "replacements": 1, "bytes_written": bytes})
}
