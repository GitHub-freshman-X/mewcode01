package instructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaths(t *testing.T) {
	t.Parallel()

	paths := NewPaths(filepath.Join("/tmp", "config"), filepath.Join("/tmp", "workspace"))

	if got, want := paths.UserRoot, filepath.Join("/tmp", "config", "mewcode"); got != want {
		t.Fatalf("UserRoot = %q, want %q", got, want)
	}
	if got, want := paths.WorkspaceRoot, filepath.Join("/tmp", "workspace"); got != want {
		t.Fatalf("WorkspaceRoot = %q, want %q", got, want)
	}
	if got, want := paths.UserMemory, filepath.Join("/tmp", "config", "mewcode", "memory"); got != want {
		t.Fatalf("UserMemory = %q, want %q", got, want)
	}
	if got, want := paths.ProjectState, filepath.Join("/tmp", "workspace", ".mewcode"); got != want {
		t.Fatalf("ProjectState = %q, want %q", got, want)
	}
	if got, want := paths.Sessions, filepath.Join("/tmp", "workspace", ".mewcode", "sessions"); got != want {
		t.Fatalf("Sessions = %q, want %q", got, want)
	}
	if got, want := paths.ProjectMemory, filepath.Join("/tmp", "workspace", ".mewcode", "memory"); got != want {
		t.Fatalf("ProjectMemory = %q, want %q", got, want)
	}
}

func TestLoadInstructionsCombinesUserAndProjectInstructions(t *testing.T) {
	paths := testPaths(t)
	writeInstruction(t, filepath.Join(paths.UserRoot, "MEWCODE.md"), "user instructions")
	writeInstruction(t, filepath.Join(paths.ProjectState, "MEWCODE.md"), "project instructions")

	got, err := LoadInstructions(paths)
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if want := "user instructions\n\n---\n\nproject instructions"; got != want {
		t.Fatalf("LoadInstructions() = %q, want %q", got, want)
	}
}

func TestLoadInstructionsMissingRootFilesAreEmpty(t *testing.T) {
	got, err := LoadInstructions(testPaths(t))
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if got != "" {
		t.Fatalf("LoadInstructions() = %q, want empty", got)
	}
}

func TestLoadInstructionsExpandsProjectIncludesRelativeToIncludingFile(t *testing.T) {
	paths := testPaths(t)
	writeInstruction(t, filepath.Join(paths.ProjectState, "MEWCODE.md"), "root\n@rules/first.md")
	writeInstruction(t, filepath.Join(paths.ProjectState, "rules", "first.md"), "first\n@nested/second.md")
	writeInstruction(t, filepath.Join(paths.ProjectState, "rules", "nested", "second.md"), "second")

	got, err := LoadInstructions(paths)
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if want := "root\nfirst\nsecond"; got != want {
		t.Fatalf("LoadInstructions() = %q, want %q", got, want)
	}
}

func TestLoadInstructionsExpandsEachIncludeOnce(t *testing.T) {
	paths := testPaths(t)
	writeInstruction(t, filepath.Join(paths.ProjectState, "MEWCODE.md"), "root\n@one.md")
	writeInstruction(t, filepath.Join(paths.ProjectState, "one.md"), "one\n@two.md")
	writeInstruction(t, filepath.Join(paths.ProjectState, "two.md"), "two\n@one.md")

	got, err := LoadInstructions(paths)
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if want := "root\none\ntwo"; got != want {
		t.Fatalf("LoadInstructions() = %q, want %q", got, want)
	}
}

func TestLoadInstructionsStopsAfterFiveIncludeLevels(t *testing.T) {
	paths := testPaths(t)
	writeInstruction(t, filepath.Join(paths.ProjectState, "MEWCODE.md"), "root\n@one.md")
	for i, name := range []string{"one", "two", "three", "four", "five"} {
		content := name
		if i < 4 {
			content += "\n@" + []string{"two", "three", "four", "five"}[i] + ".md"
		} else {
			content += "\n@six.md"
		}
		writeInstruction(t, filepath.Join(paths.ProjectState, name+".md"), content)
	}
	writeInstruction(t, filepath.Join(paths.ProjectState, "six.md"), "six")

	got, err := LoadInstructions(paths)
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if !strings.Contains(got, "root\none\ntwo\nthree\nfour\nfive") {
		t.Fatalf("LoadInstructions() = %q, missing included content through level five", got)
	}
	if strings.Contains(got, "six") {
		t.Fatalf("LoadInstructions() = %q, included content beyond level five", got)
	}
	if !strings.Contains(got, "<!--") {
		t.Fatalf("LoadInstructions() = %q, want a safe depth-limit comment", got)
	}
}

func TestLoadInstructionsBlocksIncludesOutsideProjectRoot(t *testing.T) {
	paths := testPaths(t)
	outside := filepath.Join(filepath.Dir(paths.WorkspaceRoot), "outside.md")
	writeInstruction(t, outside, "secret")
	writeInstruction(t, filepath.Join(paths.ProjectState, "MEWCODE.md"), "project\n@../../outside.md")

	got, err := LoadInstructions(paths)
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("LoadInstructions() = %q, included content outside the project root", got)
	}
	if !strings.Contains(got, "<!--") {
		t.Fatalf("LoadInstructions() = %q, want a safe blocked-include comment", got)
	}
}

func TestLoadInstructionsRejectsProjectRootInstructionSymlinkOutsideWorkspace(t *testing.T) {
	paths := testPaths(t)
	outside := filepath.Join(filepath.Dir(paths.WorkspaceRoot), "outside.md")
	writeInstruction(t, outside, "secret")
	if err := os.MkdirAll(paths.ProjectState, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(paths.ProjectState, "MEWCODE.md")); err != nil {
		t.Fatal(err)
	}

	got, err := LoadInstructions(paths)
	if err == nil {
		t.Fatal("LoadInstructions() error = nil, want error")
	}
	if got != "" {
		t.Fatalf("LoadInstructions() = %q, want no instruction content", got)
	}
}

func TestLoadInstructionsMarksMissingIncludeAndContinues(t *testing.T) {
	paths := testPaths(t)
	writeInstruction(t, filepath.Join(paths.ProjectState, "MEWCODE.md"), "before\n@missing.md\nafter")

	got, err := LoadInstructions(paths)
	if err != nil {
		t.Fatalf("LoadInstructions() error = %v", err)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") || !strings.Contains(got, "<!--") {
		t.Fatalf("LoadInstructions() = %q, want surrounding content and a safe missing-include comment", got)
	}
}

func TestLoadInstructionsReturnsErrorWhenExistingRootFileCannotBeRead(t *testing.T) {
	paths := testPaths(t)
	rootInstruction := filepath.Join(paths.ProjectState, "MEWCODE.md")
	if err := os.MkdirAll(rootInstruction, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", rootInstruction, err)
	}

	got, err := LoadInstructions(paths)
	if err == nil {
		t.Fatal("LoadInstructions() error = nil, want error")
	}
	if got != "" {
		t.Fatalf("LoadInstructions() = %q, want no instruction content on root read error", got)
	}
}

func testPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	return NewPaths(filepath.Join(root, "config"), filepath.Join(root, "workspace"))
}

func writeInstruction(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
