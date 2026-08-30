package command

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/hooks"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/skills"
	"github.com/GitHub-freshman-X/mewcode01/internal/status"
	"github.com/GitHub-freshman-X/mewcode01/internal/worktree"
)

func TestRegistryRejectsConflicts(t *testing.T) {
	handler := func(CommandContext) error { return nil }
	for _, commands := range [][]Command{
		{{Name: "one", Handler: handler}, {Name: "one", Handler: handler}},
		{{Name: "one", Aliases: []string{"x"}, Handler: handler}, {Name: "x", Handler: handler}},
		{{Name: "one", Aliases: []string{"x"}, Handler: handler}, {Name: "two", Aliases: []string{"x"}, Handler: handler}},
	} {
		if _, err := NewRegistry(commands...); err == nil || !strings.Contains(err.Error(), "x") && !strings.Contains(err.Error(), "one") {
			t.Fatalf("conflict error = %v", err)
		}
	}
}

func TestParseAndComplete(t *testing.T) {
	if got := Parse(" /HELP  topic "); !got.IsCommand || got.Name != "help" || got.Args != "topic" {
		t.Fatalf("parse=%+v", got)
	}
	if got := Parse("plain"); got.IsCommand {
		t.Fatalf("parse=%+v", got)
	}
	r := DefaultRegistry()
	if got := r.Complete("/he"); len(got) != 1 || got[0] != "/help" {
		t.Fatalf("complete=%v", got)
	}
	if c, ok := r.Find("H"); !ok || c.Name != "help" {
		t.Fatalf("alias=%+v %v", c, ok)
	}
}

type fakeUI struct {
	messages []string
	requests []agent.Request
	plan     bool
	exit     bool
	usage    provider.Usage
}

func (u *fakeUI) AddSystemMessage(s string)        { u.messages = append(u.messages, s) }
func (u *fakeUI) StartAgent(r agent.Request) error { u.requests = append(u.requests, r); return nil }
func (u *fakeUI) SetPlanMode(v bool)               { u.plan = v }
func (u *fakeUI) PlanMode() bool                   { return u.plan }
func (u *fakeUI) RequestExit()                     { u.exit = true }
func (u *fakeUI) TokenUsage() provider.Usage       { return u.usage }
func (u *fakeUI) RefreshStatus()                   {}
func (u *fakeUI) MemoryClearPending() bool         { return false }
func (u *fakeUI) SetMemoryClearPending(bool)       {}

type fakeStatus struct{ snapshot status.Snapshot }

func (s fakeStatus) StatusSnapshot() status.Snapshot { return s.snapshot }

type fakeSessions struct {
	current SessionMeta
	list    []SessionMeta
}

func (s fakeSessions) Current() SessionMeta                 { return s.current }
func (s fakeSessions) List() ([]SessionMeta, error)         { return s.list, nil }
func (s fakeSessions) New(context.Context) error            { return nil }
func (s fakeSessions) Resume(context.Context, string) error { return nil }
func (s fakeSessions) Delete(string) error                  { return nil }

func TestDispatchLocalAndPrompt(t *testing.T) {
	u := &fakeUI{}
	ctx := CommandContext{Context: context.Background(), UI: u}
	commands := append(DefaultCommands(), SkillCommands([]skills.Metadata{skills.Builtins()[1].Metadata})...)
	registry, err := NewRegistry(commands...)
	if err != nil {
		t.Fatal(err)
	}
	if err := Dispatch(registry, Parse("/help"), ctx); err != nil || len(u.messages) != 1 || len(u.requests) != 0 {
		t.Fatalf("help messages=%v requests=%v err=%v", u.messages, u.requests, err)
	}
	if !strings.Contains(u.messages[0], "/review — 结合当前对话审查代码变更并报告可操作问题。") {
		t.Fatalf("help=%q", u.messages[0])
	}
	if err := Dispatch(registry, Parse("/review concurrency"), ctx); err != nil || len(u.requests) != 1 || u.requests[0].Skill == nil || u.requests[0].Skill.Name != "review" || u.requests[0].Skill.Args != "concurrency" {
		t.Fatalf("review=%+v err=%v", u.requests, err)
	}
	if got := u.requests[0].Prompt; got != "/review concurrency" {
		t.Fatalf("review prompt=%q", got)
	}
	if err := Dispatch(registry, Parse("/plan task"), ctx); err != nil || !u.plan || len(u.requests) != 2 || u.requests[1].Mode != agent.ModePlan {
		t.Fatalf("plan=%+v mode=%v err=%v", u.requests, u.plan, err)
	}
}

func TestDispatchEmitsCommandHook(t *testing.T) {
	sink := &hookSink{}
	engine := hooks.NewEngine([]hooks.Rule{{ID: "command", Event: hooks.EventCommandExecute, Action: hooks.Action{Type: hooks.ActionPrompt, Message: "$MESSAGE"}}}, hooks.Executor{PromptSink: sink}, nil)
	registry, err := NewRegistry(Command{Name: "one", Handler: func(CommandContext) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := Dispatch(registry, Parse("/one value"), CommandContext{Context: context.Background(), Hooks: engine}); err != nil {
		t.Fatal(err)
	}
	if len(sink.messages) != 1 || sink.messages[0] != "/one value" {
		t.Fatalf("messages=%v", sink.messages)
	}
}

type hookSink struct{ messages []string }

func (s *hookSink) AddHookNotification(message string) { s.messages = append(s.messages, message) }

func TestExitCommandRequestsExitWithoutAgent(t *testing.T) {
	u := &fakeUI{}
	if err := Dispatch(DefaultRegistry(), Parse("/exit"), CommandContext{Context: context.Background(), UI: u}); err != nil {
		t.Fatal(err)
	}
	if !u.exit || len(u.requests) != 0 || len(u.messages) != 0 {
		t.Fatalf("exit=%v requests=%v messages=%v", u.exit, u.requests, u.messages)
	}
}

func TestWorktreeCommandLifecycle(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		commandGit(t, root, args...)
	}
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	commandGit(t, root, "add", "README")
	commandGit(t, root, "commit", "-m", "base")
	ui := &fakeUI{}
	ctx := CommandContext{Context: context.Background(), UI: ui, Worktrees: worktree.NewManager(root)}
	for _, input := range []string{"/worktree create review", "/worktree list", "/worktree enter review", "/worktree exit", "/worktree remove review"} {
		if err := Dispatch(DefaultRegistry(), Parse(input), ctx); err != nil {
			t.Fatalf("%s: %v", input, err)
		}
	}
	if len(ui.messages) != 5 || !strings.Contains(ui.messages[0], "worktrees") || !strings.Contains(ui.messages[1], "review") || !strings.Contains(ui.messages[4], "已删除") {
		t.Fatalf("messages=%v", ui.messages)
	}
}

func commandGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func TestSessionListUsesTitle(t *testing.T) {
	u := &fakeUI{}
	longTitle := "  第一条消息含有中文和 emoji 😀，并且它足够长，应该在字符边界截断而不会破坏最后一个字符，同时还要保留省略号  "
	sessions := fakeSessions{list: []SessionMeta{
		{ID: "20260816-100000-a1b2", Title: longTitle, MessageCount: 99},
		{ID: "20260816-090000-c3d4"},
	}}
	ctx := CommandContext{Context: context.Background(), UI: u, Sessions: sessions}
	if err := Dispatch(DefaultRegistry(), Parse("/session list"), ctx); err != nil {
		t.Fatal(err)
	}
	if len(u.messages) != 1 {
		t.Fatalf("messages=%v", u.messages)
	}
	message := u.messages[0]
	if !strings.Contains(message, "20260816-100000-a1b2 — 第一条消息") || !strings.Contains(message, "…") {
		t.Fatalf("message=%q", message)
	}
	if !strings.Contains(message, "20260816-090000-c3d4 — （空会话）") || strings.Contains(message, "99 条消息") {
		t.Fatalf("message=%q", message)
	}
}

func TestStatusCommandRendersSafeRuntimeSnapshot(t *testing.T) {
	ui := &fakeUI{usage: provider.Usage{InputTokens: 12, OutputTokens: 4, CacheReadInputTokens: 8, CacheCreationInputTokens: 4}}
	ctx := CommandContext{Context: context.Background(), UI: ui, Sessions: fakeSessions{current: SessionMeta{ID: "session-1", MessageCount: 3}}, Status: fakeStatus{snapshot: status.Snapshot{
		Workspace: "/fixture", LogDirectory: "/fixture/logs", PermissionMode: "default",
		ToolCount: 8, SkillCount: 2, UserMemoryCount: 1, ProjectMemoryCount: 3, MemoryAvailable: true,
		SubAgentDefinitionCount: 4,
		BackgroundTasks:         []status.BackgroundTask{{ID: "subagent-1", Name: "verify", Status: "running", Elapsed: 3 * time.Second}},
	}}}
	if err := Dispatch(DefaultRegistry(), Parse("/status"), ctx); err != nil {
		t.Fatal(err)
	}
	if len(ui.messages) != 1 {
		t.Fatalf("messages=%v", ui.messages)
	}
	for _, want := range []string{"MewCode 状态", "工作目录：/fixture", "日志目录：/fixture/logs", "会话：session-1（3 条消息）", "Token：in:12 out:4", "缓存读取：8", "缓存写入：4", "缓存命中率：33%", "工具：8 · Skill：2 · 记忆：用户 1 · 项目 3 · SubAgent 定义：4", "后台任务：1", "subagent-1 · verify · running · 3s"} {
		if !strings.Contains(ui.messages[0], want) {
			t.Fatalf("status missing %q: %q", want, ui.messages[0])
		}
	}
}

func TestFormatSessionTitle(t *testing.T) {
	if got := formatSessionTitle(" \n "); got != "（空会话）" {
		t.Fatalf("empty title=%q", got)
	}
	value := strings.Repeat("😀", sessionTitleLimit+1)
	if got := formatSessionTitle(value); got != strings.Repeat("😀", sessionTitleLimit)+"…" {
		t.Fatalf("title=%q", got)
	}
}
