# MewCode 跨会话记忆与会话持久化 Plan

## 架构概览

第九章在既有 Agent Loop、Prompt Builder、内存会话和第八章上下文管理器之上新增指令加载器、会话存储与管理器、记忆服务三个独立组件。启动入口负责创建路径与新会话、清理过期会话、加载指令和记忆索引；Runner 负责将已加载内容注入首轮 Prompt，并在任务正常完成后异步触发记忆提取和治理检查。

会话原始记录以 JSONL 追加存储，恢复后继续使用现有 `conversation.Session` 与第八章 `context.Manager`。第八章压缩仅替换当前工作集，不回写或重写原始 JSONL。自动记忆由模型输出受限 JSON 操作清单，本地校验后才允许写入用户级或项目级记忆目录。

## 核心数据结构

### 路径与注入

```go
type Paths struct {
	UserRoot      string // os.UserConfigDir()/mewcode
	WorkspaceRoot string // 当前项目根目录
	UserMemory    string // <UserRoot>/memory
	ProjectState  string // <WorkspaceRoot>/.mewcode
	Sessions      string // <ProjectState>/sessions
	ProjectMemory string // <ProjectState>/memory
}

type InstructionLoader interface {
	Load(ctx context.Context, paths Paths) (string, error)
}

type MemoryIndexLoader interface {
	Load(ctx context.Context, paths Paths) ([]string, error)
}
```

`InstructionLoader.Load` 返回按“用户级 → 项目级”拼接并安全展开项目级 `@` 引用后的文本。`MemoryIndexLoader.Load` 返回用户级和项目级两份已经限制为 200 行及 25KB 的索引文本。启动入口通过 `prompt.OptionalModules{CustomInstructions, LongTermMemory}` 将两者传入 Runner。

### 会话持久化

```go
type SessionMeta struct {
	ID           string
	Title        string
	CreatedAt    time.Time
	LastActiveAt time.Time
	MessageCount int
}

type Journal interface {
	Append(messages []provider.Message) error
}

type SessionStore interface {
	Create() (*conversation.Session, SessionMeta, error)
	List() ([]SessionMeta, error)
	Restore(id string) (*conversation.Session, SessionMeta, error)
	Delete(id string) error
	CleanupExpired(now time.Time) (int, error)
}
```

`Journal.Append` 在 `conversation.Session.CommitRound` 和 `CommitPlan` 更新内存前写入 JSONL。JSONL 记录使用 `role`、`content`、`tool_uses`、`tool_results`、`ts` 与最小用途字段；用途字段区分普通模型历史和 `/plan` 的展示记录。普通记录恢复为模型历史，规划记录恢复展示历史和待执行计划，不污染普通聊天上下文。`ReplaceHistory` 仅用于第八章压缩后的工作集，不回写 JSONL。

### 自动记忆

```go
type MemoryKind string

const (
	MemoryUser      MemoryKind = "user"
	MemoryFeedback  MemoryKind = "feedback"
	MemoryProject   MemoryKind = "project"
	MemoryReference MemoryKind = "reference"
)

type MemoryOperation struct {
	Action      string     // create, update, delete, noop
	Kind        MemoryKind
	Name        string
	Description string
	Content     string
}

type MemoryService interface {
	Extract(ctx context.Context, mode Mode, transcript []provider.Message) error
	MaybeConsolidate(ctx context.Context) error
}
```

`MemoryService.Extract` 使用当前 Provider 发送不带工具定义的独立请求，要求返回 `MemoryOperation` JSON 数组。本地先验证操作、类别、文件名和目标目录，再通过原子写入更新单条 Markdown 和记忆索引。`MaybeConsolidate` 使用同一受限操作格式，先完成时间、会话数量、扫描节流与锁检查，再异步执行治理。

## 模块设计

### `internal/instructions`

**职责：** 解析用户级目录 `os.UserConfigDir()/mewcode` 和项目根目录，读取两层 `MEWCODE.md`，将用户级内容置前、项目级内容置后。仅项目级指令支持单独一行 `@相对路径` 引用；递归最大深度为 5，已访问绝对路径只展开一次，解析后不在项目根内的路径被拦截。

**失败处理：** 根指令缺失视为无内容；被拦截、缺失或超深引用替换为 HTML 注释后继续；已存在根文件无法读取时返回错误，避免静默丢失约束。

### `internal/conversation`

**职责：** 扩展 `Session` 的 Journal 依赖，在普通提交和计划提交写入内存前持久化；新增 JSONL 编解码、创建、扫描、恢复、删除、过期清理能力。

**恢复规则：** 逐行读取并跳过无效 JSON；从第一个未配对工具调用开始丢弃该调用及其后续记录；从文件名获得创建时间，扫描记录获得最后活跃时间、标题和消息数；恢复超过 24 小时未活跃的会话时插入时间跨度提醒。每次启动仍调用 `Create`，恢复方法仅供第十章会话命令使用。

### `internal/memory`

**职责：** 管理用户级 `os.UserConfigDir()/mewcode/memory/` 与项目级 `<项目根>/.mewcode/memory/`。`user`、`feedback` 写用户级，`project`、`reference` 写项目级；每条记忆单独保存为带 frontmatter 的 Markdown，目录通过 `MEMORY.md` 索引。

**安全边界：** 模型只能给出 `create`、`update`、`delete`、`noop` 操作；本地拒绝未知类别、非法文件名、越界路径、无效 frontmatter 和无效 JSON。索引读取时按行和字节双上限截断并附加提示。治理通过 `.consolidate-lock` 防并发，且只允许修改目标记忆目录。

### `internal/agent`

**职责：** 在 `Options` 中接收预加载的 `prompt.OptionalModules` 和 `MemoryService`。每次请求构建 Bundle 时传递 OptionalModules；普通聊天、`/do`、`/plan` 在获得最终文本回复、完成会话提交后复制可见会话记录并异步调用 `Extract` 和 `MaybeConsolidate`。

**失败处理：** `/compact`、取消、失败、迭代限制和未知工具停止均不触发记忆工作。后台提取或治理失败只记录安全元数据，不改变已经发送的主任务终态事件。

### `cmd/mewcode/main.go`

**职责：** 派生用户级和项目级路径，初始化会话存储、创建新的初始会话、执行过期清理、加载指令和记忆索引、创建记忆服务，并将这些依赖传入 Runner。此处不添加 `/session` 命令或 TUI 选择界面。

## 模块交互

```text
启动：
main → Paths → SessionStore.CleanupExpired → SessionStore.Create
     → InstructionLoader.Load + MemoryIndexLoader.Load
     → Runner(PromptOptions, MemoryService) → 首轮 BuildBundle

普通任务：
Runner → Session.CommitRound/CommitPlan → Journal.Append → 内存更新
       → 主任务终态事件 → goroutine(MemoryService.Extract, MaybeConsolidate)

后续恢复：
SessionStore.Restore → 容错重建 Session → 第八章 Context Manager 判断/压缩
                    → 时间跨度提醒 → 第十章命令层
```

## 文件组织

```text
internal/
├── instructions/
│   ├── paths.go
│   ├── loader.go
│   └── loader_test.go
├── conversation/
│   ├── session.go
│   ├── journal.go
│   ├── store.go
│   └── store_test.go
├── memory/
│   ├── paths.go
│   ├── index.go
│   ├── operation.go
│   ├── service.go
│   └── memory_test.go
├── agent/
│   ├── event.go
│   ├── runner.go
│   └── runner_test.go
└── prompt/
    └── (复用既有 OptionalModules)

cmd/mewcode/main.go
docs/ch09-memory/plan.md
docs/ch09-memory/task.md
docs/ch09-memory/checklist.md
```

## 技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 用户级根目录 | `os.UserConfigDir()/mewcode` | 与既有默认配置目录一致。 |
| 指令优先级 | 用户级先拼接，项目级后拼接 | 项目特定约束更靠后，符合本章项目级优先语义。 |
| `@` 引用范围 | 仅项目级 `MEWCODE.md`，仅项目根内 | 当前工作区是唯一明确安全边界，避免用户级跨项目引用歧义。 |
| 会话完整性 | JSONL 逐条追加；恢复时跳过坏行、从未配对调用处截断 | 追加成本低，崩溃最多损失尾部不完整记录。 |
| 压缩与存档关系 | 原始 JSONL 不因第八章压缩改写 | 归档保持真实记录；压缩只是当前模型工作集。 |
| `/plan` 持久化 | 写展示用途记录与待执行计划，不混入默认模型历史 | 保留用户可见对话并确保恢复后普通聊天上下文干净。 |
| LLM 写文件权限 | 模型只产出受限 JSON 操作；本地验证后执行 | LLM 负责语义与去重，程序负责路径、类别、文件名和格式安全。 |
| 提取与治理并发 | 终态后 goroutine；治理使用原子锁文件 | 不阻塞 UI；阻止多个实例同时整理。 |
| 配置 | 不新增 YAML 配置项 | 目录、格式、治理门槛和索引上限均为固定产品行为。 |

## 测试策略

1. **指令加载器单测：** 验证两层顺序、缺失文件、多层引用、循环去重、五层限制、越界和缺失标记。
2. **会话存储单测：** 验证同秒 ID 唯一性、JSONL 追加、Provider 无关工具块、坏行跳过、未配对调用截断、元信息扫描、24 小时提醒、30 天清理和追加失败不更新内存。
3. **记忆服务单测：** 验证类别目录映射、frontmatter、索引更新及双上限、非法操作拒绝、无操作不写入、锁竞争、节流、时间门和会话门。
4. **Runner 集成测试：** 验证首轮注入内容，普通聊天、`/do`、`/plan` 的异步提取，`/compact`/取消/失败不提取，后台失败不改变主任务终态，以及恢复会话与第八章压缩的衔接。
5. **启动入口测试：** 验证每次启动创建新会话、初始化清理过期会话、用户级路径由 `os.UserConfigDir` 派生而非 `--config` 位置。

## Spec 覆盖

| Spec | 实现归属 |
|---|---|
| F1–F2 | `internal/instructions`、启动入口、Prompt OptionalModules |
| F3–F5 | `internal/conversation`、启动入口、现有第八章 Context Manager 衔接 |
| F6–F9 | `internal/memory`、Runner 后台编排 |
| N1–N2 | Journal 写入顺序、Provider 无关 JSONL 记录 |
| N3–N5 | 后台 goroutine、路径校验、既有 logging 安全字段 |
| N6–N7 | 固定行为不增配置；临时目录、可控时钟、Provider 测试替身 |
