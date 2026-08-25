package worktree

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// setup intentionally never makes creation fail: project-local runtime inputs
// are optional and a usable Git worktree is more valuable than a partial copy.
func (m *Manager) setup(wt Worktree) {
	for _, name := range m.Options.LocalFiles {
		source := filepath.Join(m.RepoRoot, name)
		target := filepath.Join(wt.Path, name)
		if !within(m.RepoRoot, source) || !within(wt.Path, target) {
			continue
		}
		copyFile(source, target)
	}
	for _, name := range m.Options.SymlinkDirectories {
		source := filepath.Join(m.RepoRoot, name)
		target := filepath.Join(wt.Path, name)
		if !within(m.RepoRoot, source) || !within(wt.Path, target) {
			continue
		}
		if _, err := os.Stat(source); err != nil {
			continue
		}
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		_ = os.Symlink(source, target)
	}
	m.copyIncludes(wt)
}

// copyIncludes copies explicitly listed ignored runtime files. Each non-empty,
// non-comment line in .worktreeinclude is a path relative to the repository.
func (m *Manager) copyIncludes(wt Worktree) {
	include := filepath.Join(m.RepoRoot, ".worktreeinclude")
	file, err := os.Open(include)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name == "" || strings.HasPrefix(name, "#") || filepath.IsAbs(name) {
			continue
		}
		source := filepath.Join(m.RepoRoot, name)
		target := filepath.Join(wt.Path, name)
		if !within(m.RepoRoot, source) || !within(wt.Path, target) {
			continue
		}
		copyFile(source, target)
	}
}

func copyFile(source, target string) {
	in, err := os.Open(source)
	if err != nil {
		return
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil || info.IsDir() {
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return
	}
	defer out.Close()
	_, _ = io.Copy(out, in)
}
