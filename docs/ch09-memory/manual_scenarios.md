# 第九章真实 API 人工测试方案

## 目标与范围

本方案在真实启动 `mewcode`、连接真实 Provider 后，验证第九章的分层指令与索引注入、JSONL 会话存档、新会话默认行为、普通聊天与 `/plan`、`/do` 的自动记忆提取，以及惰性治理。

自动提取和治理都会调用真实模型。场景使用短文本和固定数量会话控制成本；损坏 JSONL、24 小时时间跨度、30 天清理、并发锁竞争与错误注入仍由自动化测试验收。

## 安全约束

- 只在独立 fixture 项目和独立用户配置目录中运行，绝不写入真实项目或真实用户目录。
- 从可用配置复制测试配置，只保留连接所需字段；不要提交、截图或记录 API key。
- 人工记录只保留文件名、计数、唯一标记和结论，不记录完整 Prompt、会话、记忆或日志正文。
- 后台记忆任务可能在最终回复后继续运行；每次提交任务后等待最多 15 秒，并通过文件系统确认结果。
- 权限确认仅允许 fixture 内只读操作；完成后删除所有 fixture。

## 准备 fixture

```sh
fixture_root=$(mktemp -d /private/tmp/mewcode-ch09-manual.XXXXXX)
user_config_root=$(mktemp -d /private/tmp/mewcode-ch09-user.XXXXXX)
mkdir -p "$fixture_root/.mewcode/memory" "$fixture_root/docs" "$user_config_root/mewcode/memory"

echo '用户级规则：回复使用中文；测试标记 USER-RULE-91c4。' > "$user_config_root/mewcode/MEWCODE.md"
printf '%s\n%s\n' '项目级规则：仅操作当前 fixture；测试标记 PROJECT-RULE-7b2e。' '@../docs/project-context.md' > "$fixture_root/.mewcode/MEWCODE.md"
echo '项目引用内容：PROJECT-INCLUDE-3fd8。' > "$fixture_root/docs/project-context.md"
echo '- [user-index](user-index.md) — USER-MEMORY-6ac1' > "$user_config_root/mewcode/memory/MEMORY.md"
echo '- [project-index](project-index.md) — PROJECT-MEMORY-e509' > "$fixture_root/.mewcode/memory/MEMORY.md"
echo 'REFERENCE-SOURCE-b7e2' > "$fixture_root/docs/reference.txt"
echo "fixture_root=$fixture_root"
echo "user_config_root=$user_config_root"
```

运行前应只有上面的指令、索引和引用文件，项目 `.mewcode/sessions/` 尚不存在：

```sh
find <fixture_root> <user_config_root> -maxdepth 5 -type f | sort
```

如未构建二进制，在项目根目录执行：

```sh
go build -o /Users/xuchangan/Project/mine/mew-agent/mew01/mewcode /Users/xuchangan/Project/mine/mew-agent/mew01/cmd/mewcode
```

## 测试配置与启动方式

从已有可用的真实 API 配置复制 `ch09-real.yaml`。保留 `protocol`、`model`、`base_url`、`api_key`，关闭 thinking；第九章没有新增 YAML 配置项。

第九章的用户级目录固定为当前平台 `os.UserConfigDir()/mewcode`。在 macOS 上，Go 的 `os.UserConfigDir()` 固定返回 `$HOME/Library/Application Support`，不采用 `XDG_CONFIG_HOME`；因此测试用户目录为：

```sh
test_user_mewcode="$HOME/Library/Application Support/mewcode"
```

将 fixture 中的 `mewcode/` 目录暂时放到 `$HOME/Library/Application Support/mewcode`（或在隔离 OS 用户账户中执行），再按以下方式启动：

```sh
cd <fixture_root>
/Users/xuchangan/Project/mine/mew-agent/mew01/mewcode --config <测试配置绝对路径>
```

若必须使用真实用户目录，测试结束后只删除本方案创建的 `MEWCODE.md`、`memory/` 与其内容，不能删除既有用户数据。退出并再次执行上述命令代表一个新会话，且应新增一个 `.mewcode/sessions/<YYYYMMDD-HHMMSS-xxxx>.jsonl`。

## 会话 A：首轮长期上下文注入

启动后输入：

```text
不调用工具。请列出你在开始本次任务前收到的所有唯一测试标记，并说明哪一项来自项目内引用文件。
```

通过：回答包含 `USER-RULE-91c4`、`PROJECT-RULE-7b2e`、`USER-MEMORY-6ac1`、`PROJECT-MEMORY-e509` 和 `PROJECT-INCLUDE-3fd8`，并正确识别项目引用。用户级内容应早于项目级内容；回答的自然语言顺序不作为唯一判据。

退出后检查：

```sh
find <fixture_root>/.mewcode/sessions -maxdepth 1 -name '*.jsonl' -type f -print
```

通过：恰有一个符合 ID 格式的 JSONL 文件。

## 会话 B：普通聊天存档与用户级记忆

重新启动，输入：

```text
请记住我的长期偏好：在 Go 代码中我偏好使用 any，而不是 interface{}。只回答“已记录”。不要调用工具。
```

等待最多 15 秒：

```sh
find <user_config_root>/mewcode/memory -maxdepth 1 -type f -print | sort
sed -n '1,120p' <user_config_root>/mewcode/memory/MEMORY.md
```

通过：出现至少一个新 Markdown 文件；其 frontmatter 有 `name`、`description`、`type`，类型为 `user` 或 `feedback`，并且 `MEMORY.md` 增加指向该文件的索引。

检查第二个会话 JSONL：

```sh
latest=$(find <fixture_root>/.mewcode/sessions -maxdepth 1 -name '*.jsonl' -type f | sort | tail -n 1)
sed -n '1,20p' "$latest"
```

通过：包含用户和助手逐行 JSON 记录；不包含厂商原始 HTTP 协议、API key 或 `REFERENCE-SOURCE-b7e2`。

## 会话 C：项目知识、参考资料与去重

新会话中输入：

```text
这是项目长期知识：本 fixture 的服务运行在 Go 1.23，所有项目代码应使用 gofmt。只回答“项目知识已记录”。不要调用工具。
```

等待后台任务后输入：

```text
请使用 read_file 读取 docs/reference.txt。读取后把其中的唯一标记作为长期项目参考资料记录；不要修改文件，只回答“参考资料已记录”。
```

等待后检查：

```sh
find <fixture_root>/.mewcode/memory -maxdepth 1 -type f -print | sort
sed -n '1,160p' <fixture_root>/.mewcode/memory/MEMORY.md
```

通过：项目级目录增加 `project` 或 `reference` 记忆，并有相应索引。不要展示记忆文件正文。

记下项目级 Markdown 文件数后，输入：

```text
更新同一条项目长期知识：本 fixture 使用 Go 1.23，提交前必须运行 gofmt 和 go test ./...。不要新建重复知识；只回答“已更新”。不要调用工具。
```

等待后重新统计：

```sh
find <fixture_root>/.mewcode/memory -maxdepth 1 -name '*.md' ! -name MEMORY.md | wc -l
```

通过：项目知识的文件数不增加，索引没有第二个语义相同的条目。模型采用不同但合理类别时记录实际类别；不得重复新建是必需条件。

## 会话 D：Plan、Do 与 JSONL

在同一进程内输入：

```text
/plan 制定一个只读计划：读取 docs/reference.txt，核对唯一标记，并说明如何将该参考资料用于后续开发。不要执行工具。
```

计划完成后输入：

```text
/do
```

等待 `/do` 和后台任务后退出，检查最新 JSONL：

```sh
latest=$(find <fixture_root>/.mewcode/sessions -maxdepth 1 -name '*.jsonl' -type f | sort | tail -n 1)
grep -E '"purpose":"(history|plan)"|tool_uses|tool_results' "$latest"
```

通过：同时存在 `history` 与 `plan` 用途记录；工具调用和结果用 `tool_uses`、`tool_results` 内部字段关联。重复的 `REFERENCE-SOURCE-b7e2` 不应产生第二份语义相同的记忆。

## 会话 E：惰性治理与锁

会话 A 至 D 已创建四个会话。启动第五个会话并发送：

```text
不调用工具，只回答“第五个会话完成”。
```

确认两个记忆目录都存在且各有至少两条索引条目。为了可观察治理，可向项目级索引追加一个临时无效条目：

```sh
echo '- [temporary-duplicate](temporary-duplicate.md) — duplicate candidate' >> <fixture_root>/.mewcode/memory/MEMORY.md
```

启动第六个会话并输入：

```text
不调用工具。请简短总结当前 fixture 的长期项目约束，然后结束。
```

最终回复出现后立即检查：

```sh
find <fixture_root>/.mewcode/memory -maxdepth 1 \( -name '.consolidate-lock' -o -name '.consolidate-last-success' \) -print
```

等待最多 15 秒，再检查：

```sh
find <fixture_root>/.mewcode/memory -maxdepth 1 -type f -print | sort
grep -n 'temporary-duplicate' <fixture_root>/.mewcode/memory/MEMORY.md || true
```

通过：主任务不等待治理；治理期间最多一个锁，结束后锁移除；若模型执行治理，出现成功标记并清理临时无效索引。模型未选择清理时记录为“真实模型未选择该操作”，不判定核心功能失败。锁竞争、24 小时时间门与 10 分钟节流由自动化测试负责。

## 会话 F：新会话不自动恢复

退出后启动第七个会话，输入：

```text
不调用工具。上一轮 /do 的完整计划文本是什么？如果没有该会话完整历史，请直接说明。
```

通过：模型不会逐字复述上一轮 `/plan` 的完整计划或工具细节；它可使用本次首轮注入的长期记忆。检查会话数量：

```sh
find <fixture_root>/.mewcode/sessions -maxdepth 1 -name '*.jsonl' -type f | wc -l
```

通过：至少有七个会话文件，每次启动都创建新文件。

## 记录模板

| 场景 | TUI/API 证据 | 文件系统证据 | 实际类别/次数 | 结果 |
|---|---|---|---|---|
| A 首轮注入 | 标记和来源判断 | 一个 session JSONL | 不适用 | 通过/失败 |
| B 用户级记忆 | 最终回复后不阻塞 | 用户 memory 与索引变化 | `user` / `feedback` | 通过/失败 |
| C 项目与参考记忆 | 两次正常终态 | 项目 memory、索引、去重计数 | `project` / `reference` | 通过/失败 |
| D Plan/Do 存档 | `/plan`、`/do` 完成 | JSONL 含 `plan`、`history`、工具字段 | 不适用 | 通过/失败 |
| E 惰性治理 | 主任务立即完成 | 锁、成功标记、索引变化 | 治理调用次数 | 通过/失败 |
| F 新会话 | 不复述旧会话完整历史 | sessions 文件数增加 | 不适用 | 通过/失败 |

## 清理

```sh
rm -rf <fixture_root> <user_config_root>
rm -f /Users/xuchangan/Project/mine/mew-agent/mew01/mewcode
```

如果测试时按 macOS 实际用户配置目录放置文件，不执行上述 `<user_config_root>` 删除命令；只移除本方案创建的 `$HOME/Library/Application Support/mewcode/MEWCODE.md`、`memory/` 及其内容，且先确认它们不是已有用户数据。
