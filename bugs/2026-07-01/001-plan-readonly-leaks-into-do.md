# 001 Plan Mode 只读约束疑似污染 `/do`

## 状态

已修复（2026-07-02）

## 用户可见现象

连续规划“创建 `hello.txt` 并写入 hello world”和“把 world 改为 changan”后执行 `/do`，模型回复当前环境只有只读权限，未调用写工具。

## 影响范围

同一 Session 中先执行一个或多个 `/plan`、随后执行 `/do` 的工作流。普通 Act Mode 暂无证据受影响。

## 当前证据

- `ModeDo` 使用完整 Registry，理论上会向 Provider 声明六个工具，包括 `write_file`、`edit_file` 和 `run_command`。
- 每个成功的 Plan Mode 完整轮次都会提交到 Session 历史。
- Plan Mode 首轮 user 消息包含“read-only tools only”及“do not modify files or run commands”。
- `/do` 提示要求使用 available tools，但没有明确说明此前只读限制已解除、当前允许写入和执行命令。
- 定向测试 `TestPlanAppendsInOrderAndDoConsumesPlansOnSuccess` 与 `TestDoPromptContainsAllPlans` 通过，确认多个计划均进入 `/do` 请求且该请求声明六个工具。

## 高概率根因

模型在 `/do` 请求中同时看到了历史里的只读规划指令和当前执行指令。虽然工具定义已经恢复完整，但当前提示没有显式覆盖旧约束，模型可能继续遵循历史中的只读语义并自行拒绝写操作。

## 下一步

1. 将 Plan Mode 改为任务内临时历史，禁止规划提示、探索响应和工具结果进入共享 Session。
2. 保留正常完成的最终计划追加逻辑，确保 `/do` 仍收到全部计划及完整工具。
3. 增加回归测试，复现“创建 hello.txt 后修改内容”的双计划执行流程。

## 选定修复方案

采用历史隔离，而不是仅在 `/do` 提示中声明“现在允许写入”。后者仍依赖模型正确覆盖历史指令；前者从上下文结构上消除权限语义污染。

技术设计将 Session 拆分为模型上下文历史与界面展示记录，并引入单次 Plan 任务临时历史。普通轮次同时写入两类历史；规划内部轮次只写临时历史；成功规划仅把最终计划写入展示记录和待执行列表。

Plan 自检已完成：旧的 `AppendPlan` 直接保存路径已从设计中移除；`BuildRound` 复用轮次校验，`taskHistory` 保证多轮规划连续性，`DisplaySnapshot` 保证隔离后 TUI 不丢失已完成计划。

任务拆解覆盖 Session 双历史、`BuildRound`、Plan 临时历史、Runner 模式分流、TUI 展示快照和 `hello.txt` 双计划回归，不需要修改 Provider 或工具权限实现。

Task 自检已完成：受影响文件、依赖与验证命令均明确；旧的 `AppendPlan`/`LatestPlan` 设计名未残留；回归场景明确要求最终文件内容为 `hello changan`。

Checklist 新增模型/展示双历史隔离、Plan 临时多轮、`/do` 污染扫描、TUI 持久展示和 `hello changan` 端到端条目；全量质量项已重置为待验证。首次补丁因既有文案不匹配未应用，已使用实际文本重新更新。

Checklist 自检结果：相关新增行为均为未通过状态并带具体测试命令；当前 48 项既有证据保持通过、15 项等待本次修复及端到端验收。

实施前代码复核确认：`Runner` 每轮无条件读取并提交 `Session.Snapshot/CommitRound`，Plan 成功后另行 `AppendPlan`；TUI 也直接读取同一 Snapshot。这正是模型历史与展示职责耦合、只读提示泄漏的具体代码路径。

已实现 Session 层改造：新增 `DisplaySnapshot`，抽取 `BuildRound`，普通 `CommitRound` 同时写模型/展示历史，`CommitPlan` 仅写展示记录与待执行计划。对应隔离、深拷贝、原子消费和轮次校验测试已补充，待运行验证。

测试文件已补齐 `strings` 依赖，用于构造去除外围空白的最终计划展示内容。

Session 定向验证已通过：`TestSessionSnapshotIsolation`、`TestSessionCommitPlanAndHistoryIsolation`、`TestSessionPlanConsumption`、`TestBuildRoundValidation` 全部成功。

已新增 `taskHistory` 及多轮测试；Runner 现在按模式选择临时历史或 Session 模型历史，Plan 最终通过 `CommitPlan` 提交展示结果与待执行项。`preparedRequest` 同时保存内部规划提示和无只读约束的用户可见提示。

Runner 测试已迁移到 `CommitPlan`，并新增断言：Plan 第二轮能看到临时工具历史、Session 模型历史保持为空、`/do` 请求含六个工具且不含只读提示或规划工具结果。待运行验证。

Agent 定向验证已通过：`TestTaskHistoryMultiRound`、Plan/Do 生命周期测试、`TestPlanTaskHistoryMultiRoundAndDoExcludesPlanInternalHistory` 以及规划/执行四类非成功终态保留测试全部成功。

TUI 已切换为读取 `DisplaySnapshot`，并新增回归测试保证 Plan 最终结果持续可见，同时 Session 模型历史仍为空。待运行 TUI 验证。

TUI 定向验证已通过：部分输出/Usage 回归与 `TestDisplayHistoryShowsPlanWithoutModelHistory` 均成功。

已新增脚本化端到端回归 `TestPlanIsolationHelloChanganEndToEnd`：两次规划期间文件必须不存在，`/do` 请求不得含只读内部提示且必须声明写/改工具，最终文件必须为 `hello changan`。待运行验证。

脚本化端到端回归已通过：规划阶段未创建文件，`/do` 无只读提示污染并恢复 `write_file`/`edit_file`，最终磁盘内容为 `hello changan`，计划成功消费。

README 已补充 Plan 临时上下文隔离、TUI 最终计划展示及 `/do` 恢复完整工具能力的用户说明。

迁移扫描完成：生产代码无 `AppendPlan` 残留；TUI 生产代码不再读取 `Session.Snapshot`。Runner 对 Snapshot 的使用仅用于 Plan 临时历史基线与普通 Act/Do 模型上下文，符合设计。格式与 diff 检查通过。工作区另有用户文件 `现有问题.md`，本修复未触碰。

全量质量验证已通过：`go test -race ./...`、`go vet ./...`、`go build ./...` 均退出 0；Agent、Session、TUI 及两家 Provider 测试全部成功。

Checklist 复核发现非成功 Plan 测试此前只断言计划列表未变，现已补充 Session 模型历史为空的断言，并把 AC8 验证命令对齐实际测试名称；需重新运行相关测试。

补强后的 AC8/AC15/AC17/AC22 定向测试全部通过，包括成功规划、四类非成功规划、临时多轮历史、`/do` 污染排除和 `hello changan` 回归。Checklist 已按实际证据勾选自动化与脚本化端到端条目，仅保留三个尚未人工执行的通用 TUI E2E 场景。

Spec 自检已完成：无 TBD/TODO；F6、F13、F15 分别定义共享历史边界、规划临时历史和 `/do` 隔离要求，AC8、AC15、AC17、AC22 提供对应可观察验收。

## 验证方式

使用脚本化 Provider 复现两个计划，确认 `/do` 请求含写工具，模型会调用 `write_file`/`edit_file`，最终文件内容为 `hello changan`。

## 最终修复

- Session 分离模型上下文历史与 TUI 展示记录。
- Plan Mode 使用任务级临时历史完成多轮探索，不再提交内部提示、响应或工具结果到 Session 模型历史。
- 成功规划仅通过 `CommitPlan` 写入用户可见记录和待执行计划列表。
- TUI 改为读取 `DisplaySnapshot`，因此隔离模型历史不会让最终计划消失。
- `/do` 继续恢复全部六个工具，并只接收普通模型历史与最终计划。

## 最终验证证据

- `TestPlanTaskHistoryMultiRoundAndDoExcludesPlanInternalHistory`：临时多轮完整，`/do` 无只读历史污染且包含六个工具。
- `TestPlanIsolationHelloChanganEndToEnd`：规划阶段不写文件，执行阶段调用 `write_file`/`edit_file`，最终内容为 `hello changan`。
- `TestDisplayHistoryShowsPlanWithoutModelHistory`：最终计划在 TUI 可见，同时模型历史为空。
- `go test -race ./...`、`go vet ./...`、`go build ./...` 全部通过。
