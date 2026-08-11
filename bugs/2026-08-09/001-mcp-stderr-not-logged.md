# stdio MCP Server stderr 未进入应用日志

## 状态

日志模块问题已修复；真实 filesystem Server 的配置环境仍待处理。

## 现象

MCP 日志显示 `initialize_failed`，但没有 Server 的 stderr，无法定位 Server 在初始化前退出的原因。

## 根因

`StdioTransport` 原先固定创建 no-op logger，Manager 未注入带 Server 上下文的记录器；即使 stderr 被读取也不会落盘。初次修复后使用 `Scanner` 按行写入，导致多行 Node 堆栈被拆成多条日志；原脱敏正则也会让 `Authorization: Bearer <secret>` 中的 secret 遗留在日志中。

## 修复

Stdio transport 接收 Manager 传入的 Server-scoped logger。stderr 在子进程关闭该流后被整体读取、限长并作为单条日志记录；脱敏规则优先匹配 `Authorization: Bearer`，避免其凭据值泄漏。

## 验证

`go test -race ./internal/mcp` 通过；`TestStdioTransportLogsStderr` 验证多行 stderr 合并为一个事件，且 `Authorization: Bearer SECRET_VALUE` 不会写入日志。真实 filesystem Server 日志显示 Node 因缺少 `@modelcontextprotocol/sdk` 的 `completable.js` 在初始化前退出。

## 后续

已清除日志中标识的损坏 npx 临时安装目录并重新安装，缺失 `completable.js` 的错误不再出现。新的启动校验显示 `.env` 中的 `MCP_TEST_ROOT` 当前解析为 `/tmp/mewcode-test`，该目录不存在或不可访问；需创建并授权该目录，或将变量改为一个可访问的目录。运行环境为 Node 21.2.0，依赖声明支持 Node 18、20 或 22+，也应切换到受支持版本后再次验证。

## 2026-08-09 CI 诊断

GitHub 对提交 `64bae30` 的两次 `ci` 运行分别报告 macOS 与 Ubuntu 的 `test` 任务失败，Windows 任务因矩阵 fail-fast 被取消；邮件未包含失败步骤或断言。受限本地环境首次运行会因禁止 `httptest` 绑定回环端口而失败，该结果不代表项目缺陷。允许本地端口后，使用工作区内独立 Go 缓存执行 `go test ./...` 全部通过。当前未能读取 GitHub Actions 原始日志（GitHub API 与网页会话均无该私有仓库的读取授权），因此具体失败根因仍待通过有仓库访问权限的 GitHub Actions 日志确认。
