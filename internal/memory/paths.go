package memory

import (
	"errors"
	"path/filepath"
	"strings"
)

type MemoryKind string

const (
	MemoryUser      MemoryKind = "user"
	MemoryFeedback  MemoryKind = "feedback"
	MemoryProject   MemoryKind = "project"
	MemoryReference MemoryKind = "reference"
)

type Paths struct {
	UserRoot      string
	WorkspaceRoot string
	UserMemory    string
	ProjectState  string
	ProjectMemory string
}

func NewPaths(userConfigRoot, workspaceRoot string) Paths {
	userRoot := filepath.Join(userConfigRoot, "mewcode")
	projectState := filepath.Join(workspaceRoot, ".mewcode")
	return Paths{
		UserRoot:      userRoot,
		WorkspaceRoot: workspaceRoot,
		UserMemory:    filepath.Join(userRoot, "memory"),
		ProjectState:  projectState,
		ProjectMemory: filepath.Join(projectState, "memory"),
	}
}

func MemoryDirectory(paths Paths, kind MemoryKind) (string, error) {
	var directory, boundary string
	switch kind {
	case MemoryUser, MemoryFeedback:
		directory, boundary = paths.UserMemory, paths.UserRoot
	case MemoryProject, MemoryReference:
		directory, boundary = paths.ProjectMemory, paths.ProjectState
	default:
		return "", errors.New("unknown memory kind")
	}
	if !contained(boundary, directory) {
		return "", errors.New("memory directory is outside its storage boundary")
	}
	return directory, nil
}

func MemoryFilePath(paths Paths, kind MemoryKind, name string) (string, error) {
	if !validSlug(name) {
		return "", errors.New("invalid memory name")
	}
	directory, err := MemoryDirectory(paths, kind)
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, name+".md")
	if !contained(directory, path) {
		return "", errors.New("memory file is outside its directory")
	}
	return path, nil
}

func memoryIndexPath(paths Paths, kind MemoryKind) (string, error) {
	directory, err := MemoryDirectory(paths, kind)
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, "MEMORY.md")
	if !contained(directory, path) {
		return "", errors.New("memory index is outside its directory")
	}
	return path, nil
}

func validSlug(value string) bool {
	if value == "" || len(value) > 128 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func contained(root, target string) bool {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(target) == "" {
		return false
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetPath, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootPath, targetPath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
