# SWE-bench-Live 评测暴露：平台相关代码验证盲区与自写测试假信心

## 状态

部分修复（2026-08-28）。根因 3（工具 30 秒硬超时）已修复；根因 1、2（平台验证盲区、自写测试假信心）待处理。

评测来源：2026-08-28 SWE-bench-Live go split 10 题端到端评测；8 题环境有效，resolved 3/8。

## 影响范围

- 非交互任务执行（`mewcode run`）中涉及平台相关代码（`//go:build linux` 等）的修复质量验证。
- Agent 对"验证通过"的判定可靠性：凡 Agent 自己写/改测试再跑该测试，通过与否均不构成对官方契约的验证。

## 用户可见现象

同一根因导致 4 道 Go 题在官方评测中失败（补丁源码本身方向正确，但与官方测试的 API/常量/错误文案契约不一致）：

1. `moonD4rk__HackBrowserData-571`：Agent 在 macOS 上跑 `go test ./crypto` 验证 Linux 解密修复；`//go:build linux` 文件未参与本地编译，官方测试 `crypto_linux_test.go` 引用的常量 `kEmptyKey` 未定义，容器内 `crypto` 包 build failed。
2. `github__gh-aw-26689`：官方测试要求 `mergeMCPToolUsageInfo` 符号及 `deriveRunAgenticAnalysis` 的特定参数表；Agent 用自己的内联设计实现并通过自己改写过的测试。
3. `nextlevelbuilder__goclaw-841`：官方测试调用 `*Channel.filterComment`；Agent 以 feature-gate 路由实现，方法名不同。
4. `kubernetes__kube-state-metrics-2926`：官方测试期望字段 `Options.CustomResourceConfigFileDeprecated`；Agent 只恢复了 YAML tag，未加兼容字段。
5. `github__gh-aw-27242`（非编译类）：官方测试要求错误文案 `ERR_VALIDATION: Repository '...' is not in the allowed-repos list`；Agent 自造 `E004: INVALID_TARGET_REPO` 并用自己改写的测试文件验证（5 passing），官方替换测试文件后 2 个目标用例失败。

## 复现条件

按 `swe-bench-live-2/` 目录内本次评测流程（gold 校验通过的 8 题 + `logs/eval/*/report.json`）可复现上述 5 题的失败。

## 根因

1. MewCode 缺少跨平台验证意识：在 macOS 主机上修改带 `//go:build linux` 约束的代码后，`go build/test` 默认本地 GOOS，Linux 路径完全未编译，形成验证盲区。
2. 非交互任务的系统指令未引导 Agent"从仓库既有调用方/错误约定推断官方契约"，而是以自写测试自证；自写测试通过即宣布完成。
3. 工具级 30 秒超时导致大型包（如 `pkg/cli` 全量测试、sqlc TestReplay）无法完整运行，Agent 转而依赖聚焦测试，放大了上述盲区。

## 修复方案

已修复（工具超时部分）：

- `internal/tools/run_command.go`：`timeout_ms` 上限（`MaxTimeout`）从 30s 提到 600s；未显式传 `timeout_ms` 时默认仍为 30s（防挂死命令），长测试命令由模型显式申请更大值；工具描述注明该语义。
- `internal/tools/executor.go` + `cmd/mewcode/main.go`：工具执行器外层安全网同步从 30s 提到 600s，否则外层会先于工具内层超时截断长命令。
- 新增 `TestRunCommandTimeoutSchema`：校验 schema 上限 600000ms、300000ms 合法、600001ms 被拒。

待定（模型行为部分，候选方向）：

- 非交互/任务系统指令中加入：修改平台相关代码时必须用 `GOOS=<目标平台> go build/vet/test` 交叉验证。
- 引导 Agent 优先复用仓库既有错误类型/命名约定（如先查 `validateTargetRepo` 的调用方如何抛错），而非发明新契约。
- 完成判定不得仅依赖 Agent 本轮自写/自改的测试；至少要求编译期契约（引用既有符号）或交叉验证。

## 验证方式

- `go build ./...`、`go test ./...` 全部通过，`go vet ./...` 干净（2026-08-28）。
- 端到端：重跑 SWE-bench-Live 同一 8 题（`swe-bench-live-2/logs/` 中脚本与数据可复用），观察上述题目 resolved 变化。实测参考：sqlc `TestReplay` 宿主机需 109s、gh-aw `pkg/cli` 完整包容器内 184s，均在 600s 上限内。

## 后续工作

- 评测产物与逐题复盘表见 `swe-bench-live-2/logs/`（predictions.json、logs/eval/*/report.json、logs/<instance_id>.jsonl 运行结果、work/<instance_id>/ 内 Agent 运行时日志）。
- 其余可疑行为（见复盘表）：结束前留下"顺手格式化"改动并等待确认（gh-aw-26074）、两次在自写测试中引入语法错误后自愈（kube-state-metrics、HackBrowserData）。
