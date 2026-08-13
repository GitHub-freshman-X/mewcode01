package instructions

import "path/filepath"

type Paths struct {
	UserRoot      string
	WorkspaceRoot string
	UserMemory    string
	ProjectState  string
	Sessions      string
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
		Sessions:      filepath.Join(projectState, "sessions"),
		ProjectMemory: filepath.Join(projectState, "memory"),
	}
}
