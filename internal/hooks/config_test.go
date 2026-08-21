package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRuleSetAppendsAllSources(t *testing.T) {
	dir := t.TempDir()
	paths := FilePaths{User: filepath.Join(dir, "user.yaml"), Project: filepath.Join(dir, "project.yaml")}
	for path, id := range map[string]string{paths.User: "user", paths.Project: "project"} {
		if err := os.WriteFile(path, []byte("hooks:\n  - id: "+id+"\n    event: session_start\n    action:\n      type: prompt\n      message: hello\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rules, err := LoadRuleSet(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].ID != "user" || rules[1].ID != "project" {
		t.Fatalf("rules=%+v", rules)
	}
}

func TestDefaultFilePaths(t *testing.T) {
	workspace := t.TempDir()
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := DefaultFilePaths(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if paths.User != filepath.Join(configDir, "mewcode", "config.yaml") {
		t.Fatalf("user path=%q", paths.User)
	}
	if paths.Project != filepath.Join(workspace, ".mewcode", "config.yaml") {
		t.Fatalf("project path=%q", paths.Project)
	}
}

func TestLoadRuleSetRejectsInvalidRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "hooks:\n  - id: bad\n    event: turn_start\n    reject: true\n    action:\n      type: prompt\n      message: x\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRuleSet(FilePaths{User: path})
	if err == nil || !strings.Contains(err.Error(), "bad") || !strings.Contains(err.Error(), "reject") {
		t.Fatalf("err=%v", err)
	}
}
