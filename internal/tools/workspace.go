package tools

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	defaultReadLimit   = 512 * 1024
	defaultWriteLimit  = 512 * 1024
	defaultOutputLimit = 64 * 1024
)

type Workspace struct {
	Root        string
	ReadLimit   int
	WriteLimit  int
	OutputLimit int
}

func NewWorkspace(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Workspace{Root: filepath.Clean(abs), ReadLimit: defaultReadLimit, WriteLimit: defaultWriteLimit, OutputLimit: defaultOutputLimit}, nil
}

func (w *Workspace) Resolve(path string) (string, string, *ToolError) {
	if strings.TrimSpace(path) == "" {
		return "", "", &ToolError{Type: ErrorValidation, Message: "path is required"}
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(w.Root, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", &ToolError{Type: ErrorValidation, Message: "path is invalid", Details: map[string]any{"cause": err.Error()}}
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(w.Root, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", "", &ToolError{Type: ErrorPermission, Message: "path escapes workspace"}
	}
	return abs, filepath.ToSlash(rel), nil
}

func (w *Workspace) ReadText(path string) (string, string, int, bool, *ToolError) {
	abs, rel, terr := w.Resolve(path)
	if terr != nil {
		return "", "", 0, false, terr
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", rel, 0, false, &ToolError{Type: ErrorNotFound, Message: "file does not exist", Details: map[string]any{"path": rel}}
		}
		return "", rel, 0, false, &ToolError{Type: ErrorExecution, Message: "failed to stat file", Details: map[string]any{"path": rel, "cause": err.Error()}}
	}
	if info.IsDir() {
		return "", rel, 0, false, &ToolError{Type: ErrorValidation, Message: "path is a directory", Details: map[string]any{"path": rel}}
	}
	if info.Size() > int64(w.ReadLimit) {
		return "", rel, 0, false, &ToolError{Type: ErrorValidation, Message: "file is too large", Details: map[string]any{"path": rel, "limit": w.ReadLimit}}
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", rel, 0, false, &ToolError{Type: ErrorExecution, Message: "failed to read file", Details: map[string]any{"path": rel, "cause": err.Error()}}
	}
	if isBinary(data) || !utf8.Valid(data) {
		return "", rel, 0, false, &ToolError{Type: ErrorValidation, Message: "file is not valid text", Details: map[string]any{"path": rel}}
	}
	content, truncated := TruncateString(string(data), w.OutputLimit)
	return rel, content, len(data), truncated, nil
}

func (w *Workspace) WriteText(path string, content string) (string, int, *ToolError) {
	if len([]byte(content)) > w.WriteLimit {
		return "", 0, &ToolError{Type: ErrorValidation, Message: "content is too large", Details: map[string]any{"limit": w.WriteLimit}}
	}
	if !utf8.ValidString(content) {
		return "", 0, &ToolError{Type: ErrorValidation, Message: "content must be valid UTF-8 text"}
	}
	abs, rel, terr := w.Resolve(path)
	if terr != nil {
		return "", 0, terr
	}
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return "", 0, &ToolError{Type: ErrorValidation, Message: "path is a directory", Details: map[string]any{"path": rel}}
	}
	parent := filepath.Dir(abs)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return "", 0, &ToolError{Type: ErrorNotFound, Message: "parent directory does not exist", Details: map[string]any{"path": filepath.ToSlash(parent)}}
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", 0, &ToolError{Type: ErrorExecution, Message: "failed to write file", Details: map[string]any{"path": rel, "cause": err.Error()}}
	}
	return rel, len([]byte(content)), nil
}

func TruncateString(value string, limit int) (string, bool) {
	if limit <= 0 || len([]byte(value)) <= limit {
		return value, false
	}
	data := []byte(value)
	cut := limit
	for cut > 0 && !utf8.Valid(data[:cut]) {
		cut--
	}
	return string(data[:cut]) + "\n[truncated]", true
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	return false
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".idea", ".vscode", "dist", "build":
		return true
	default:
		return false
	}
}
