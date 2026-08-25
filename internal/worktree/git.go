package worktree

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type GitError struct {
	Output string
	Err    error
}

func (e *GitError) Error() string { return e.Err.Error() + ": " + strings.TrimSpace(e.Output) }
func (e *GitError) Unwrap() error { return e.Err }

func (m *Manager) git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, &GitError{Output: string(out), Err: err}
	}
	return out, nil
}

func (m *Manager) gitText(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := m.git(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitErrorf(format string, args ...any) error { return fmt.Errorf(format, args...) }
