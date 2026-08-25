# MewCode Git Worktree 文件系统隔离 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/worktree/*.go` | 生命周期、安全、初始化与清理 |
| 修改 | `internal/tools/{registry,agent}.go` | 独立 Workspace registry、隔离参数 |
| 修改 | `internal/subagent/*`、`internal/agent/*` | 定义隔离与 Fork 接入 |
| 修改 | `internal/command/*`、配置、启动入口 | 手动管理和依赖组装 |
| 修改 | README、示例配置、章节文档 | 用户说明与验收资料 |

## T1：数据模型与 slug 校验

**文件：** `internal/worktree/{manager,validate}.go`、测试；**依赖：** 无。定义 Manager、Worktree、Session 与服务接口；实现长度、字符集、嵌套段校验及受控路径、分支名推导。

**验证：** 合法与非法输入单元测试；非法输入不执行 Git。

## T2：Git 生命周期与快速恢复

**文件：** `internal/worktree/{git,recover,manager}.go`、测试；**依赖：** T1。封装非交互 Git；实现创建、列表、进入、退出、删除及 `.git`/`HEAD` 快速恢复。

**验证：** 临时 Git 仓库中创建、恢复、删除和会话退出测试通过。

## T3：初始化、保护与清理

**文件：** `internal/worktree/{setup,cleanup,session}.go`、测试；**依赖：** T2。实现尽力初始化、Session 持久化、变更保护、自动及过期清理。

**验证：** 初始化失败不破坏目录；有改动拒删；过期清理只删安全目录。

## T4：独立 Workspace 工具注册

**文件：** `internal/tools/registry.go`、测试；**依赖：** T1。按 Workspace 创建内置工具 registry，保持主工作区权限与 schema。

**验证：** 两个 Workspace 的同名文件与命令 cwd 相互隔离。

## T5：隔离定义与 Agent 参数

**文件：** `internal/subagent/{definition,discover}.go`、`internal/tools/agent.go`、测试；**依赖：** T1。解析校验 `isolation: worktree`，支持调用参数。

**验证：** 定义加载、非法值和兼容路径测试通过。

## T6：SubAgent Runtime 接入

**文件：** `internal/agent/{subagent,runner,scheduler}.go`、测试；**依赖：** T2-T5。Fork 强制隔离；定义式按声明隔离；注入路径说明；终态清理；阻止未隔离任务转后台。

**验证：** Fork cwd、清理与保留，以及既有 Fork 语义测试通过。

## T7：命令、配置与启动组装

**文件：** 命令、配置和 `cmd/mewcode/main.go`；**依赖：** T2、T3。增加 `/worktree`、配置默认值、启动恢复及安全日志。

**验证：** 命令、配置和组装测试通过。

## T8：文档与回归

**文件：** README、示例配置、`docs/README.md`、本章文档；**依赖：** T6、T7。同步用户说明、手工场景并执行回归。

**验证：** 文档与实现一致，项目测试通过。

## 执行顺序

```text
T1 → T2 → T3
 └→ T4 → T6 → T8
T1 → T5 ────↗
T2 + T3 → T7 → T8
```

## 完成记录

- [x] T1：名称校验、受控路径和数据模型已完成。
- [x] T2：Git 生命周期、只读快速恢复、会话进入/退出与恢复已完成。
- [x] T3：本地文件、依赖链接、`.worktreeinclude`、删除保护和过期清理已完成。
- [x] T4：工具注册表可按显式 Workspace 重建，并保留非本地工具。
- [x] T5：定义 frontmatter 与调用参数支持 `isolation: worktree`。
- [x] T6：Fork 强制隔离；隔离子任务完成后自动清理干净目录，保留有改动目录并回传路径和分支。
- [x] T7：`/worktree`、配置、启动恢复与生命周期安全日志已接入。
- [x] T8：README、配置模板、章节索引和验收记录已同步；全量测试已执行。
