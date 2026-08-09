# MewCode `.env` 自动加载 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 自动加载项目根 `.env`，保留系统环境优先级，并为加载和冲突提供安全日志。

**Architecture:** 新增独立 `internal/envfile` 包，仅解析并注入简单 `KEY=value` 值。`main` 在读取主配置前确定工作区、创建日志器并调用该包；所有事件以变量名、数量和状态记录，不传递变量值或原始文件行。

**Tech Stack:** Go 标准库、现有 `internal/logging`、Go 测试与 race detector。

## Global Constraints

- 只读取 `<工作区>/.env`，不搜索父目录或加载其他 dotenv 文件。
- 只支持空行、`#` 注释与 `KEY=value`；不执行 shell、插值或命令。
- 系统环境优先；冲突日志不包含变量值。
- `.env` 读取或解析失败不终止启动。

---

## 文件组织

```text
internal/envfile/
├── load.go       # 文件发现、逐行解析、优先级和结果统计
└── load_test.go  # 合法、冲突、缺失、格式错误和父目录隔离测试
cmd/mewcode/
├── main.go       # 启动顺序调整、日志事件与安全 stderr 诊断
└── main_test.go  # `.env` 在配置前加载且失败不阻断启动
docs/ch00/02-env-loading/
├── spec.md
├── plan.md
├── task.md
└── checklist.md
```

## 核心接口

```go
package envfile

type Result struct {
    Loaded  []string
    Skipped []string
    Found   bool
}

func Load(path string, lookup func(string) (string, bool), set func(string, string) error) (Result, error)
```

`Load` 只接收显式路径和环境读写函数，便于测试。返回的变量名用于日志，调用方绝不记录对应值；文件缺失返回 `Found=false` 与 nil error；错误仅携带安全的行号和类别。

## 模块交互

```text
main
  → os.Getwd() 得到工作区
  → logging.New(workspace)
  → envfile.Load(workspace/.env, os.LookupEnv, os.Setenv)
  → log dotenv_not_found / dotenv_loaded / dotenv_variable_skipped / dotenv_load_failed
  → config.Load
  → MCP ExpandServer(os.LookupEnv)
```

## 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 解析 | 自建最小解析器 | 避免依赖与 shell 执行风险，行为边界明确。 |
| 优先级 | `lookup` 已存在即跳过 | 保持调用者/系统显式环境优先。 |
| 错误 | 缺失文件正常；读写/格式失败可诊断后继续 | 不破坏既有启动路径。 |
| 日志 | 每次加载事件仅含状态、数量、变量名 | 支持排障而不泄露秘密。 |
| 启动顺序 | 工作区和日志前置到主配置加载前 | `.env` 对配置和 MCP 展开均生效。 |

## 规格覆盖自检

| Spec | 归属 |
|---|---|
| F1–F3 | `internal/envfile` 路径、解析与优先级测试 |
| F4 | `main` 启动顺序与 MCP 集成测试 |
| F5 | `main` 的安全日志事件 |
| F6、N2 | 缺失/失败降级测试 |
| N1、N3 | 无 shell 执行的解析器与临时目录测试 |

