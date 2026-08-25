package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateRecoverAndRemove(t *testing.T) {
	root := testRepo(t)
	m := NewManager(root)
	wt, err := m.Create(context.Background(), "team/one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatal(err)
	}
	if wt.HeadCommit == "" {
		t.Fatal("missing head")
	}
	second, err := NewManager(root).Create(context.Background(), "team/one")
	if err != nil {
		t.Fatal(err)
	}
	if second.Path != wt.Path || second.HeadCommit != wt.HeadCommit {
		t.Fatalf("did not recover: %#v", second)
	}
	if err := m.Remove(context.Background(), "team/one", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree remains: %v", err)
	}
}

func TestRemoveProtectsChanges(t *testing.T) {
	root := testRepo(t)
	m := NewManager(root)
	wt, err := m.Create(context.Background(), "tmp-safe")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(context.Background(), wt.Name, false); err == nil {
		t.Fatal("removed changed worktree")
	}
	if err := m.Remove(context.Background(), wt.Name, true); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveProtectsNewCommit(t *testing.T) {
	root := testRepo(t)
	m := NewManager(root)
	wt, err := m.Create(context.Background(), "tmp-commit")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wt.Path, "add", "new.txt")
	runGit(t, wt.Path, "commit", "-m", "change")
	if err := m.Remove(context.Background(), wt.Name, false); err == nil {
		t.Fatal("removed worktree with new commit")
	}
}

func TestEnterExitAndRecover(t *testing.T) {
	root := testRepo(t)
	m := NewManager(root)
	s, err := m.Enter(context.Background(), "session")
	if err != nil {
		t.Fatal(err)
	}
	if s.WorktreePath == "" {
		t.Fatal("missing path")
	}
	recovered, err := NewManager(root).RecoverSession()
	if err != nil || recovered == nil || recovered.WorktreeName != "session" {
		t.Fatalf("recover: %#v, %v", recovered, err)
	}
	if err := m.Exit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mewcode", "worktree_session.json")); !os.IsNotExist(err) {
		t.Fatalf("session persists: %v", err)
	}
}

func TestCreateInitializesConfiguredAndIncludedFiles(t *testing.T) {
	root := testRepo(t)
	if err := os.WriteFile(filepath.Join(root, ".local.env"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime.json"), []byte("runtime"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".worktreeinclude"), []byte("# runtime input\nruntime.json\n../outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "config", "core.hooksPath", ".githooks")
	m := NewManager(root)
	m.Options = Options{LocalFiles: []string{".local.env"}, SymlinkDirectories: []string{"node_modules"}}
	wt, err := m.Create(context.Background(), "init")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".local.env", "runtime.json"} {
		if data, err := os.ReadFile(filepath.Join(wt.Path, name)); err != nil || len(data) == 0 {
			t.Fatalf("%s was not copied: %v", name, err)
		}
	}
	info, err := os.Lstat(filepath.Join(wt.Path, "node_modules"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("node_modules was not linked: %v", err)
	}
	hooks, err := m.gitText(context.Background(), wt.Path, "config", "core.hooksPath")
	if err != nil || hooks != ".githooks" {
		t.Fatalf("hooks path = %q, %v", hooks, err)
	}
}

func TestCleanupExpiredSkipsChangedWorktree(t *testing.T) {
	root := testRepo(t)
	m := NewManager(root)
	m.Options.Retention = time.Hour
	wt, err := m.Create(context.Background(), "tmp-old")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(wt.Path, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := m.CleanupExpired(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("changed worktree removed: %v", err)
	}
}

func testRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		runGit(t, root, args...)
	}
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README")
	runGit(t, root, "commit", "-m", "base")
	return root
}
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
