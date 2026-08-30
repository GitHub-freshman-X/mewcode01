# 状态诊断：缓存命中率人工测试方案

## 目标与范围

本方案只确认本章新增的用户可见缓存统计：新会话的未知状态、真实 Provider 请求后的累计统计，以及重启恢复后的连续性。Provider 是否在某一具体请求上产生缓存命中受模型、最小缓存长度和服务端缓存生命周期影响；本方案不把 `0%` 本身判为失败。

## 安全约束

- 只在 `/private/tmp` 创建的独立 fixture 和隔离的 `HOME` 下运行，不使用真实项目或日常用户配置目录。
- 使用专用、限额的真实 API 凭据；不要在终端记录 API key、完整 Provider 配置、完整提示词、会话 JSONL 或日志正文。
- 输入只使用无敏感信息的短文本；本方案不授权文件写入、命令执行或工具调用。

## 准备

在项目根目录执行，`<real-config.yaml>` 替换为本机已验证、且仅供测试使用的 Provider 配置：

```sh
project_root=$(git rev-parse --show-toplevel)
fixture_root=$(mktemp -d /private/tmp/mewcode-cache-manual.XXXXXX)
test_home=$(mktemp -d /private/tmp/mewcode-cache-home.XXXXXX)
binary=/private/tmp/mewcode-cache-manual

go -C "$project_root" build -o "$binary" ./cmd/mewcode
cd "$fixture_root"
HOME="$test_home" "$binary" --config <real-config.yaml>
```

## 场景 A：新会话与真实请求后的状态栏、`/status`

启动后立刻输入：

```text
/status
```

通过条件：状态栏显示 `缓存：—`；`/status` 显示“缓存读取：0”“缓存写入：0”“缓存命中率：—”。这表示当前会话尚无可计算的 Provider 用量，并非缓存命中为 0%。

随后依次发送两条普通消息，并等待每条回复完成：

```text
只回复：CACHE-MANUAL-ONE
只回复：CACHE-MANUAL-TWO
```

再次输入 `/status`。通过条件：

- 状态栏显示 `缓存：NN%`，而非 `缓存：—`；`NN` 可以为 `0`。
- `/status` 的“缓存读取”“缓存写入”均为非负整数，缓存命中率与状态栏一致。
- 不显示完整请求、回复、API key 或 Provider 原始响应。

若显示 `0%`，记录“本次真实请求未命中缓存”；不要通过重复请求或修改缓存断点来人为追求命中。是否命中由实际 Provider usage 决定。

## 场景 B：重启并恢复会话后的累计值

在场景 A 的 `/status` 输出中记录会话 ID、缓存读取、缓存写入和缓存命中率三个数字（不要记录正文），随后输入：

```text
/exit
```

以同一 `fixture_root`、`test_home` 和配置重新启动程序，再输入：

```text
/session list
/session resume <场景 A 的会话 ID>
/status
```

通过条件：恢复后 `/status` 的缓存读取、缓存写入和缓存命中率与退出前一致，状态栏也显示相同百分比。若会话列表中未出现该 ID，记录为会话恢复问题，不要手工编辑 JSONL。

## 结果记录

| 场景 | 会话 ID | 缓存读取 | 缓存写入 | 命中率 | 结果 |
|---|---|---:|---:|---:|---|
| A |  |  |  |  |  |
| B |  |  |  |  |  |
