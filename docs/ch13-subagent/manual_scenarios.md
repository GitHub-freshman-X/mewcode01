# SubAgent 人工验证场景

> 需要有效 Provider 配置。除明确说明外，在临时项目目录中执行，避免修改真实项目。

## 准备

1. 构建：`go build -o /tmp/mewcode-ch13 ./cmd/mewcode`。
2. 复制可用配置到当前平台的用户配置目录；确认 `agent.enable_verification_agent: false`。
3. 创建临时项目的 `.mewcode/agents/`，并放入一个包含 `name`、`description` 和正文的角色定义。

## 定义覆盖

1. 分别在 `<用户配置目录>/mewcode/agents/` 与项目 `.mewcode/agents/` 创建同名定义，但正文写入不同识别文本。
2. 启动 MewCode，要求主 Agent 委派该角色。
3. 预期：项目级定义优先；删除项目文件并重启后用户级定义生效。

## Verification 开关

1. 保持 `agent.enable_verification_agent: false` 启动，确认不可使用 Verification。
2. 设为 true 后重启。
3. 预期：Verification 可被委派；其默认角色为只读验证。

## 显式后台与 Fork

1. 要求主 Agent 委派一个带 `run_in_background: true` 的定义式任务。
2. 再要求它不指定类型委派一个 Fork 任务。
3. 预期：两次调用立即返回任务 ID；Fork 不阻塞主会话；完成后 TUI 显示安全摘要通知，下一轮主 Agent 请求可获知通知。

## ESC 转后台

1. 请求一个会进行多次工具调用的定义式子 Agent。
2. TUI 显示“子 Agent 前台运行”后按 ESC。
3. 预期：显示“已转入后台”，任务继续执行；完成、失败或取消时显示终态通知。Ctrl+C 仍取消主任务。

## 边界

1. 请求 Agent 工具使用 `isolation: worktree`。
2. 预期：收到“worktree isolation is not supported in this chapter”之类的明确错误，未创建 Worktree。
3. 要求 Fork 子 Agent 再 Fork，或后台任务创建 Agent。
4. 预期：调用被拒绝，且主 Agent 保持可用。
