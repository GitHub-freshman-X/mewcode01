package status

import "testing"

func TestSnapshotContainsOnlySafeStatusFields(t *testing.T) {
	snapshot := Snapshot{Workspace: "/fixture", ToolCount: 3, BackgroundTasks: []BackgroundTask{{ID: "subagent-1", Name: "verify", Status: "running"}}}
	if snapshot.Workspace != "/fixture" || snapshot.ToolCount != 3 || len(snapshot.BackgroundTasks) != 1 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}
