# MewCode

MewCode 是一个用 Go 1.25 构建的全屏终端 AI 对话客户端。本章支持 Anthropic Messages API、OpenAI Responses API、流式输出、多轮上下文和 Claude Extended Thinking，不包含工具调用、文件操作或会话持久化。

## 构建与运行

```sh
go build -o mewcode ./cmd/mewcode
./mewcode
./mewcode --config /path/to/config.yaml
```

默认配置路径：macOS 为 `~/Library/Application Support/mewcode/config.yaml`，Linux 通常为 `~/.config/mewcode/config.yaml`，Windows 为 `%AppData%\mewcode\config.yaml`。实际位置遵循操作系统用户配置目录。

复制 `config.example.yaml` 后设置 `protocol`、`model`、`base_url`、`api_key`。`max_tokens` 可省略，默认 4096。Anthropic 可启用 `thinking`；`budget_tokens` 至少为 1024，并且必须小于 `max_tokens`。OpenAI 不支持该配置。

## 操作

- `Enter`：发送消息
- `Page Up` / `Page Down`：滚动历史
- `Ctrl+T`：展开或折叠 thinking
- `Ctrl+C`：生成中取消当前回复；空闲时退出

## API Key 安全

API Key 以明文保存在 YAML 中。请限制配置文件权限（例如 Unix 上使用 `chmod 600`），不要把配置或密钥提交到版本控制。错误信息和界面不会主动显示 `api_key` 的值。
