# 第八章摘要标签容错 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development`（推荐）或 `executing-plans` 按任务执行；步骤使用 `- [ ]` 跟踪。

**Goal:** 让真实 Provider 重复输出完整 summary 标签时，自动压缩仍能选定一个摘要并继续正常请求。

**Architecture:** `internal/context` 将摘要文本解析为带安全元数据的单一结果，并只采用最后一个完整候选块。`Runner` 用该结果重建历史，在日志中记录候选数量和是否采用最后一个块，绝不记录摘要正文或模型原始响应。

**Tech Stack:** Go、现有 Provider 流接口、标准库字符串扫描、既有 `internal/logging`。

## Global Constraints

- 历史中只写入一个摘要正文，不拼接多个块，不保留标签外文本。
- 缺少、空白、嵌套或未闭合 summary 仍应失败且不得替换原历史。
- 日志字段只能记录候选数量、选择标识、长度、阶段和状态；不得记录 prompt、摘要、工具结果或凭据。
- 仅修改摘要解析、调用处、测试与第八章 bug/documentation；不引入 Provider 专属 structured output。

---

### Task 1: 单一摘要解析与提示词约束

**Files:**
- Modify: `internal/context/compact.go:10-57`
- Test: `internal/context/context_test.go:135-177`

**Interfaces:**
- Produces: `type SummaryExtraction struct { Text string; CandidateCount int; UsedLast bool }`。
- Produces: `func ExtractSummary(text string) (SummaryExtraction, error)`。
- Consumes: `Manager.BuildSummaryRequest` 的现有 Provider-neutral 文本提示。

- [ ] **Step 1: 写失败测试**

将现有 `TestExtractSummaryRequiresExactlyNonEmptySummaryTag` 改为表驱动测试，至少覆盖：

```go
cases := []struct {
    name, input, want string
    candidates int
    usedLast bool
    wantErr bool
}{
    {"one", "<summary>keep</summary>", "keep", 1, false, false},
    {"two chooses last", "<summary>draft</summary>x<summary>final</summary>", "final", 2, true, false},
    {"outside text discarded", "prefix<summary>final</summary>suffix", "final", 1, false, false},
    {"missing", "plain", "", 0, false, true},
    {"empty", "<summary> </summary>", "", 0, false, true},
    {"nested", "<summary>a<summary>b</summary></summary>", "", 0, false, true},
    {"unclosed", "<summary>a", "", 0, false, true},
}
```

- [ ] **Step 2: 运行红测**

Run: `go test ./internal/context -run 'TestExtractSummaryRequiresExactlyNonEmptySummaryTag' -count=1`

Expected: FAIL，因为现有实现将两个完整块判为 `multiple summary tags`，且返回值没有元数据。

- [ ] **Step 3: 实现最小扫描器与提示词加强**

在 `compact.go` 定义：

```go
type SummaryExtraction struct {
    Text           string
    CandidateCount int
    UsedLast       bool
}
```

按从左到右顺序扫描 `<summary>` 与其最近的 `</summary>`：每个候选正文 `TrimSpace` 后必须非空；正文中再次出现 `<summary>` 视为嵌套错误；找不到闭合标签视为未闭合错误。候选数为零时返回缺失错误；否则返回最后一个候选正文、候选数和 `CandidateCount > 1`。

将 `summarySystemPrompt` 的结尾强化为：

```text
Return no explanation, Markdown, analysis, or text outside the block. Do not emit a second <summary> tag and do not write the literal <summary> tag inside its contents.
```

保留现有九段摘要要求与无工具约束。

- [ ] **Step 4: 运行绿测与 context 回归**

Run: `go test ./internal/context -count=1`

Expected: PASS。

- [ ] **Step 5: 提交 Task 1**

```sh
git add internal/context/compact.go internal/context/context_test.go
git commit -m "fix: tolerate repeated summary blocks"
```

### Task 2: Runner 使用选定摘要并记录安全元数据

**Files:**
- Modify: `internal/agent/runner.go:329-345`
- Test: `internal/agent/runner_test.go:383-409,645-670`

**Interfaces:**
- Consumes: `contextmanager.ExtractSummary` 返回的 `SummaryExtraction`。
- Produces: 成功自动压缩后的共享 history 只含最后一个摘要正文；日志带 `summary_candidates`、`summary_used_last` 和 `summary_chars`。

- [ ] **Step 1: 写失败测试**

新增 Runner 集成测试，脚本化摘要响应为：

```go
textRound("<summary>draft state</summary><summary>final state</summary>", provider.Usage{InputTokens: 30})
```

断言：任务完成；后续正常请求含 `final state` 且不含 `draft state`；压缩事件为 automatic。扩展 `TestRunnerLogsContextCompactionLifecycle`，断言完成日志含：

```go
map[string]any{
    "summary_candidates": float64(2),
    "summary_used_last": true,
    "summary_chars": float64(len("final state")),
}
```

并继续断言序列化日志不含 `draft state`、`final state` 或原 history 文本。

- [ ] **Step 2: 运行红测**

Run: `go test ./internal/agent -run 'MultipleSummary|LogsContextCompactionLifecycle' -count=1`

Expected: FAIL，因为当前自动压缩因多个标签失败，也没有所需日志字段。

- [ ] **Step 3: 实现调用与日志字段**

将 Runner 中的调用改为：

```go
extraction, err := contextmanager.ExtractSummary(messageText(round.Assistant))
```

用 `extraction.Text` 调用 `manager.Rebuild`；完成日志添加：

```go
"summary_candidates": extraction.CandidateCount,
"summary_used_last": extraction.UsedLast,
"summary_chars": len(extraction.Text),
```

保持失败路径不替换 history；不把 `messageText(round.Assistant)` 写入日志、事件或错误文本。

- [ ] **Step 4: 运行目标测试与回归**

Run: `go test ./internal/context ./internal/agent -count=1`

Expected: PASS。

- [ ] **Step 5: 提交 Task 2**

```sh
git add internal/agent/runner.go internal/agent/runner_test.go
git commit -m "fix: retain final repeated summary block"
```

### Task 3: 同步第八章记录与完整验证

**Files:**
- Modify: `bugs/2026-08-11/003-ch08-summary-multiple-tags.md`
- Modify: `docs/ch08/checklist.md`

**Interfaces:**
- Consumes: Task 1–2 的测试与日志字段。
- Produces: bug 记录包含修复方式、验证命令和最终状态；第八章 checklist 有摘要多标签回归验证记录。

- [ ] **Step 1: 更新记录**

将 003 状态改为已修复，写明“最后一个完整 summary 被保留；多个块及标签外文本不写 history；结构损坏仍失败”，并列出目标测试命令。

- [ ] **Step 2: 执行完整验证**

Run: `go test ./...`

Expected: PASS。

Run: `go test -race ./internal/context ./internal/agent`

Expected: PASS，无 data race。

- [ ] **Step 3: 提交 Task 3**

```sh
git add bugs/2026-08-11/003-ch08-summary-multiple-tags.md docs/ch08/checklist.md
git commit -m "docs: verify summary tag tolerance"
```

## 自检

- 设计中的单一 history 摘要、最后候选选择、结构异常失败与安全日志均由 Task 1–2 覆盖。
- 不涉及 Provider 专属 API、额外修复请求或多个摘要拼接。
- 所有代码步骤含准确文件、测试、运行命令和预期结果。
