package status

import "time"

// BackgroundTask contains only safe metadata for a running background task.
type BackgroundTask struct {
	ID      string
	Name    string
	Status  string
	Elapsed time.Duration
}

// Snapshot is the safe, local runtime data rendered by /status.
type Snapshot struct {
	Workspace               string
	LogDirectory            string
	PermissionMode          string
	ToolCount               int
	SkillCount              int
	UserMemoryCount         int
	ProjectMemoryCount      int
	MemoryAvailable         bool
	SubAgentDefinitionCount int
	BackgroundTasks         []BackgroundTask
}

// Provider supplies one safe runtime snapshot for local status rendering.
type Provider interface {
	StatusSnapshot() Snapshot
}
