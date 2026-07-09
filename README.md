# MewCode

MewCode 是一个用 Go 1.25 构建的全屏终端 Agent。它支持 Anthropic Messages API、OpenAI Responses API、流式输出、多轮上下文、工具调用和 Claude Extended Thinking。

## Agent Loop

普通输入会启动 Agent Loop：模型可连续读取、搜索、修改文件并执行命令，直到返回最终回答。循环在最终回答、用户取消、模型流错误、连续 3 次未知工具，或达到 `agent.max_iterations` 时停止；迭代上限默认 20。

- `/plan <任务>`：仅开放读文件、找文件和搜代码；成功产出的计划按顺序追加到当前会话的待执行列表，不会覆盖已有计划。
- `/do`：把全部待执行计划按追加顺序交给模型并恢复完整工具。计划之间存在重复或冲突时由模型结合工作区自行判断，不会预先丢弃任何计划。

计划不会跨进程持久化。`/do` 正常完成后清空本次执行的计划；取消、失败、达到迭代上限或未知工具停止时保留全部待执行计划，之后可以再次执行。没有待执行计划时，`/do` 不会调用模型。

Plan Mode 的只读提示、探索回复和工具结果仅保存在当前规划任务的临时上下文中，不会进入后续 `/do` 的模型历史；TUI 仍会保留并展示最终计划。`/do` 会获得写文件、编辑文件和执行命令等完整工具能力。

## 构建与运行

```sh
go build -o mewcode ./cmd/mewcode
./mewcode
./mewcode --config /path/to/config.yaml
```

默认配置路径：macOS 为 `~/Library/Application Support/mewcode/config.yaml`，Linux 通常为 `~/.config/mewcode/config.yaml`，Windows 为 `%AppData%\mewcode\config.yaml`。实际位置遵循操作系统用户配置目录。

复制 `config.example.yaml` 后设置 `protocol`、`model`、`base_url`、`api_key`。`max_tokens` 可省略，默认 4096；`agent.max_iterations` 可省略，默认 20。Anthropic 可启用 `thinking`；`budget_tokens` 至少为 1024，并且必须小于 `max_tokens`。OpenAI 不支持该配置。

## 操作

- `Enter`：发送消息
- `/plan <任务>`：只读探索并追加待执行计划
- `/do`：执行当前会话的全部待执行计划
- `Page Up` / `Page Down`：滚动历史
- `Ctrl+T`：展开或折叠 thinking
- `Ctrl+C`：生成中取消当前回复；空闲时退出

## API Key 安全

API Key 以明文保存在 YAML 中。请限制配置文件权限（例如 Unix 上使用 `chmod 600`），不要把配置或密钥提交到版本控制。错误信息和界面不会主动显示 `api_key` 的值。
