package subagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
)

const testDefinition = "---\nname: sample\ndescription: sample role\nmodel: haiku\nmaxTurns: 2\npermissionMode: relaxed\n---\nYou are a sample.\n"

func TestParseDefinition(t *testing.T) {
	definition, err := ParseDefinition("sample.md", testDefinition, SourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Name != "sample" || definition.Model != "haiku" || definition.PermissionMode != permissions.ModeRelaxed {
		t.Fatalf("definition=%+v", definition)
	}
	if _, err := ParseDefinition("bad.md", "no frontmatter", SourceProject); err == nil {
		t.Fatal("expected frontmatter error")
	}
}

func TestDiscoverPrecedence(t *testing.T) {
	root := t.TempDir()
	user, project := filepath.Join(root, "user"), filepath.Join(root, "project")
	for _, item := range []struct{ path, content string }{
		{filepath.Join(user, "role.md"), testDefinition},
		{filepath.Join(project, "role.md"), "---\nname: sample\ndescription: project role\n---\nProject body.\n"},
	} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(item.path, []byte(item.content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	plugin, err := ParseDefinition("plugin.md", "---\nname: sample\ndescription: plugin role\n---\nPlugin body.\n", SourcePlugin)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := Discover(DiscoverOptions{UserDir: user, ProjectDir: project, PluginDefinitions: []Definition{plugin}})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := registry.Get("sample")
	if !ok || got.Source != SourceProject || got.Description != "project role" {
		t.Fatalf("definition=%+v found=%v", got, ok)
	}
}

func TestBuiltinsVerificationSwitch(t *testing.T) {
	without, err := Builtins(false)
	if err != nil {
		t.Fatal(err)
	}
	with, err := Builtins(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(without) != 3 || len(with) != 4 {
		t.Fatalf("builtin counts: %d, %d", len(without), len(with))
	}
}

func TestFilterTools(t *testing.T) {
	known := []string{"agent", "read_file", "write_file", "run_command"}
	filtered, err := FilterTools(known, Definition{Path: "x", Tools: []string{"read_file", "agent"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0] != "read_file" {
		t.Fatalf("filtered=%v", filtered)
	}
	if _, err := FilterTools(known, Definition{Path: "x", Tools: []string{"missing"}}, false); err == nil {
		t.Fatal("expected unknown tool error")
	}
}

func TestTaskManagerCompletionAndNotification(t *testing.T) {
	manager := NewTaskManager()
	updates, cancel := manager.Subscribe()
	defer cancel()
	info, err := manager.Launch(context.Background(), LaunchRequest{Name: "test", Worker: func(_ context.Context, progress func(Progress)) Outcome {
		progress(Progress{ToolCalls: 2})
		return Outcome{Status: TaskCompleted, Result: "done", ToolCalls: 2}
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	finished, err := manager.Wait(ctx, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != TaskCompleted || finished.Result != "done" || finished.ToolCalls != 2 {
		t.Fatalf("task=%+v", finished)
	}
	seenTerminal := false
	for !seenTerminal {
		select {
		case update := <-updates:
			seenTerminal = update.Task.Status == TaskCompleted
		case <-ctx.Done():
			t.Fatal("missing terminal notification")
		}
	}
}
