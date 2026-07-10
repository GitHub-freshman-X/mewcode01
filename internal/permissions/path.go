package permissions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrPathEscapesSandbox = errors.New("path escapes workspace sandbox")

type Sandbox struct {
	Root     string
	RealRoot string
}

func NewSandbox(root string) (Sandbox, error) {
	if strings.TrimSpace(root) == "" {
		return Sandbox{}, errors.New("workspace root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Sandbox{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Sandbox{}, fmt.Errorf("resolve real workspace root: %w", err)
	}
	return Sandbox{Root: filepath.Clean(abs), RealRoot: filepath.Clean(realRoot)}, nil
}

func (s Sandbox) Resolve(raw string, parameter string) (PathCheck, error) {
	if strings.TrimSpace(raw) == "" {
		return PathCheck{}, errors.New("path is required")
	}
	candidate := raw
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(s.Root, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return PathCheck{}, fmt.Errorf("resolve path %q: %w", parameter, err)
	}
	real, err := resolveExistingPrefix(filepath.Clean(abs))
	if err != nil {
		return PathCheck{}, err
	}
	rel, err := filepath.Rel(s.RealRoot, real)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return PathCheck{}, ErrPathEscapesSandbox
	}
	return PathCheck{
		Raw:       raw,
		Real:      real,
		Relative:  filepath.ToSlash(rel),
		Parameter: parameter,
	}, nil
}

func resolveExistingPrefix(abs string) (string, error) {
	if _, err := os.Lstat(abs); err == nil {
		return filepath.EvalSymlinks(abs)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect path: %w", err)
	}
	var missing []string
	cursor := abs
	for {
		if cursor == "." || cursor == string(filepath.Separator) || cursor == filepath.Dir(cursor) {
			break
		}
		if _, err := os.Lstat(cursor); err == nil {
			real, err := filepath.EvalSymlinks(cursor)
			if err != nil {
				return "", fmt.Errorf("resolve path symlink: %w", err)
			}
			for i := len(missing) - 1; i >= 0; i-- {
				real = filepath.Join(real, missing[i])
			}
			return filepath.Clean(real), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect path prefix: %w", err)
		}
		missing = append(missing, filepath.Base(cursor))
		cursor = filepath.Dir(cursor)
	}
	return filepath.Clean(abs), nil
}
