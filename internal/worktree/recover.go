package worktree

import (
	"os"
	"path/filepath"
	"strings"
)

func recoverWorktree(path, name, branch string) (Worktree, bool) {
	head, ok := readHead(path)
	if !ok {
		return Worktree{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return Worktree{}, false
	}
	return Worktree{Name: name, Path: filepath.Clean(path), Branch: branch, BasedOn: "HEAD", HeadCommit: head, CreatedAt: info.ModTime()}, true
}

func isRecoveredPath(path string) bool { _, ok := readHead(path); return ok }

func readHead(path string) (string, bool) {
	pointer, err := os.ReadFile(filepath.Join(path, ".git"))
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(pointer))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", false
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(path, gitdir)
	}
	head, err := os.ReadFile(filepath.Join(gitdir, "HEAD"))
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(head))
	if !strings.HasPrefix(value, "ref: ") {
		return value, len(value) >= 7
	}
	ref := strings.TrimSpace(strings.TrimPrefix(value, "ref: "))
	if !strings.HasPrefix(ref, "refs/") || strings.Contains(ref, "..") {
		return "", false
	}
	sha, err := os.ReadFile(filepath.Join(gitdir, ref))
	if err == nil {
		return strings.TrimSpace(string(sha)), true
	}
	common, err := os.ReadFile(filepath.Join(gitdir, "commondir"))
	if err != nil {
		return "", false
	}
	commonDir := strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitdir, commonDir)
	}
	sha, err = os.ReadFile(filepath.Join(commonDir, ref))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(sha)), true
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
