# 用户级权限 YAML 格式阻断启动，MCP 配置另有独立诊断

## 状态

权限 YAML 格式问题已修正；MCP 诊断仍待处理。

## 用户可见现象

执行 `./mewcode --config .mewcode/config.yaml` 后报出：

- 用户级 `permissions.yaml` 在 YAML scanner 解析阶段失败；
- `filesystem` MCP 初始化时会话关闭；
- Microsoft Learn MCP 关闭时返回 HTTP 405。

## 根因

`/Users/xuchangan/Library/Application Support/mewcode/permissions.yaml` 的第 1、4 行写为 `":deny`，未在 YAML 键值分隔符后加入空格。`go-yaml/v4` 严格解析该文件，最终在第 3 行报告 scanner 错误。

MCP 两条信息与权限 YAML 无直接因果关系：`filesystem` 表示 stdio 子进程在初始化前退出；Microsoft Learn 端点为初始化会话分配了 session，但拒绝客户端关闭时发送的 `DELETE`。

## 修复方案

已将用户级权限文件中所有 `":deny` 改为 `": deny`。未修改 MCP 配置或传输实现。

## 验证

- `ruby -e 'require "yaml"; YAML.load_file(ARGV[0])' <permissions-path>`：通过。
- `go test ./internal/permissions ./cmd/mewcode`：通过。
- `go test ./internal/mcp ./internal/permissions ./cmd/mewcode` 在受限沙箱中无法启动 `httptest` 监听端口，未作为 MCP 行为结论。

## 后续工作

在具备网络和 `MCP_TEST_ROOT` 环境变量的终端中启动后，查看 `filesystem` 子进程 stderr，确认 `npx`、包版本与目录参数；若不需要该服务器，可先从 `mcp_servers` 移除。Microsoft Learn 的 405 仅发生于关闭；若需消除该诊断，应确认该端点的 MCP 会话关闭能力，或在客户端中将不支持关闭的状态单独处理。
