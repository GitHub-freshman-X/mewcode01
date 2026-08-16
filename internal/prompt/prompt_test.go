package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestModuleOrderAndRendering(t *testing.T) {
	env := testEnvironment(ModeAct)
	bundle, modules, err := BuildBundle(BuildContext{Environment: env, Mode: ModeAct, Iteration: 1})
	if err != nil {
		t.Fatalf("BuildBundle returned error: %v", err)
	}

	wantKeys := []ModuleKey{
		ModuleIdentity,
		ModuleSystemRules,
		ModuleTaskMode,
		ModuleAction,
		ModuleToolUse,
		ModuleTone,
		ModuleOutput,
	}
	if len(modules) != len(wantKeys) {
		t.Fatalf("module count = %d, want %d: %#v", len(modules), len(wantKeys), modules)
	}
	for i, want := range wantKeys {
		if modules[i].Key != want {
			t.Fatalf("module %d key = %q, want %q", i, modules[i].Key, want)
		}
	}
	for _, title := range []string{
		"## 身份",
		"## 系统约束",
		"## 任务模式",
		"## 动作执行",
		"## 工具使用",
		"## 语气风格",
		"## 文本输出",
	} {
		if !strings.Contains(bundle.StableSystem, title) {
			t.Fatalf("stable system missing title %q:\n%s", title, bundle.StableSystem)
		}
	}
	if strings.Contains(bundle.StableSystem, "\n\n\n") {
		t.Fatalf("stable system contains extra blank lines:\n%s", bundle.StableSystem)
	}
	again, _, err := BuildBundle(BuildContext{Environment: env, Mode: ModeAct, Iteration: 1})
	if err != nil {
		t.Fatalf("second BuildBundle returned error: %v", err)
	}
	if bundle.StableSystem != again.StableSystem {
		t.Fatalf("stable system is not deterministic")
	}
}

func TestFixedModulesCoverSystemPromptPolicy(t *testing.T) {
	bundle, _, err := BuildBundle(BuildContext{
		Environment: testEnvironment(ModeAct),
		Mode:        ModeAct,
		Iteration:   1,
	})
	if err != nil {
		t.Fatalf("BuildBundle returned error: %v", err)
	}

	for _, want := range []string{
		"终端环境中的 AI 编程助手",
		"回复应简短",
		"探索性问题",
		"不确定时先询问",
		"不要添加超出任务需求的功能、抽象或重构",
		"OWASP Top 10",
		"破坏性操作前先获得用户确认",
		"不要猜测或编造 URL",
		"不要跳过 Git hook 或绕过签名检查",
		"file_path:line_number",
	} {
		if !strings.Contains(bundle.StableSystem, want) {
			t.Fatalf("stable system missing policy %q:\n%s", want, bundle.StableSystem)
		}
	}
}

func TestOptionalModules(t *testing.T) {
	env := testEnvironment(ModeAct)
	empty, _, err := BuildBundle(BuildContext{Environment: env, Mode: ModeAct, Iteration: 1})
	if err != nil {
		t.Fatalf("BuildBundle returned error: %v", err)
	}
	for _, title := range []string{"## 自定义指令", "## 已激活 Skill", "## 长期记忆"} {
		if strings.Contains(empty.StableSystem, title) {
			t.Fatalf("empty optional module title %q should not be rendered", title)
		}
	}

	withOptions, modules, err := BuildBundle(BuildContext{
		Environment: env,
		Mode:        ModeAct,
		Iteration:   1,
		OptionalModules: OptionalModules{
			CustomInstructions: []string{"custom one"},
			ActiveSkills:       []string{"skill one"},
			LongTermMemory:     []string{"memory one"},
		},
	})
	if err != nil {
		t.Fatalf("BuildBundle returned error: %v", err)
	}
	if got, want := modules[len(modules)-3].Key, ModuleCustom; got != want {
		t.Fatalf("first optional key = %q, want %q", got, want)
	}
	if got, want := modules[len(modules)-2].Key, ModuleSkills; got != want {
		t.Fatalf("second optional key = %q, want %q", got, want)
	}
	if got, want := modules[len(modules)-1].Key, ModuleMemory; got != want {
		t.Fatalf("third optional key = %q, want %q", got, want)
	}
	if !strings.Contains(withOptions.StableSystem, "custom one") ||
		!strings.Contains(withOptions.StableSystem, "skill one") ||
		!strings.Contains(withOptions.StableSystem, "memory one") {
		t.Fatalf("optional content missing:\n%s", withOptions.StableSystem)
	}
}

func TestCollectEnvironment(t *testing.T) {
	registry := testRegistry(t)
	env, err := CollectEnvironment(ModePlan, registry, "/tmp/project", fixedClock{time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("CollectEnvironment returned error: %v", err)
	}
	if env.Workspace != "/tmp/project" || env.Date != "2026-07-09" || env.Mode != ModePlan {
		t.Fatalf("unexpected environment: %+v", env)
	}
	if env.OS == "" || env.Shell == "" {
		t.Fatalf("environment missing OS or shell: %+v", env)
	}
	if got := strings.Join(env.Tools, ","); got != "find_files,read_file" {
		t.Fatalf("tools = %q, want find_files,read_file", got)
	}
	msg, err := EnvironmentMessage(env)
	if err != nil {
		t.Fatalf("EnvironmentMessage returned error: %v", err)
	}
	if msg.Tag != "mew.environment" || !strings.Contains(msg.Content, "Workspace: /tmp/project") {
		t.Fatalf("unexpected environment message: %+v", msg)
	}
}

func TestCollectEnvironmentShellPriority(t *testing.T) {
	t.Setenv("SHELL", "/bin/test-shell")
	t.Setenv("COMSPEC", "C:\\Windows\\System32\\cmd.exe")

	env, err := CollectEnvironment(ModeAct, testRegistry(t), "/tmp/project", fixedClock{time.Now()})
	if err != nil {
		t.Fatalf("CollectEnvironment returned error: %v", err)
	}
	if env.Shell != "/bin/test-shell" {
		t.Fatalf("Shell = %q, want SHELL value", env.Shell)
	}
}

func TestCollectEnvironmentCOMSPECFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	t.Setenv("COMSPEC", "C:\\Windows\\System32\\cmd.exe")

	env, err := CollectEnvironment(ModeAct, testRegistry(t), "/tmp/project", fixedClock{time.Now()})
	if err != nil {
		t.Fatalf("CollectEnvironment returned error: %v", err)
	}
	if env.Shell != "C:\\Windows\\System32\\cmd.exe" {
		t.Fatalf("Shell = %q, want COMSPEC value", env.Shell)
	}
	if _, err := EnvironmentMessage(env); err != nil {
		t.Fatalf("EnvironmentMessage returned error: %v", err)
	}
}

func TestCollectEnvironmentShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	t.Setenv("COMSPEC", "")

	env, err := CollectEnvironment(ModeAct, testRegistry(t), "/tmp/project", fixedClock{time.Now()})
	if err != nil {
		t.Fatalf("CollectEnvironment returned error: %v", err)
	}
	if env.Shell == "" {
		t.Fatal("Shell fallback is empty")
	}
}

func TestEnvironmentRequired(t *testing.T) {
	_, err := CollectEnvironment(ModeAct, nil, "/tmp/project", fixedClock{time.Now()})
	if err == nil || !strings.Contains(err.Error(), "tool registry") {
		t.Fatalf("missing registry error = %v", err)
	}
	_, err = CollectEnvironment(ModeAct, tools.NewRegistry(), "", fixedClock{time.Now()})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("missing workspace error = %v", err)
	}
}

func TestModeInjection(t *testing.T) {
	policy := InjectionPolicy{ReminderEvery: 3, BriefEvery: 2}
	cases := []struct {
		name      string
		mode      Mode
		iteration int
		wantKind  InjectionKind
		wantText  string
	}{
		{"plan first full", ModePlan, 1, InjectionFull, "只读"},
		{"plan reminder", ModePlan, 3, InjectionReminder, "不得修改"},
		{"plan brief", ModePlan, 2, InjectionBrief, "规划模式"},
		{"plan none", ModePlan, 5, InjectionNone, ""},
		{"do first full", ModeDo, 1, InjectionFull, "执行"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ModeInjectionFor(tc.mode, tc.iteration, policy)
			if got.Kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if tc.wantText != "" && !strings.Contains(got.Content, tc.wantText) {
				t.Fatalf("content %q does not contain %q", got.Content, tc.wantText)
			}
		})
	}
}

func TestBuildBundleStableDynamicSplit(t *testing.T) {
	env1 := testEnvironment(ModeAct)
	env2 := testEnvironment(ModePlan)
	env2.Workspace = "/tmp/other"
	env2.Date = "2026-07-10"

	first, _, err := BuildBundle(BuildContext{Environment: env1, Mode: ModeAct, Iteration: 1})
	if err != nil {
		t.Fatalf("BuildBundle first returned error: %v", err)
	}
	second, _, err := BuildBundle(BuildContext{Environment: env2, Mode: ModePlan, Iteration: 1})
	if err != nil {
		t.Fatalf("BuildBundle second returned error: %v", err)
	}
	if first.StableSystem != second.StableSystem {
		t.Fatalf("stable system changed when dynamic environment changed")
	}
	if len(first.DynamicSystem) == 0 || len(second.DynamicSystem) == 0 {
		t.Fatalf("dynamic messages missing")
	}
	if first.DynamicSystem[0].Content == second.DynamicSystem[0].Content {
		t.Fatalf("dynamic environment did not change")
	}
	if !first.CachePolicy.Enable || !first.CachePolicy.StableSystem || !first.CachePolicy.StableTools || first.CachePolicy.DynamicMessages {
		t.Fatalf("unexpected cache policy: %+v", first.CachePolicy)
	}
}

func TestEnhanceDefinitionsAndToolRulesStable(t *testing.T) {
	defs := []provider.ToolDefinition{
		{Name: "read_file", Description: "Read a file."},
		{Name: "write_file", Description: "Write a file."},
		{Name: "search_code", Description: "Search code."},
	}

	act := EnhanceDefinitions(defs, ModeAct)
	plan := EnhanceDefinitions(defs, ModePlan)

	if len(act) != len(defs) || len(plan) != len(defs) {
		t.Fatalf("enhanced length changed")
	}
	if act[0].Name != "read_file" || act[1].Name != "write_file" || act[2].Name != "search_code" {
		t.Fatalf("tool order changed: %+v", act)
	}
	for _, def := range act {
		if !def.Cacheable {
			t.Fatalf("tool %s is not cacheable", def.Name)
		}
		if !strings.Contains(def.Description, "编辑前") {
			t.Fatalf("tool %s missing global rule: %q", def.Name, def.Description)
		}
	}
	if !strings.Contains(act[0].Description, "读取目标文件") {
		t.Fatalf("read_file missing read-specific rule: %q", act[0].Description)
	}
	if !strings.Contains(act[1].Description, "工作区边界") {
		t.Fatalf("write_file missing workspace rule: %q", act[1].Description)
	}
	if !strings.Contains(act[2].Description, "搜索工具") {
		t.Fatalf("search_code missing search-specific rule: %q", act[2].Description)
	}
	if !strings.Contains(plan[1].Description, "规划模式不得请求副作用工具") {
		t.Fatalf("plan mode side-effect rule missing: %q", plan[1].Description)
	}
	for _, want := range []string{
		"优先使用专用工具",
		"编辑前必须先读取",
		"搜索优先使用搜索工具",
		"破坏性操作前先获得用户确认",
		"不要猜测或编造 URL",
	} {
		if !strings.Contains(act[0].Description, want) {
			t.Fatalf("tool rule missing %q: %q", want, act[0].Description)
		}
	}

	again := EnhanceDefinitions(defs, ModeAct)
	for i := range act {
		if act[i].Description != again[i].Description {
			t.Fatalf("tool enhancement is not stable for %s", act[i].Name)
		}
	}
}

func testEnvironment(mode Mode) Environment {
	return Environment{
		Workspace: "/tmp/project",
		OS:        "darwin",
		Shell:     "zsh",
		Date:      "2026-07-09",
		Mode:      mode,
		Tools:     []string{"read_file", "search_code"},
	}
}

func testRegistry(t *testing.T) *tools.Registry {
	t.Helper()
	workspace, err := tools.NewWorkspace("/tmp/project")
	if err != nil {
		t.Fatalf("NewWorkspace returned error: %v", err)
	}
	registry := tools.NewRegistry()
	for _, tool := range []tools.Tool{
		tools.NewFindFilesTool(workspace),
		tools.NewReadFileTool(workspace),
	} {
		if err := registry.Register(tool); err != nil {
			t.Fatalf("Register returned error: %v", err)
		}
	}
	return registry
}
