# MewCode `.env` 自动加载 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
|---|---|---|
| 新建 | `internal/envfile/load.go` | 安全 `.env` 解析与变量注入 |
| 新建 | `internal/envfile/load_test.go` | 解析、优先级、缺失和错误测试 |
| 修改 | `cmd/mewcode/main.go` | 启动顺序、日志事件和降级处理 |
| 修改 | `cmd/mewcode/main_test.go` | `.env` 在配置前加载的集成测试 |
| 新建 | `docs/ch00/02-env-loading/checklist.md` | 可观察验收清单 |

## T1：实现安全 `.env` 加载器

**文件：** `internal/envfile/load.go`、`internal/envfile/load_test.go`。

1. 写失败测试：临时 `.env` 含空行、注释和 `MCP_TEST_ROOT=/tmp/fixtures`，断言 `Load` 调用 setter、返回 `Found=true` 和 `Loaded=["MCP_TEST_ROOT"]`。
2. 运行 `go test ./internal/envfile -run TestLoadSetsMissingValues`，期望因 `Load` 未定义而失败。
3. 实现逐行扫描：仅接受变量名匹配 `[A-Za-z_][A-Za-z0-9_]*` 的首个 `=` 分隔行；保留等号后的原始值，不执行任何解释。
4. 写失败测试：lookup 返回已有值时 setter 不被调用，结果含 `Skipped` 变量名。
5. 实现系统环境优先逻辑。
6. 写失败测试：缺失文件返回 `Found=false` 且无错误；格式错误返回只包含行号的错误，不含原行或值。
7. 实现缺失文件和安全格式错误。
8. 运行 `go test -race ./internal/envfile`，期望通过。
9. 提交：`git add internal/envfile && git commit -m "feat: add dotenv loader"`。

**验证：** `go test -race ./internal/envfile`。

## T2：在启动前加载 `.env` 并记录安全事件

**文件：** `cmd/mewcode/main.go`、`cmd/mewcode/main_test.go`。

1. 写失败测试：在临时工作区创建 `.env`，替换 `loadConfig` 读取 `os.LookupEnv("MCP_TEST_ROOT")`，断言配置加载时已得到 `.env` 值。
2. 运行 `go test ./cmd/mewcode -run TestRunLoadsDotEnvBeforeConfig`，期望失败。
3. 将 `os.Getwd` 和日志初始化移动到主配置加载前；调用 `envfile.Load(filepath.Join(root, ".env"), os.LookupEnv, os.Setenv)`。
4. 对缺失文件记录 `dotenv_not_found`；加载成功记录 `dotenv_loaded`（计数）；每个冲突记录 `dotenv_variable_skipped`（变量名）；失败记录 `dotenv_load_failed`（安全状态），并向 stderr 输出安全诊断。
5. 写测试：系统预设值覆盖 `.env` 值，日志只含变量名和冲突状态，不含任一值。
6. 写测试：格式错误不阻断 `run`，stderr 和日志不含原始行或变量值。
7. 运行 `go test ./cmd/mewcode`，期望通过。
8. 提交：`git add cmd/mewcode && git commit -m "feat: load project dotenv at startup"`。

**验证：** `go test ./cmd/mewcode`。

## T3：回归与验收记录

**文件：** `docs/ch00/02-env-loading/checklist.md`。

1. 从 spec 创建可观察 checklist，覆盖 `.env` 解析、系统优先、日志脱敏、缺失/格式错误降级和 MCP 变量展开。
2. 运行 `go test -race ./internal/envfile ./internal/mcp/...`，期望通过。
3. 运行 `go test ./...`，期望通过。
4. 在临时工作区创建 `.env`，以 `${MCP_TEST_ROOT}` 配置 filesystem Server，启动 MewCode 并验证日志出现 dotenv 加载成功和 MCP 注册事件。
5. 在验收记录填入真实命令结果；手工场景无法执行时明确标注原因。
6. 提交：`git add docs/ch00/02-env-loading/checklist.md && git commit -m "docs: record dotenv verification"`。

**验证：** `go test -race ./internal/envfile ./internal/mcp/... && go test ./...`。

## 执行顺序

```text
T1 → T2 → T3
```
