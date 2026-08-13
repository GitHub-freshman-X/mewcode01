package instructions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const maxIncludeDepth = 5

func LoadInstructions(paths Paths) (string, error) {
	userInstructions, err := readRootInstruction(filepath.Join(paths.UserRoot, "MEWCODE.md"))
	if err != nil {
		return "", errors.New("cannot read user instructions")
	}

	projectPath := filepath.Join(paths.ProjectState, "MEWCODE.md")
	projectInstructions, err := readRootInstruction(projectPath)
	if err != nil {
		return "", errors.New("cannot read project instructions")
	}
	if projectInstructions != "" {
		projectInstructions, err = expandProjectInstructions(projectPath, projectInstructions, paths.WorkspaceRoot)
		if err != nil {
			return "", errors.New("cannot load project instructions")
		}
	}

	return joinInstructions(userInstructions, projectInstructions), nil
}

func readRootInstruction(path string) (string, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func expandProjectInstructions(projectPath, content, workspaceRoot string) (string, error) {
	projectRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	projectRoot, err = filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return "", err
	}

	rootInstruction, err := filepath.Abs(projectPath)
	if err != nil {
		return "", err
	}
	rootInstruction, err = filepath.EvalSymlinks(rootInstruction)
	if err != nil {
		return "", err
	}
	if !within(projectRoot, rootInstruction) {
		return "", errors.New("project instruction is outside workspace")
	}

	visited := map[string]struct{}{rootInstruction: {}}
	return expandContent(content, rootInstruction, projectRoot, visited, 0), nil
}

func expandContent(content, currentPath, projectRoot string, visited map[string]struct{}, depth int) string {
	lines := strings.Split(content, "\n")
	expanded := make([]string, 0, len(lines))
	for _, line := range lines {
		includePath, ok := includePath(line)
		if !ok {
			expanded = append(expanded, line)
			continue
		}

		if depth >= maxIncludeDepth {
			expanded = append(expanded, "<!-- mewcode include depth limit reached -->")
			continue
		}

		included, status := expandInclude(includePath, currentPath, projectRoot, visited, depth+1)
		switch status {
		case includeExpanded:
			expanded = append(expanded, included)
		case includeAlreadyVisited:
			continue
		case includeBlocked:
			expanded = append(expanded, "<!-- mewcode include blocked -->")
		default:
			expanded = append(expanded, "<!-- mewcode include unavailable -->")
		}
	}
	return strings.Join(expanded, "\n")
}

type includeStatus int

const (
	includeExpanded includeStatus = iota
	includeAlreadyVisited
	includeBlocked
	includeUnavailable
)

func expandInclude(reference, currentPath, projectRoot string, visited map[string]struct{}, depth int) (string, includeStatus) {
	if filepath.IsAbs(reference) {
		return "", includeBlocked
	}

	candidate, err := filepath.Abs(filepath.Join(filepath.Dir(currentPath), reference))
	if err != nil || !within(projectRoot, candidate) {
		return "", includeBlocked
	}
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", includeUnavailable
	}
	if !within(projectRoot, candidate) {
		return "", includeBlocked
	}
	if _, ok := visited[candidate]; ok {
		return "", includeAlreadyVisited
	}

	content, err := os.ReadFile(candidate)
	if err != nil {
		return "", includeUnavailable
	}
	visited[candidate] = struct{}{}
	return expandContent(string(content), candidate, projectRoot, visited, depth), includeExpanded
}

func includePath(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "@") || len(trimmed) == 1 {
		return "", false
	}
	return strings.TrimSpace(trimmed[1:]), true
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func joinInstructions(userInstructions, projectInstructions string) string {
	parts := make([]string, 0, 2)
	if userInstructions != "" {
		parts = append(parts, userInstructions)
	}
	if projectInstructions != "" {
		parts = append(parts, projectInstructions)
	}
	return strings.Join(parts, "\n\n---\n\n")
}
