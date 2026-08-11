# 已持久化工具结果回读识别 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 通过已持久化文件的规范绝对路径识别 `read_file` 回读结果，避免大结果在内存中复制，并避免截断回读被再次持久化。

**Architecture:** `ResultStore` 由完整内容集合改为已创建文件路径集合。持久化成功时登记文件的规范绝对路径；结果预处理时解析 `read_file` 成功 JSON 的 `data.path`，只有路径精确命中当前会话集合时才跳过第一层压缩。

**Tech Stack:** Go 标准库（`encoding/json`、`path/filepath`）、现有 `provider.ToolResult`、Go 测试框架。

## Global Constraints

- 仅修改第八章第一层回读识别逻辑；不改变 `read_file` 的 512 KiB 文件限制和 64 KiB 输出限制。
- 仅信任当前 `ResultStore` 成功创建并登记的规范绝对路径，不接受任意 `.mew` 路径或目录前缀匹配。
- 不引入 MD5/SHA 或其他哈希；本次只识别文件来源。
- 保留 `PrepareResults` 的单项阈值、聚合预算、结果顺序与错误处理语义。
- Bug 修复过程同步更新 `bugs/` 记录。

---

## 文件结构

| 文件 | 改动职责 |
|---|---|
| `internal/context/results.go` | 将 `ResultStore` 的回读识别索引改为路径集合，并通过结构化 `read_file` 路径判断回读。 |
| `internal/context/context_test.go` | 覆盖持久化路径命中、截断内容路径命中和未登记路径不豁免。 |
| `bugs/2026-08-12/README.md` | 建立当天问题索引。 |
| `bugs/2026-08-12/001-ch08-persisted-readback-content-copy.md` | 记录根因、修复和验证结果。 |
| `bugs/README.md` | 增加当天索引。 |

### Task 1: 路径索引回读识别

**Files:**
- Modify: `internal/context/results.go:19-43,164-180`
- Modify: `internal/context/context_test.go:95-134`
- Create: `bugs/2026-08-12/README.md`
- Create: `bugs/2026-08-12/001-ch08-persisted-readback-content-copy.md`
- Modify: `bugs/README.md`

**Interfaces:**
- Consumes: `provider.ToolResult{Name, Content}`；`tools.Success` 编码的 `read_file` JSON 中的 `success` 与 `data.path`。
- Produces: `Manager.PrepareResults` 对当前 `ResultStore` 创建路径的 `read_file` 结果不再次调用 `Persist`。

- [ ] **Step 1: 写入失败测试，定义路径命中而非内容命中的行为。**

在 `TestPrepareResultsDoesNotRepersistWrappedReadFileContent` 中使用一个与已存盘文件内容不同的截断 `content`，但保留 `persisted[0].Path`：

```go
wrapped := `{"tool_name":"read_file","success":true,"data":{"path":"` + persisted[0].Path + `","content":"123","truncated":true}}`
results, again, err := m.PrepareResults([]provider.ToolResult{{CallID: "b", Name: "read_file", Content: wrapped}})
if err != nil || len(again) != 0 || results[0].Content != wrapped {
	 t.Fatalf("results=%+v persisted=%v err=%v", results, again, err)
}
```

新增未登记路径场景，构造 `read_file` 成功 JSON 并令内容超过 `SingleResultChars`，断言该结果会产生一次新的持久化记录：

```go
wrapped := `{"tool_name":"read_file","success":true,"data":{"path":"` + filepath.Join(t.TempDir(), "other.txt") + `","content":"123456","truncated":false}}`
_, again, err := m.PrepareResults([]provider.ToolResult{{CallID: "b", Name: "read_file", Content: wrapped}})
if err != nil || len(again) != 1 {
	 t.Fatalf("persisted=%v err=%v", again, err)
}
```

- [ ] **Step 2: 运行测试并确认它因旧内容匹配逻辑失败。**

Run: `go test ./internal/context -run 'TestPrepareResultsDoesNotRepersistWrappedReadFileContent|TestPrepareResultsRepersistsReadFileFromUnregisteredPath' -count=1`

Expected: 截断内容的回读测试失败，并显示产生了额外的持久化记录；未登记路径测试在实现前应通过或作为保护性基线。

- [ ] **Step 3: 以最小代码替换内容集合为路径集合。**

将 `ResultStore` 字段改为：

```go
paths map[string]struct{}
```

在 `NewResultStore` 初始化 `paths`。在 `Persist` 的 `os.WriteFile` 成功后登记：

```go
s.paths[filepath.Clean(path)] = struct{}{}
```

将 `isReadback` 的 JSON 解析结构扩展为：

```go
var wrapped struct {
	Success bool `json:"success"`
	Data struct {
		Path string `json:"path"`
	} `json:"data"`
}
```

仅在 `Success == true` 且 `Data.Path` 非空时，通过 `filepath.Abs` 和 `filepath.Clean` 得到路径，并查询 `m.Store.paths`；解析/规范化失败或未命中时返回 `false`。

- [ ] **Step 4: 运行 Context 包测试确认通过。**

Run: `go test ./internal/context -count=1`

Expected: PASS。

- [ ] **Step 5: 更新 bug 记录。**

在 `bugs/2026-08-12/README.md` 建立编号 `001` 索引；在问题记录中写明：旧实现以完整内容为 map key 导致内存复制且截断回读不能命中，修复后以当前会话创建的规范绝对路径精确匹配；记录步骤 4 的实际测试命令和结果。更新 `bugs/README.md` 的日期索引。

- [ ] **Step 6: 运行目标回归、静态格式检查并提交。**

Run: `gofmt -w internal/context/results.go internal/context/context_test.go && go test ./internal/context -count=1 && go test ./internal/agent -run 'Context|Persist|Readback' -count=1 && git diff --check`

Expected: 全部通过，且 diff 无空白错误。

```bash
git add internal/context/results.go internal/context/context_test.go bugs/README.md bugs/2026-08-12
git commit -m "fix: identify persisted result readback by path"
```

## 验收映射

| 要求 | 对应步骤 |
|---|---|
| 不再在 `ResultStore` 保存完整内容 | Task 1，步骤 3；字段改为 `paths`。 |
| 已持久化文件正常回读不二次持久化 | Task 1，步骤 1、4。 |
| 截断回读仍不二次持久化 | Task 1，步骤 1、2、4。 |
| 未登记文件不能获得豁免 | Task 1，步骤 1、4。 |
| Bug 记录同步 | Task 1，步骤 5。 |

## 自检

- 设计说明的三个测试要求都映射到 Task 1。
- 所有新增/修改文件、函数行为与测试命令均已明确。
- 计划不改变 `read_file` 的业务限制，未引入哈希或目录前缀信任。
