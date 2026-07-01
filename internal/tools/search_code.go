package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type SearchCodeTool struct{ workspace *Workspace }

func NewSearchCodeTool(workspace *Workspace) *SearchCodeTool {
	return &SearchCodeTool{workspace: workspace}
}

func (t *SearchCodeTool) Metadata() Metadata {
	return Metadata{
		Name:        "search_code",
		Description: "Search text files in the workspace by literal text or regular expression.",
		Safety:      SafetyReadOnly,
		Schema: Schema{"type": "object", "required": []any{"pattern"}, "properties": map[string]any{
			"pattern": map[string]any{"type": "string"},
			"regex":   map[string]any{"type": "boolean"},
			"limit":   map[string]any{"type": "integer", "minimum": 1.0, "maximum": 1000.0},
		}},
	}
}

func (t *SearchCodeTool) Execute(ctx context.Context, input json.RawMessage) Result {
	args, verr := Validate(t.Metadata().Schema, input)
	if verr != nil {
		return Failure(t.Metadata().Name, verr.Type, verr.Message, verr.Details)
	}
	pattern := args["pattern"].(string)
	if pattern == "" {
		return Failure(t.Metadata().Name, ErrorValidation, "pattern must not be empty", nil)
	}
	useRegex, _ := args["regex"].(bool)
	limit := intValue(args["limit"], 100)
	var re *regexp.Regexp
	if useRegex {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return Failure(t.Metadata().Name, ErrorValidation, "invalid regex pattern", map[string]any{"cause": err.Error()})
		}
	}
	var matches []map[string]any
	err := filepath.WalkDir(t.workspace.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || len(matches) >= limit+1 {
			return nil
		}
		if d.IsDir() && path != t.workspace.Root && shouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || isBinary(data) {
			return nil
		}
		rel, err := filepath.Rel(t.workspace.Root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			col := -1
			if useRegex {
				loc := re.FindStringIndex(line)
				if loc != nil {
					col = loc[0] + 1
				}
			} else if idx := strings.Index(line, pattern); idx >= 0 {
				col = idx + 1
			}
			if col > 0 {
				snippet, truncated := TruncateString(line, 240)
				matches = append(matches, map[string]any{"file": rel, "line": lineNo, "column": col, "snippet": snippet, "snippet_truncated": truncated})
				if len(matches) >= limit+1 {
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return Failure(t.Metadata().Name, ErrorExecution, "search failed", map[string]any{"cause": err.Error()})
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a["file"] != b["file"] {
			return a["file"].(string) < b["file"].(string)
		}
		return a["line"].(int) < b["line"].(int)
	})
	truncated := false
	if len(matches) > limit {
		matches = matches[:limit]
		truncated = true
	}
	return Success(t.Metadata().Name, map[string]any{"matches": matches, "count": len(matches), "truncated": truncated})
}
