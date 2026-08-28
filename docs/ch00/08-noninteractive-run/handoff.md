# SWE-bench-Live 测试交接

## 下一会话目标

使用 MewCode 的非交互式单任务入口，开始准备或执行 SWE-bench-Live 的单题测试；Codex 负责外层调度与评测。本次交接只覆盖入口的使用与当前验证状态，不包含题库下载、容器编排、批量调度或判分实现。

## 当前状态

- 工作分支：`codex/ch00-noninteractive-run`；非交互入口已提交为 `e1cfd58 feat:非交互式单任务`。本交接文档本身尚未提交。
- `mewcode run` 已实现，相关设计与验收记录见 [spec.md](spec.md)、[plan.md](plan.md)、[task.md](task.md)、[checklist.md](checklist.md)。
- 人工验证步骤见 [manual_scenarios.md](manual_scenarios.md)。
- 已通过离线全量回归：`go test ./...`；`git diff --check` 通过。真实模型、MCP 和真实中断信号的端到端验证尚未执行。

## 非交互式使用

在**待解题仓库的根目录**运行。当前目录就是 Agent 的工作区，不提供 `--workspace` 参数。

```sh
# 构建当前分支的二进制
go build -o /private/tmp/mewcode ./cmd/mewcode

# 短任务文本
/private/tmp/mewcode run --config /path/to/config.yaml \
  --prompt '根据仓库中的 issue 描述修复问题，并运行相关测试。'

# 题目内容写入文件；适合基准调用方生成任务文件
/private/tmp/mewcode run --config /path/to/config.yaml \
  --prompt-file /path/to/task.txt --json > /path/to/result.json
```

任务输入必须且只能提供 `--prompt` 或 `--prompt-file` 之一；空内容、缺失输入或双输入会在模型请求前失败。

默认模式将模型文本增量写到 stdout；诊断写到 stderr。`--json` 模式不会输出中间文本，结束时 stdout 为单个 JSON，字段包含：`status`、`stop_reason`、`error`、`final_text`、`elapsed_ms`、`iterations`、`usage`。

总超时默认 30 分钟；可按单题预算调整，或显式关闭：

```sh
/private/tmp/mewcode run --config /path/to/config.yaml \
  --prompt-file /path/to/task.txt --timeout 30m --json

# 仅在外层控制器负责超时时使用
/private/tmp/mewcode run --config /path/to/config.yaml \
  --prompt-file /path/to/task.txt --timeout 0 --json
```

退出码：`0` 正常完成；`1` 参数、启动或运行失败；`2` 取消；`3` 总超时；`4` 达到迭代或未知工具安全停止。

## 基准运行时的重要边界

- 当前调用会加载常规配置、项目/用户指令、记忆索引、Skill、Hook、MCP 和权限规则。
- 调用使用临时会话：不会创建可恢复会话，也不会写入长期记忆；安全元数据日志仍按现有规则产生。
- 未被规则允许、原本需要人工确认的工具调用会自动作为“拒绝”返回给 Agent，绝不等待输入。因此若题目必须修改或执行命令，应在专用测试 fixture 的项目权限规则中显式且最小化地预授权需要的操作。
- 前台子 Agent 可以完成；显式后台与 Fork 子 Agent 被拒绝，避免主进程退出后遗留任务。
- `run` 只执行一条普通动作任务（`ModeAct`），不支持 `plan`、`do`、`compact` 或交互式会话管理。

## 建议的外层职责

由 Codex 或其他外层控制器负责每题的隔离工作目录、任务文件、进程超时、stdout JSON 收集、Git diff 提取和官方测试/判分。不要把这些职责实现进 `mewcode run`，除非新会话明确开启单独的功能需求。

## 建议使用的 Skills

- `mew-spec`：若要新增 SWE-bench-Live 下载、容器运行、批量调度或评分能力，先按该流程建立独立章节规格。
- `diagnosing-bugs`：若非交互入口在真实题目中失败、阻塞或输出不符合契约时，用于定位问题。
- `defuddle`：若下一会话需要阅读用户提供的网页形式的基准文档，可用它提取正文；官方仓库 README 则优先直接查看本地 clone 或官方原始文件。
