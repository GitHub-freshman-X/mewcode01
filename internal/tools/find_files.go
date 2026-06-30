package tools

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type FindFilesTool struct{ workspace *Workspace }

func NewFindFilesTool(workspace *Workspace) *FindFilesTool {
	return &FindFilesTool{workspace: workspace}
}

func (t *FindFilesTool) Metadata() Metadata {
	return Metadata{
		Name:        "find_files",
		Description: "Find workspace files matching a glob pattern.",
		Schema: Schema{"type": "object", "required": []any{"pattern"}, "properties": map[string]any{
			"pattern": map[string]any{"type": "string"},
			"limit":   map[string]any{"type": "integer", "minimum": 1.0, "maximum": 1000.0},
		}},
	}
}

func (t *FindFilesTool) Execute(ctx context.Context, input json.RawMessage) Result {
	args, verr := Validate(t.Metadata().Schema, input)
	if verr != nil {
		return Failure(t.Metadata().Name, verr.Type, verr.Message, verr.Details)
	}
	pattern := filepath.ToSlash(args["pattern"].(string))
	if pattern == "" {
		return Failure(t.Metadata().Name, ErrorValidation, "pattern must not be empty", nil)
	}
	limit := intValue(args["limit"], 100)
	var matches []string
	err := filepath.WalkDir(t.workspace.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && path != t.workspace.Root && shouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(t.workspace.Root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		ok, err := globMatch(pattern, rel)
		if err != nil {
			return err
		}
		if ok {
			matches = append(matches, rel)
		}
		return nil
	})
	if err != nil {
		return Failure(t.Metadata().Name, ErrorValidation, "invalid pattern", map[string]any{"cause": err.Error()})
	}
	sort.Strings(matches)
	truncated := false
	if len(matches) > limit {
		matches = matches[:limit]
		truncated = true
	}
	return Success(t.Metadata().Name, map[string]any{"matches": matches, "count": len(matches), "truncated": truncated})
}

func globMatch(pattern, rel string) (bool, error) {
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := strings.TrimSuffix(parts[0], "/")
			suffix := strings.TrimPrefix(parts[1], "/")
			return (prefix == "" || strings.HasPrefix(rel, prefix)) && (suffix == "" || simpleGlob(suffix, rel)), nil
		}
	}
	return filepath.Match(pattern, rel)
}

func simpleGlob(pattern, rel string) bool {
	if ok, _ := filepath.Match(pattern, filepath.Base(rel)); ok {
		return true
	}
	ok, _ := filepath.Match(pattern, rel)
	return ok
}

func intValue(value any, fallback int) int {
	if n, ok := numeric(value); ok {
		return int(n)
	}
	return fallback
}
