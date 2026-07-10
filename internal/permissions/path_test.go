package permissions

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSandboxResolveAllowsProjectInternalPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sandbox, err := NewSandbox(root)
	if err != nil {
		t.Fatalf("NewSandbox returned error: %v", err)
	}
	check, err := sandbox.Resolve("notes.txt", "path")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if check.Raw != "notes.txt" || check.Parameter != "path" || check.Relative != "notes.txt" {
		t.Fatalf("unexpected path check: %#v", check)
	}
	if check.Real != filepath.Join(sandbox.RealRoot, "notes.txt") {
		t.Fatalf("unexpected real path %q", check.Real)
	}
}

func TestSandboxResolveRejectsRelativeAndAbsoluteEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sandbox, err := NewSandbox(root)
	if err != nil {
		t.Fatalf("NewSandbox returned error: %v", err)
	}
	tests := []string{
		filepath.Join("..", filepath.Base(outside), "secret.txt"),
		filepath.Join(outside, "secret.txt"),
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if _, err := sandbox.Resolve(raw, "path"); !errors.Is(err, ErrPathEscapesSandbox) {
				t.Fatalf("expected ErrPathEscapesSandbox, got %v", err)
			}
		})
	}
}

func TestSandboxResolveRejectsSymlinkToOutside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation often needs privileges on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	sandbox, err := NewSandbox(root)
	if err != nil {
		t.Fatalf("NewSandbox returned error: %v", err)
	}
	if _, err := sandbox.Resolve("link.txt", "path"); !errors.Is(err, ErrPathEscapesSandbox) {
		t.Fatalf("expected symlink file escape rejection, got %v", err)
	}
}

func TestSandboxResolveRejectsSymlinkParentWriteEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation often needs privileges on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	sandbox, err := NewSandbox(root)
	if err != nil {
		t.Fatalf("NewSandbox returned error: %v", err)
	}
	if _, err := sandbox.Resolve(filepath.Join("out", "new.txt"), "path"); !errors.Is(err, ErrPathEscapesSandbox) {
		t.Fatalf("expected symlink parent write escape rejection, got %v", err)
	}
}
