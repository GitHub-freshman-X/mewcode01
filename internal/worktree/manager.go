package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
)

type Worktree struct {
	Name, Path, Branch, BasedOn, HeadCommit string
	CreatedAt                               time.Time
}

type Session struct {
	OriginalWorkspace, WorktreePath, WorktreeName, OriginalBranch, OriginalHeadCommit, ID string
	HookBased                                                                             bool
}

// Options configures best-effort worktree setup and conservative cleanup.
type Options struct {
	LocalFiles         []string
	SymlinkDirectories []string
	Retention          time.Duration
}

type Manager struct {
	RepoRoot  string
	Directory string
	Options   Options
	Logger    *logging.Logger

	mu      sync.Mutex
	active  map[string]Worktree
	current *Session
}

func NewManager(root string) *Manager {
	root, _ = filepath.Abs(root)
	root = filepath.Clean(root)
	return &Manager{RepoRoot: root, Directory: filepath.Join(root, ".mewcode", "worktrees"), active: make(map[string]Worktree)}
}

func (m *Manager) Create(ctx context.Context, name string) (Worktree, error) {
	path, err := worktreePath(m.RepoRoot, name)
	if err != nil {
		return Worktree{}, err
	}
	branch, err := branchName(name)
	if err != nil {
		return Worktree{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if wt, ok := m.active[name]; ok {
		m.log("worktree_create", "reused", "manual")
		return wt, nil
	}
	if wt, ok := recoverWorktree(path, name, branch); ok {
		m.active[name] = wt
		m.log("worktree_recovery", "recovered", "manual")
		return wt, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Worktree{}, fmt.Errorf("create worktree directory: %w", err)
	}
	if _, err := m.git(ctx, m.RepoRoot, "worktree", "add", "-B", branch, path, "HEAD"); err != nil {
		return Worktree{}, err
	}
	head, err := m.gitText(ctx, path, "rev-parse", "HEAD")
	if err != nil {
		return Worktree{}, err
	}
	if _, err := m.git(ctx, m.RepoRoot, "update-ref", m.baseRef(name), head); err != nil {
		return Worktree{}, err
	}
	wt := Worktree{Name: name, Path: filepath.Clean(path), Branch: branch, BasedOn: "HEAD", HeadCommit: head, CreatedAt: time.Now()}
	m.active[name] = wt
	m.setup(wt)
	m.log("worktree_create", "created", "manual")
	return wt, nil
}

func (m *Manager) List(ctx context.Context) ([]Worktree, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, err := os.ReadDir(m.Directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]Worktree, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ReplaceAll(entry.Name(), "+", "/")
		branch := "worktree-" + entry.Name()
		if wt, ok := recoverWorktree(filepath.Join(m.Directory, entry.Name()), name, branch); ok {
			m.active[name] = wt
			out = append(out, wt)
		}
	}
	return out, nil
}

func (m *Manager) Enter(ctx context.Context, name string) (Session, error) {
	wt, err := m.Create(ctx, name)
	if err != nil {
		return Session{}, err
	}
	branch, err := m.gitText(ctx, m.RepoRoot, "branch", "--show-current")
	if err != nil {
		return Session{}, err
	}
	head, err := m.gitText(ctx, m.RepoRoot, "rev-parse", "HEAD")
	if err != nil {
		return Session{}, err
	}
	s := Session{OriginalWorkspace: m.RepoRoot, WorktreePath: wt.Path, WorktreeName: wt.Name, OriginalBranch: branch, OriginalHeadCommit: head, ID: fmt.Sprintf("%d", time.Now().UnixNano())}
	m.mu.Lock()
	m.current = &s
	m.mu.Unlock()
	if err := m.saveSession(s); err != nil {
		return Session{}, err
	}
	m.log("worktree_enter", "entered", "manual")
	return s, nil
}

func (m *Manager) Exit() error {
	m.mu.Lock()
	m.current = nil
	m.mu.Unlock()
	err := os.Remove(m.sessionPath())
	if errors.Is(err, os.ErrNotExist) {
		m.log("worktree_exit", "exited", "manual")
		return nil
	}
	if err == nil {
		m.log("worktree_exit", "exited", "manual")
	}
	return err
}

func (m *Manager) Current() *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return nil
	}
	copy := *m.current
	return &copy
}

func (m *Manager) RecoverSession() (*Session, error) {
	data, err := os.ReadFile(m.sessionPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode worktree session: %w", err)
	}
	if s.WorktreePath == "" || !within(m.Directory, s.WorktreePath) || !isRecoveredPath(s.WorktreePath) {
		return nil, nil
	}
	m.mu.Lock()
	m.current = &s
	m.mu.Unlock()
	m.log("worktree_recovery", "restored_session", "manual")
	return &s, nil
}

func (m *Manager) Remove(ctx context.Context, name string, discard bool) error {
	path, err := worktreePath(m.RepoRoot, name)
	if err != nil {
		return err
	}
	branch, err := branchName(name)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil && m.current.WorktreeName == name {
		m.log("worktree_remove", "refused_active", "manual")
		return fmt.Errorf("worktree %q is active; exit it before removal", name)
	}
	if !discard {
		changed, err := m.changed(ctx, path, name)
		if err != nil {
			m.log("worktree_remove", "refused_unverified", "manual")
			return fmt.Errorf("preserve worktree because safety check failed: %w", err)
		}
		if changed {
			m.log("worktree_remove", "refused_changes", "manual")
			return fmt.Errorf("worktree %q has uncommitted or new commits; pass discard to remove it", name)
		}
	}
	args := []string{"worktree", "remove"}
	if discard {
		args = append(args, "--force")
	}
	args = append(args, path)
	if _, err := m.git(ctx, m.RepoRoot, args...); err != nil {
		return err
	}
	if _, err := m.git(ctx, m.RepoRoot, "branch", "-D", branch); err != nil {
		return err
	}
	_, _ = m.git(ctx, m.RepoRoot, "update-ref", "-d", m.baseRef(name))
	delete(m.active, name)
	m.log("worktree_remove", "removed", "manual")
	return nil
}

func (m *Manager) AutoCleanup(ctx context.Context, wt Worktree) error {
	if !strings.HasPrefix(wt.Name, "tmp-") {
		return nil
	}
	err := m.Remove(ctx, wt.Name, false)
	if err != nil {
		m.log("worktree_cleanup", "retained", "temporary")
		return err
	}
	m.log("worktree_cleanup", "removed", "temporary")
	return nil
}

// CleanupExpired removes only temporary worktrees that are older than the
// configured retention period and pass the same conservative removal checks.
func (m *Manager) CleanupExpired(ctx context.Context, now time.Time) error {
	if m.Options.Retention <= 0 {
		return nil
	}
	items, err := m.List(ctx)
	if err != nil {
		return err
	}
	for _, wt := range items {
		if !strings.HasPrefix(wt.Name, "tmp-") || now.Sub(wt.CreatedAt) < m.Options.Retention {
			continue
		}
		m.mu.Lock()
		active := m.current != nil && m.current.WorktreeName == wt.Name
		m.mu.Unlock()
		if active {
			continue
		}
		if err := m.Remove(ctx, wt.Name, false); err != nil {
			m.log("worktree_cleanup", "skipped", "expired")
			continue
		}
		m.log("worktree_cleanup", "removed", "expired")
	}
	return nil
}

func (m *Manager) log(stage, status, kind string) {
	if m.Logger != nil {
		m.Logger.Info("worktree lifecycle", logging.Fields{"stage": stage, "status": status, "kind": kind})
	}
}

func (m *Manager) changed(ctx context.Context, path, name string) (bool, error) {
	status, err := m.gitText(ctx, path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	if status != "" {
		return true, nil
	}
	base, err := m.gitText(ctx, path, "rev-parse", m.baseRef(name))
	if err != nil {
		return false, err
	}
	ahead, err := m.gitText(ctx, path, "rev-list", "--count", base+"..HEAD")
	if err != nil {
		return false, err
	}
	return ahead != "0", nil
}

func (m *Manager) sessionPath() string {
	return filepath.Join(m.RepoRoot, ".mewcode", "worktree_session.json")
}

func (m *Manager) baseRef(name string) string { return "refs/mewcode/worktrees/" + flatSlug(name) }

func (m *Manager) saveSession(s Session) error {
	if err := os.MkdirAll(filepath.Dir(m.sessionPath()), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := m.sessionPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.sessionPath())
}
