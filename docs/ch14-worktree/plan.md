# MewCode Git Worktree 文件系统隔离 Plan

## 架构概览

本章新增 `internal/worktree` 作为 Git Worktree 的唯一生命周期边界，负责安全名称验证、路径与分支推导、Git 调用、快速恢复、初始化、会话持久化、变更保护及清理；不依赖 TUI、LLM 或 SubAgent。

Worktree 会话不改变进程 cwd。进入 Worktree 的 Runner 使用 `Workspace.Root` 为 Worktree 路径的独立工具注册表，使文件与命令工具沿用现有路径解析和 `cmd.Dir`，自然以 Worktree 为显式 cwd。

`SubAgentRuntime` 在创建子 Runner 前决定隔离：Fork 强制创建临时 Worktree；定义式 Agent 由 frontmatter 与调用参数决定。隔离 Runner 注入路径说明，终态后自动清理干净目录，保留有改动目录并回传路径和分支。未隔离的前台定义式 Agent 保持现有工作区且不可转后台。

`internal/command` 增加 `/worktree` 本地命令。组合根创建唯一 Manager，恢复会话并注入命令和 SubAgent Runtime。

```text
/worktree create | enter | exit | remove | list
  → WorktreeManager → Git / 文件系统 / 会话存储

Agent 工具 → SubAgentRuntime 判定隔离
  → 创建临时 Worktree（Fork 强制）→ Worktree Workspace 子 Runner
  → 终态 AutoCleanup → 主 Agent 摘要
```

## 核心数据结构

```go
type Worktree struct {
    Name, Path, Branch, BasedOn, HeadCommit string
    CreatedAt time.Time
}

type Session struct {
    OriginalWorkspace, WorktreePath, WorktreeName string
    OriginalBranch, OriginalHeadCommit, ID string
    HookBased bool
}

type Manager struct {
    RepoRoot, Directory string
    active map[string]Worktree
    current *Session
}
```

`Worktree` 描述可保留目录；`Session` 仅描述当前进入的目录，持久化到 `.mewcode/worktree_session.json`。退出清除 Session，不强制删除 Worktree。

`subagent.Definition` 新增 `Isolation string`；`tools.AgentInput.Isolation` 支持 `worktree`。Fork 忽略 `none` 或空值，始终创建临时 Worktree。

## 模块设计

### `internal/worktree`

- `manager.go`：生命周期、锁与状态。
- `validate.go`：slug 校验与路径、分支推导。
- `git.go`：非交互 Git 执行、变更和 hooks 查询。
- `recover.go`：只读 `.git`、`HEAD` 与 ref 的快速恢复。
- `setup.go`：本地配置、hooks、依赖软链接和 `.worktreeinclude` 的尽力初始化。
- `session.go`：会话原子读写与恢复。
- `cleanup.go`：删除保护、自动和过期清理。

### 既有模块

- `internal/tools`：新增按 `Workspace` 构造 registry 的工厂；工具 schema 与权限语义保持不变。
- `internal/subagent`：解析、校验 `isolation` frontmatter。
- `internal/agent`：启动前隔离决策、独立工具 registry、路径通知、仅隔离任务可后台接管、终态自动清理。
- `internal/command`：增加 `/worktree`，通过窄服务接口调用 Manager。
- `cmd/mewcode/main.go`：组装 Manager、恢复会话并注入依赖。
- 配置与用户文档：增加初始化、清理配置并同步 README、示例配置与章节索引。

## 文件组织

```text
internal/worktree/{manager,validate,git,recover,setup,session,cleanup}.go
internal/worktree/*_test.go
internal/tools/{registry,agent}.go
internal/subagent/{definition,discover}.go
internal/agent/{subagent,runner,scheduler}.go
internal/command/{command,builtins}.go
internal/config/{config,load,validate}.go
cmd/mewcode/main.go
.mewcode/config.example.yaml
README.md · docs/README.md · docs/ch14-worktree/*
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| Worktree 位置 | `.mewcode/worktrees/` | 项目内可发现且不被追踪。 |
| Fork 隔离 | 启动时强制临时 Worktree | Fork 后台且可写，避免文件冲突。 |
| 前台转后台 | 仅已隔离任务允许 | 避免运行中迁移造成状态不一致。 |
| cwd | 独立 `Workspace` | 不修改全局 cwd，复用现有工具边界。 |
| 快速恢复 | 读取 `.git` 与 `HEAD` | 避免重复检出和 Git 子进程。 |
| 删除策略 | fail-closed | 状态不确定时保护成果。 |
| 初始化失败 | best-effort + 安全警告 | 可选环境项不应破坏已创建目录。 |
