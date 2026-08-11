# 已持久化工具结果回读识别 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 修改 | `internal/context/context_test.go` | 以路径而非内容定义回读豁免，并覆盖截断和未登记路径。 |
| 修改 | `internal/context/results.go` | 将完整内容集合替换为规范绝对路径集合。 |
| 新建 | `bugs/2026-08-12/README.md` | 当天问题索引。 |
| 新建 | `bugs/2026-08-12/001-ch08-persisted-readback-content-copy.md` | 根因、修复、验证记录。 |
| 修改 | `bugs/README.md` | 增加当天索引。 |

## T1：先写回读路径识别测试

**文件：** `internal/context/context_test.go`

**依赖：** 无。

**步骤：**

1. 修改既有包装 `read_file` 测试：原始工具结果仍为 `123456`，但 `data.content` 改成截断的 `123`、`truncated:true`，`data.path` 使用 `persisted[0].Path`。
2. 断言调用 `PrepareResults` 后没有产生新 `Persistence`，且包装 JSON 原样保留。
3. 新增 `TestPrepareResultsRepersistsReadFileFromUnregisteredPath`：使用未登记的临时绝对路径，并让 `data.content` 等于已持久化的原始 `123456`；断言产生一条新 `Persistence`。
4. 运行：

```bash
go test ./internal/context -run 'TestPrepareResultsDoesNotRepersistWrappedReadFileContent|TestPrepareResultsRepersistsReadFileFromUnregisteredPath' -count=1
```

**预期：** 截断回读测试因旧的内容匹配实现发生失败；未登记路径测试也因旧实现按内容错误豁免而失败。

## T2：改为路径集合识别

**文件：** `internal/context/results.go`

**依赖：** T1。

**步骤：**

1. 将 `ResultStore.contents map[string]struct{}` 改为 `paths map[string]struct{}`，在 `NewResultStore` 初始化。
2. 在 `Persist` 成功写入文件后，使用 `filepath.Clean(path)` 将文件路径登记到 `paths`，删除完整结果内容登记。
3. 将 `isReadback` 改为解析 `read_file` 成功 JSON 的 `success` 和 `data.path`；对非成功、缺路径、JSON 无效或路径规范化失败返回 `false`。
4. 对路径使用 `filepath.Abs` 后 `filepath.Clean`，只在精确命中当前 `ResultStore.paths` 时返回 `true`。
5. 运行 T1 的测试命令，确认通过。

**验证：**

```bash
go test ./internal/context -run 'TestPrepareResultsDoesNotRepersistWrappedReadFileContent|TestPrepareResultsRepersistsReadFileFromUnregisteredPath' -count=1
```

## T3：记录 bug 并完成回归

**文件：** `bugs/2026-08-12/README.md`、`bugs/2026-08-12/001-ch08-persisted-readback-content-copy.md`、`bugs/README.md`

**依赖：** T2。

**步骤：**

1. 建立当天 `001` 问题索引，状态标为“已修复”。
2. 记录旧逻辑持有完整内容造成内存复制，并在截断回读时因内容不相等再次持久化；记录路径精确匹配的修复方案和测试结果。
3. 更新全局 bug 日期索引。
4. 执行格式化与回归：

```bash
gofmt -w internal/context/results.go internal/context/context_test.go
go test ./internal/context -count=1
go test ./internal/agent -run 'Context|Persist|Readback' -count=1
git diff --check
```

**预期：** 全部命令通过，且无空白错误。

## T4：提交修复

**文件：** T1–T3 列出的全部文件。

**依赖：** T3。

**步骤：**

1. 检查 `git status --short`，确认只暂存本任务文件，不包含用户已有改动。
2. 暂存本任务文件并提交：

```bash
git add internal/context/results.go internal/context/context_test.go bugs/README.md bugs/2026-08-12
git commit -m "fix: identify persisted result readback by path"
```

**验证：** `git show --stat --oneline HEAD` 仅显示本任务文件。

## 执行顺序

```text
T1 → T2 → T3 → T4
```
