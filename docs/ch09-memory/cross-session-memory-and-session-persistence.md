# 理论学习：跨会话记忆与会话持久化

> **架构定位**：本章继续实现 ④ 记忆层，焦点从会话内的上下文压缩转向跨会话的持久记忆。

---

## 每次开新会话都像失忆了一样

MewCode 有了上下文管理之后，可以在一个会话中持续工作好几个小时而不撞墙。但你试着用了几天之后，会发现一个越来越让人抓狂的问题。

昨天下午你花了两个小时跟 Agent 讨论项目架构，它对你的代码了如指掌：知道 handler 放在哪个目录，知道数据库用的是 PostgreSQL 15，知道你们团队的 commit message 必须用英文。一切配合得很默契。

今天早上你打开终端，输入「帮我在 handler 里加个接口」。

Agent 回了一句：「handler 在哪个目录？这个项目是做什么的？」

你深呼一口气，又开始把 README 喂给它，又开始解释项目结构。

![跨会话失忆](https://feishu.cn/file/Qb6Qb0LrpoSujwx0wH6ck9HYn5g)

更烦的是那些个人偏好。你已经说过三次「我不喜欢用 `interface{}`，请用 `any`」。每个新会话，Agent 还是会写 `interface{}`。你已经强调过「commit message 用英文」，新会话里 Agent 又开始写中文 commit message。

人类同事不会这样。你跟同事说过一次「我们团队的命名规范是驼峰式」，他就记住了，不需要每天早上重复一遍。

问题出在哪？上一章的上下文管理解决的是「会话内」的记忆管理：在一个对话窗口里，如何在有限的 token 空间里保留最有价值的信息。但一旦关闭终端、开启新会话，一切归零。对话历史没了，Agent 的「记忆」彻底清空。

这一章要解决的就是「跨会话」的记忆问题：如何让 Agent 在新会话开始时，快速回到「了解你和你的项目」的状态。

---

## 记忆是分层的

在设计记忆系统之前，我们先想想人类的记忆是怎么工作的。

你现在正在读这段文字，你的大脑里同时「记着」两层东西。表层是你此刻正在处理的内容：这段话的意思、上一段讲了什么。这是**工作记忆**，容量极小，访问速度极快，用完就丢。

深层是经过反复巩固的长期知识。你的编程语言偏好、你们团队的开发规范、项目的整体架构、昨天跟同事讨论的方案。这些信息被持久地存储在大脑中，需要时可以调取出来。这是**长期记忆**。

![工作记忆与长期记忆](https://feishu.cn/file/CU64baV56oO5sNxiGA2c28GYnle)

MewCode 的记忆系统正好对应这两层。

**工作记忆**，就是上一章讲的上下文窗口。那个 200K token 的空间就是 Agent 的「大脑带宽」，当前正在处理的所有信息都在这里，容量有限，需要压缩管理。

**长期记忆**，对应的是所有持久化到磁盘上的信息。这又分为三种形态：

- **会话持久化**：你跟 Agent 的每次对话都保存下来，下次可以恢复。就像你第二天早上打开电脑，看到昨天的聊天记录，上下文立刻回来了。
- **项目指令文件**：预先写好的项目知识和编码规范，相当于员工的「入职文档」。
- **自动记忆**：Agent 在对话中自动积累的经验，比如你的编码偏好、项目的技术细节。

![长期记忆三种来源](https://feishu.cn/file/YDO9bYBREoE1DMxY41tcQos8nNd)

---

## 项目指令文件：Agent 的入职文档

新员工入职第一天，你不会让他自己去翻代码猜这个项目是干嘛的。你会给他一份文档：技术栈是什么、代码规范是什么、常见的坑有哪些、部署流程是什么样的。他读完这份文档，就能快速进入状态。

项目指令文件就是给 Agent 准备的入职文档。它是一个叫 `MEWCODE.md` 的 Markdown 文件，放在项目根目录下。MewCode 每次启动新会话时，会自动读取这个文件，把内容作为上下文注入到发给 API 的 messages 中。Agent 在第一轮对话开始之前，就已经「读完了入职文档」。

![Agent 阅读 MEWCODE.md](https://feishu.cn/file/GEvBb5nXMoahxIxSwRMcxQoynpe)

一个典型的 MEWCODE.md 长这样：

```Markdown
# 项目：电商后台 API

## 技术栈
- Go 1.22 + Gin + GORM
- PostgreSQL 15

## 代码规范
- 使用 any 代替 interface{}
- 错误处理用 fmt.Errorf("xxx: %w", err) 包装
- commit message 用英文，格式：type(scope): description

## 注意事项
- 不要直接修改 internal/generated/ 目录，那是自动生成的代码
- 运行测试前需要先启动 PostgreSQL

```

有了这个文件，你再也不用每次新会话都重复介绍项目背景了。Agent 一上来就知道你的技术栈，知道错误处理要怎么写，知道哪些目录不能碰。

你可能会说：这不就是一个 README 吗？有点像，但有本质区别。README 是写给人看的，而 MEWCODE.md 是写给 Agent 看的。你不需要在里面写如何安装、如何运行这些人类开发者需要但 Agent 不关心的内容。你写的是 Agent 在编码过程中需要遵守的规则和需要知道的上下文。

### 优先级栈：项目级覆盖用户级

MEWCODE.md 不只存在于项目根目录。MewCode 支持多个层级的指令文件：

```Plain
1. ~/.mewcode/MEWCODE.md           <- 用户级（个人偏好，最先加载）
2. 项目根目录/MEWCODE.md          <- 项目级
3. 项目根目录/.mewcode/MEWCODE.md <- 项目级（可被 .gitignore 忽略）
4. MEWCODE.local.md               <- 本地私有（不进 git，最后加载）


```

![图片展示了MewCode指令文件的加载顺序及优先级。从上至下依次为用户级的`~/.mewcode/MEWCODE.md`（最先加载）、项目级的`项目根目录/MEWCODE.md`、可`gitignore`的`~/.mewcode/MEWCODE.md`、本地私有的`MEWCODE.local.md`（最后加载）。箭头表示加载顺序，优先级从上至下越来越高。该图与文档中介绍MewCode支持多层级指令文件、内容拼接不覆盖等设计内容相关，直观呈现了文件加载机制。](https://feishu.cn/file/WP2RbSRtVop2hhxGab3c3DLunbd)

多个文件的内容会被拼接在一起，后加载的不会覆盖前面的，而是用分隔线连接。高优先级的内容排在后面。因为 LLM 对 prompt 靠后出现的内容通常更为重视，排在后面意味着在有冲突时优先被遵循。Claude Code 和 Codex 都采用了这种设计。

这个设计的好处是什么？你可以把个人偏好写在用户级文件里，比如「我偏好函数式风格」「回复用中文」。这些偏好在你参与的所有项目中都生效。然后在某个具体项目的项目级文件里写「这个项目用 OOP 风格」「commit message 用英文」。项目级的内容排在用户级后面，当项目需求和个人偏好冲突时，项目说了算。

这跟 Git 的 `.gitconfig` 思路类似——都是分层管理、项目级优先于用户级。但具体机制不一样：`.gitconfig` 是按 key 值覆盖（项目级的 `user.name` 直接替换用户级的同名 key），MEWCODE.md 是拼接排序（多个层级的内容全部保留，高优先级的排在后面，靠位置让 LLM 优先关注）。理解到这一层就够了：层级和顺序的设计意图相同，但 LLM 不像 Git 那样"严格替换"，所以底下是拼接而不是覆盖。

为什么有两个项目级的位置？因为有些团队希望 Agent 的指令文件提交到 Git，让所有团队成员共享同一份 Agent 指令，那就放在项目根目录。而有些开发者的 Agent 指令包含个人特定的内容，比如本地的数据库密码，不想提交到 Git，就放在 `.mewcode/` 目录下，然后把这个目录加到 `.gitignore`。

加载流程很直观：MewCode 启动时，按优先级顺序依次检查每个路径，文件存在就读取，不存在就跳过。所有找到的文件内容用 `---` 分隔线拼接成一段完整的指令文本，最终通过 system-reminder 注入到 messages 中。Agent 在处理用户的实际请求之前，就已经「读完了入职文档」。

![图片展示了MewCode加载项目指令文件的流程。从上至下依次是用户级、项目级、.mewcode级和本地私有指令文件，按此顺序读取。项目级和.mewcode级文件存在则读取，不存在跳过，用“---”分隔线拼接成一段完整的指令文本，最终注入到messages中。该图与文档中MewCode启动时按优先级顺序检查文件，拼接指令文本注入messages的内容相呼应，直观呈现了加载流程。](https://feishu.cn/file/KsVLbMb1BoLk1PxEWWGcTHOcnoh)

### @ 引用：大型项目的模块化指令

当项目变大之后，把所有指令塞在一个 MEWCODE.md 里会变得很臃肿。比如一个微服务项目，每个服务的编码规范可能不一样，API 网关模块和支付模块的注意事项也完全不同。

`@` 引用指令解决这个问题。你可以在 MEWCODE.md 中用 `@` 前缀引用其他文件的内容：

```Markdown
# 项目说明

@./docs/architecture.md
@./docs/coding-style.md
@./internal/handler/README.md

```

MewCode 在加载 MEWCODE.md 时，会把 `@` 引用行替换为被引用文件的完整内容。这样你可以把不同模块的说明分散到各自的目录下，MEWCODE.md 只做一个「汇总入口」。

![include 汇总入口](https://feishu.cn/file/TpEKbYCDoojKfhx2oSmcbSyMnyf)

处理 `@` 引用的核心逻辑是逐行扫描内容，遇到 `@` 引用行就解析路径、读取文件、替换原行。如果被引用的文件里还有 `@` 引用，递归处理：

```Plain Text
function ProcessIncludes(content, baseDir, depth):
    for each line in content.split("\n"):
        if line.startsWith("@") and isPathLike(line[1:]):
            path = joinPath(baseDir, line[1:])
            included = readFile(path)
            processed = ProcessIncludes(included, directoryOf(path), depth + 1)
            result.append(processed)
        else:
            result.append(line)
    return join(result, "\n")

```

但递归引入了两个安全风险。

第一个是无限递归。A 文件 include B，B 又 include A，就会死循环。所以用 `depth` 参数限制嵌套深度，最多 5 层，超过就停止：

```Plain Text
if depth >= 5:
    return content

```

但只看 depth 还不够——它能拦住"递归太深"，却拦不住"反复展开同一个文件"。比如 A include B、B include A，在 depth 用尽之前，A 和 B 已经各自被展开了好几次。所以实际还要维护一个 `visited` 集合，记下已经处理过的绝对路径，遇到重复路径直接跳过。两层防护互补：`visited` 防环路（精确去重），`depth` 防深度（兜底）。

第二个是路径越界。如果有人在 `@` 引用里写了 `@../../etc/passwd`，那就不是在加载项目说明了，是在读系统文件。所以 `@` 引用路径必须在项目目录内，越界的直接拦截：

```Plain Text
if not isWithinProject(absPath):
    result.append("<!-- @ blocked: path outside project -->")
    continue

```

文件不存在时也不报错，只是插入一条 HTML 注释标记，然后继续处理后面的内容。

![include 安全保护](https://feishu.cn/file/Aoe6blFlnocIA0xDvs0cGYxNnag)

---

## 会话持久化：让对话可以「存档读档」

项目指令文件解决了预先写好的项目知识。但还有一类长期记忆是动态产生的：你跟 Agent 的每次对话、每个操作、每个决策，需要在会话结束后持久化保存，以便后续恢复。

MewCode 的会话持久化使用 JSONL（JSON Lines）格式：每行一个 JSON 对象，代表一条消息。

![会话存档与读档](https://feishu.cn/file/ZBBmbx86woBRZLxpu7DccmdynQk)

你可能会问：为什么是 JSONL？为什么不用 SQLite，或者普通的 JSON 文件？

这个选择背后有很实际的工程考量。

先说为什么不用数据库。SQLite 之类的嵌入式数据库确实功能强大，但它引入了额外的依赖。MewCode 是一个命令行工具，理想状态下编译出来一个二进制文件，丢到任何机器上就能跑，不依赖任何运行时。

如果引入数据库依赖，编译变得复杂，交叉编译变得痛苦，分发的简洁性就没了。而且对于会话记录的场景，关系查询能力完全用不上，杀鸡用牛刀了。

![JSON 与 JSONL 对比](https://feishu.cn/file/HsTCbXMB8o4vtyxyTJycE538nEs)

再说为什么不用普通 JSON。普通 JSON 是把整个数组序列化成一个文件。要追加一条消息，你得：读取整个文件，解析成数组，往数组里追加一个元素，再把整个数组序列化写回文件。当会话有几百条消息时，每追加一条都要读写整个文件，效率很低。更严重的问题是：如果程序在写文件的过程中崩溃了（比如被强制杀掉），文件可能只写了一半，变成一个不合法的 JSON，整个会话数据就全坏了。

JSONL 完美解决了这两个问题。

追加写入：每条新消息直接追加到文件末尾，一行搞定，不需要读取和重写整个文件。性能是 O(1) 而不是 O(n)。

崩溃安全：即使程序中途崩溃，已经写入的完整行不会受影响。最坏的情况是最后一行不完整，恢复时跳过那一行就好，前面的消息全部完好。

增量加载：恢复时逐行读取，遇到解析错误的行跳过继续，不会因为一行坏数据就丢掉整个会话。

这些特性天然适合 Agent 的工作模式：每产生一条消息就写一行，流式写入，实时持久化。

### 存储格式

每条 JSONL 记录的骨架是三个字段：消息角色、内容、时间戳。如果这条消息还携带了工具调用或者工具结果，再挂上两个可选的数组。

```
{"role":"user","content":"帮我写一个 HTTP handler","ts":1736951405}
{"role":"assistant","content":"好的，让我看看...","tool_uses":[{"tool_use_id":"t1","tool_name":"ReadFile","arguments":{"path":"/project/main.go"}}],"ts":1736951407}
{"role":"user","content":"","tool_results":[{"tool_use_id":"t1","content":"package main..."}],"ts":1736951408}
{"role":"assistant","content":"我来创建...","ts":1736951415}


```

这几个字段足以完整重建一个会话。`role` 标识消息角色，`content` 是这条消息的文本部分，`ts` 是 Unix 时间戳，用整数比 ISO 格式更紧凑。

这里有一个容易踩错的地方：工具结果不是一种独立的消息角色。模型 API 只认 user 和 assistant 两种角色，工具结果是以 user 消息的身份回传给模型的，所以落盘时它也挂在一条 `role` 为 `user` 的记录上，只是这条记录的 `content` 通常为空，真正的内容在 `tool_results` 里。第三行看起来「没有正文」，不是数据坏了，而是它本来就是一条工具结果消息。

![图片展示了JSONL记录的结构，分为role、content、ts、tool_uses、tool_results五个部分。role有assistant/user标识，content为纯文本，ts是Unix整数秒。tool_uses记录助手消息发起的工具调用，tool_results记录用户消息携带的工具结果。图片与上下文紧密相关，上下文提到存储格式中纯文本消息落盘是干干净净的一行，而toolUses和toolResults靠toolUseId配成对，是恢复会话时的唯一线索，图片直观呈现了这些字段在JSONL记录中的位置和含义。](https://feishu.cn/file/FwlJbnpovoNCYKxbIo2c5dHen4b)

定义对应的数据类型：

```
ToolUseRecord:
    toolUseId:  string        // 这次工具调用的 ID
    toolName:   string        // 调用的工具名
    arguments:  map           // 调用参数

ToolResultRecord:
    toolUseId:  string        // 指回对应的工具调用
    content:    string        // 工具输出
    isError:    bool          // 工具是否执行失败

SessionRecord:
    role:         string
    content:      string
    toolUses:     ToolUseRecord[]?      // 助手消息发起的工具调用
    toolResults:  ToolResultRecord[]?   // 用户消息携带的工具结果
    ts:           int64                 // Unix 时间戳


```

`toolUses` 和 `toolResults` 靠 `toolUseId` 配成对，这是恢复会话时能把「模型要读哪个文件」和「读出来是什么」重新接上的唯一线索。这两个字段为空时会整个从 JSON 里省略，所以一条纯文本消息落盘还是干干净净的一行。

值得说一句的是这里存的是什么形式的工具块。模型厂商各有各的线格式，Anthropic 用 `tool_use` 和 `tool_result` 两种 content block，OpenAI 的 Responses API 用 `function_call` 和 `function_call_output`，Chat Completions 那套又是 `tool_calls` 加一条 `tool` 消息。如果直接把某一家的原始 JSON 存进会话文件，这份会话就跟那家厂商绑死了，用户改一下配置换个模型，历史立刻读不回来。

所以落盘的是一份跟协议无关的内部表示，只保留调用 ID、工具名、参数、输出这几样任何厂商都有的东西。恢复的时候先读成内部表示，再由对应的模型适配层翻译成当前厂商要的线格式。这样换 provider 是配置层面的事，不会波及已经存在磁盘上的历史。

### 会话文件的组织

会话文件放在项目的 `.mewcode/sessions/` 目录下：

```Plain Text
.mewcode/
  sessions/
    20250115-143000-a3f7.jsonl
    20250116-091500-b2c9.jsonl

```

会话 ID 格式是 `YYYYMMDD-HHMMSS-xxxx`：前 15 字符的时间戳让你一眼看出创建时间，末尾 4 字符十六进制随机后缀避免同秒冲突。单进程下普通用户几乎用不到后缀，但 CI/CD 这种可能瞬时启动多个会话的场景能保证 ID 不撞车——同秒生成两个会话也会落到不同文件，不会出现多个进程往同一个 `.jsonl` 里交错写消息。

你可能会问：没有元信息文件，`/session list` 要怎么快速展示会话列表？答案是直接解析 JSONL。列表展示需要的信息就两个：会话创建时间（从文件名就能读出来）和最后活跃时间（读文件最后一行的 `ts` 字段）。JSONL 文件通常不大，逐行扫描也足够快，额外维护一个元信息文件带来的复杂度不值得。

![图片展示了MewCode的会话存储结构。在`.mewcode/`目录下，有一个`sessions/`文件夹，其中存放了多个JSONL文件，如`20250115-143000.jsonl`和`20250116-091500.jsonl`。图片下方提示“会话ID = 时间戳，无需额外元信息文件”，并说明`/session list`可直接解析JSONL。该图与上下文紧密相关，直观呈现了会话文件的组织方式，帮助理解会话管理器如何维护和操作会话。](https://feishu.cn/file/EN4mbZOGMoi37dxfkXscDUg4nD5)

### 会话管理器

会话的创建、恢复、列表、删除，统一由 `SessionManager` 管理。它维护 `.mewcode/sessions/` 目录，提供所有会话操作的入口。

创建新会话时，生成一个唯一 ID，创建 `.jsonl` 文件：

```Plain Text
function SessionManager.Create():
    id = formatNow("YYYYMMDD-HHMMSS") + "-" + randomHex(4)
    file = openFile(sessionsDir + "/" + id + ".jsonl", CREATE | APPEND)
    session = Session { id: id, file: file, conv: newConversation() }
    return session

```

`Session` 对象是一个活跃的会话实例，持有打开的文件句柄。每次追加消息时同步写入 JSONL 文件：

```Plain Text
function Session.Append(message):
    record = SessionRecord.fromMessage(message)
    file.write(jsonSerialize(record) + "\n")
    conv.addMessage(message)
    meta.messageCount += 1
    meta.lastActive = now()

```

注意这里的写入顺序：**先写文件再更新内存**。如果写文件成功但更新内存前程序崩溃，没关系，下次恢复时会从文件重建内存状态。如果反过来先更新内存再写文件，崩溃时内存中有但文件里没有的消息就永远丢失了。

![先写文件再更新内存](https://feishu.cn/file/SA75bwUoQog55ZxwlMTcLE8inBb)

### 恢复会话：处理各种异常情况

![图片展示了恢复会话的四步流程。第一步是逐行解析JSONL，用书本图标表示；第二步是修复调用链，以链条图标呈现；第三步是检查token，用时钟图标示意；第四步是时间提示，以闹钟图标标注24小时。这些步骤对应文档中恢复会话流程的四步走，直观呈现了各步骤内容及顺序，帮助理解恢复会话的复杂操作。](https://feishu.cn/file/AruQbdvB2ovjgOxNiP1c0QQVnSc)

恢复一个已有会话是整个会话系统中最复杂的部分，因为你要处理各种异常情况。整个流程分四步走。

![图片展示了JSONL文件恢复示意图，说明了JSONL（JSON Lines）是一种按行存储JSON对象的文本格式，每行一个独立的JSON对象。示例中，messages.jsonl文件包含5条消息，其中第5条消息因崩溃时只写了半行而被跳过，其余4条消息恢复成功。右侧显示4条消息恢复成功，1行跳过，标注只恢复完整的JSON行，不完整的行将被跳过。该图与上下文介绍的JSONL逐行解析、修复工具调用链等内容相关，直观呈现了JSONL文件的处理情况。](https://feishu.cn/file/TWGyb8z0IoMjqaxMzMFctTjWnHg)

第一步是**逐行解析 JSONL**。跟普通 JSON 的一次性解析不同，JSONL 可以逐行独立读取，遇到解析失败的行就跳过继续：

```Plain Text
messages = []
for each line in file:
    record = jsonParse(line)
    if record.error:
        log("line parse error, skipping")
        continue
    messages.append(record.toMessage())

```

如果某一行 JSON 解析失败，可能是崩溃时只写了半行。不要放弃整个会话，跳过这一行继续读后面的。这正是我们选 JSONL 而不是普通 JSON 的原因之一。

![图片展示了工具调用链的配对情况。左侧“配对完整”部分，从ReadFile读取文件，经EditFile编辑后，结果被正确记录；右侧“补一条错误结果，不截断”部分，Bash执行被中断，需补一条错误结果。该图与上下文紧密相关，直观呈现了模型API对`tool_use`与`tool_result`配对的严格要求，以及在历史里出现不配对情况时的处理方式，即补一条错误结果以确保请求能正常发出。](https://feishu.cn/file/PtuZbKcOuomJQsxidomcDdbGnUd)

第二步是**修复工具调用链**。模型 API 在这件事上很严格，每一个 `tool_use` 都必须能找到配对的 `tool_result`，缺一个整条请求就会被拒掉，不是模型答得差一点，是请求根本发不出去。

历史里出现不配对的情况，来路比你想的多。工具跑到一半用户按了中断，写完了调用还没写结果；进程被强制杀掉，最后一条工具结果没落盘；恢复出来的会话本身就停在一个悬空的调用上。这些都会留下一个没有下文的 `tool_use`。

一种做法是截断，往前退到最近一个「所有工具调用都有结果」的位置，把后面的全丢掉。这个方案的问题是代价太大：用户可能已经在这一轮里改了七八个文件，只因为最后一个工具调用没写完结果，前面做完的工作全被抹掉，恢复出来的会话回到了几分钟以前。

MewCode 选的是补位。悬空的工具调用后面补一条标记为错误的结果，告诉模型这个工具被中断了。措辞上不能说得太满，因为工具可能压根没启动，也可能已经把文件改了一半，所以补的话是「执行被中断，产生的影响需要自行确认」这个意思，让模型知道该去核实而不是直接假定什么都没发生。反过来，如果历史里有一条工具结果找不到对应的调用，那它留着只会让请求被拒，直接丢掉。

这里有两个设计上的选择值得单独说。

一个是补出来的结果不写回会话历史。会话文件的职责是如实记录发生过什么，补位只是为了让这一次请求合法。写回去的话，磁盘上就会多出一条用户根本没经历过的工具结果，下次恢复读到的历史和真实发生的事情就对不上了。

另一个是这道修复放在发请求之前，而不是放在加载会话之后。中断、崩溃恢复、并发写入交错，这几种来路最后都汇到同一个出口，就是把消息发给模型的那一刻。守在出口上只需要写一遍，守在入口上则要在每个可能产生不配对的地方各写一遍，还容易漏。

第三步是**检查 token 量**。如果恢复的会话很长，token 量可能已经超过压缩阈值。直接触发一次压缩再返回给用户，免得用户刚恢复会话就因为 token 超限报错。

第四步是**插入时间跨度提示**。如果距离上次活跃超过 24 小时，在对话中插入一条消息提醒 Agent：上次会话是什么时候，中间可能有代码变更，建议重新读取相关文件。这能有效避免 Agent 拿着过期的文件内容做决策。

### 过期会话清理

用久了之后，`.mewcode/sessions/` 目录下会堆积大量的旧会话文件。超过 30 天没有活跃的会话，基本不会再被恢复了，MewCode 会在启动时自动清理它们。会话列表则按最后活跃时间倒序排列，最近用过的排在最前面。

![过期会话清理](https://feishu.cn/file/CPvGbN7uCo5fddxc2x6cDz8BnQe)

---

## 自动记忆：让 Agent 自己「记笔记」

项目指令文件是你手动维护的长期记忆。但有些知识是在对话过程中冒出来的，你不太可能每次都去编辑 MEWCODE.md。

比如你在对话中随口说了一句「不要用 tab，用 4 个空格」。这是一个明确的偏好，Agent 应该记住。但你大概不会专门打开 MEWCODE.md 去加一行「缩进用 4 空格」。

再比如 Agent 写了一段代码，你纠正说「不对，这里应该用互斥锁而不是消息通道」。这是一个反馈，Agent 下次遇到类似场景时应该做出不同的选择。

自动记忆系统就是解决这个问题的：Agent 在对话过程中自动识别值得记住的信息，分类存储，在后续会话中自动加载。

![自动记忆记笔记](https://feishu.cn/file/XpoqbfK8dokKw7xHo2YciEXinAh)

### 四类记忆

自动记忆按内容性质分成四个类别。这四类天然分成两组：**用户偏好**和**纠正反馈**是关于"人"的，跟你走，所以存到用户级目录（`~/.mewcode/memory/`）；**项目知识**和**参考信息**是关于"项目"的，跟项目走，所以存到项目级目录（`<项目根>/.mewcode/memory/`）。每个记忆文件 frontmatter 里的 `type` 字段决定它落在哪个目录——type 选好了，目录也就确定了。

第一类是「用户偏好」（user）。用户的个人编码习惯和风格要求，比如「我喜欢简洁的代码风格」「不要用 tab」「回复尽量用中文」。这类信息不依赖具体项目——你换到另一个仓库，「不要用 tab」依然成立——所以存在用户级目录 `~/.mewcode/memory/` 下，让所有项目都能复用。

第二类是「纠正反馈」（feedback）。用户明确指出 Agent 的输出有问题并给出正确做法，比如「这个函数名不好，改成 ProcessOrder」「这里应该用互斥锁不是消息通道」。这类记忆是关于"用户怎么纠错"的——同一个人在不同项目里大概率会做出类似的纠正——所以同样存到用户级目录 `~/.mewcode/memory/`。

第三类是「项目知识」（project）。关于当前项目的具体技术信息，比如「部署脚本在 scripts/ 目录」「用 GitHub Actions 做 CI」「数据库是 PostgreSQL 15」。这类信息换个项目就完全不适用了，所以存在项目级目录 `<项目根>/.mewcode/memory/` 下，可以选择提交到 Git 跟团队共享，也可以加到 `.gitignore` 个人保留。

第四类是「参考信息」（reference）。外部链接和资料，比如「API 文档在 https://docs.example.com」「设计文档在 Confluence 的某个页面」。这些链接通常是项目特定的——「我们这个项目的 API 文档」——所以也存到项目级目录 `<项目根>/.mewcode/memory/`，团队成员需要时一起看得到。

![四类自动记忆](https://feishu.cn/file/YesJbvrU9oeRZgxyNUMcAlqQn0f)

### 自动记忆存在哪：目录结构

既然 MEWCODE.md 已经是 Markdown，自动记忆也用同样的格式。但不是把所有记忆塞进一个文件，而是用一个**目录**来组织：每条记忆是一个独立的 `.md` 文件，再用一个 `MEMORY.md` 索引文件把它们串起来。

```Plain Text
.mewcode/memory/
  MEMORY.md            # 索引文件（注入到 messages）
  user-prefers-any.md   # 每条记忆一个文件
  feedback-testing.md
  project-deadline.md


```

为什么不直接写进 MEWCODE.md？因为 MEWCODE.md 是你手动维护的、提交到 Git 的、团队共享的。自动记忆是机器生成的、个人专属的，混在一起会污染你精心维护的指令文件。放在 `.mewcode/memory/` 目录下加到 `.gitignore` 就好。

为什么用目录而不是单文件？因为每条记忆可以独立增删改，不需要编辑一个大文件。删除一条记忆就是删一个文件，更新一条就是改一个文件，不会干扰其他记忆。

每个记忆文件有 YAML frontmatter 描述元信息：

```YAML
---
name: user-prefers-any
description: 用户偏好使用 any 而非 interface{}
type: feedback
---

用户明确要求使用 any 替代 interface{}。
**Why:** 现代 Go 语法更简洁
**How to apply:** 所有新写的泛型代码用 any


```

`MEMORY.md` 索引文件只存指针，每行一条，格式简洁：

```Markdown
- [偏好 any 语法](user-prefers-any.md) — 用户要求用 any 替代 interface{}
- [项目用 golang-migrate](project-migration-tool.md) — 数据库迁移工具选型


```

索引文件注入到 messages 时有上限：最多 200 行或 25KB，超了会截断并附加警告。这防止记忆太多时撑爆上下文。

为什么是 200 行 / 25KB？按平均约 125 字符一行的密度算，200 行 ≈ 25KB ≈ 2000-3000 tokens，正好占 200K 上下文窗口的 1-2%。每条记忆指针通常占 1-2 行，这个上限既能装下相当数量的记忆，又把长期记忆的固定开销控制在窗口的零头里，不会挤压工作记忆的空间。

### 提取与去重

Agent 不需要逐字逐句地检测记忆。合适的时机是**每轮 Agent Loop 结束后**，模型给出最终回复、不再调用工具的那个时刻。这时候在后台异步回顾本轮对话，看看有没有值得记住的东西。异步执行不阻塞用户的下一轮输入，用户可以立刻开始新的对话。

提取的过程通过一次独立的 LLM 调用完成。系统把 `MEMORY.md` 索引和所有现有记忆文件的摘要清单发给模型，连同最近一轮对话，让它分析对话、按四个类别决定是否需要创建新记忆、更新已有记忆、或删除过时记忆：

```Plain Text
memoryExtractionPrompt = """
下面是当前的记忆目录清单和最近一轮对话。
分析对话，提取值得长期记忆的信息。

操作：
- 创建新记忆文件（写 frontmatter + 正文，更新 MEMORY.md 索引）
- 更新已有记忆文件（如果信息有变化）
- 删除过时记忆文件（同时从 MEMORY.md 移除指针）

分类：user / feedback / project / reference
已有相同含义的记忆不要重复创建。
没有值得记忆的内容就什么都不做。
"""


```

因为模型拿到了所有现有记忆的清单，去重是天然的。「用 any 代替 interface{}」和「不要使用 interface{} 类型」说的是同一件事，LLM 能识别出来，不会重复创建。不需要我们自己实现相似度算法。

注意 prompt 里「没有值得记忆的内容就什么都不做」这句。大多数对话其实没什么值得长期记住的信息，比如「帮我加个函数」「好的做完了」。如果 Agent 对每轮对话都创建记忆文件，很快记忆目录就被垃圾信息淹没了。

![图片展示了MEWCODE记忆提取与加载的工作流程。左侧为对话场景，用户提出代码缩进格式问题，助手给出反馈。中间是LLM（大语言模型）分析提取内容。右侧是记忆存储（项目内存），包含MEMORY.md（索引文件）、user-prefers-any.md（用户偏好相关记忆）和feedback-indent.md（缩进格式反馈记忆）。底部标注“创建新记忆文件 + 更新索引”。该图与文档中加载记忆时跟MEWCODE.md走同一条管线的内容相关，直观呈现了记忆提取与加载过程。](https://feishu.cn/file/SIAkbXcunonUdVxlFHXc1Fk8n3e)

### 加载：跟 MEWCODE.md 走同一条管线

每次新会话启动时，MEMORY.md 索引文件跟 MEWCODE.md 走同一条加载管线：发现文件、读取内容、注入到 messages 上下文中。用户什么都不用做，Agent 自动就「想起来」了之前学到的东西。

最终发给 API 的请求结构看起来像这样：

```JSON
{
  "model": "claude-sonnet-4-6",
  "system": "你是 MewCode，一个终端 AI 编程助手。",
  "messages": [
    { "role": "user", "content": "## 项目指令\n...\n\n## 自动记忆\n- [偏好 any 语法](user-prefers-any.md) — ...\n- [项目用 golang-migrate](project-migration-tool.md) — ..." },
    { "role": "assistant", "content": "好的，我已了解项目背景。" },
    { "role": "user", "content": "帮我在 handler 里加一个用户注册接口" }
  ],
  "tools": [ ... ]
}


```

项目指令和自动记忆索引作为上下文注入 messages。Agent 在处理用户的实际请求之前，已经「读过」了所有长期记忆的索引。如果某条记忆的详细内容跟当前任务相关，Agent 可以用 ReadFile 去读对应的记忆文件。

![图片展示了API请求结构，分为system、messages和assistant应答三部分。system部分为角色设定（很短）；messages部分包含环境信息+项目指令、自动记忆（memories.md）、assistant应答、用户实际请求等，其中自动记忆（memories.md）被标注为长期记忆，每次自动注入。该图与上下文紧密相关，上下文提到Agent在处理用户请求前已读过长期记忆索引，若相关可读对应记忆文件，此图直观呈现了这一请求结构。](https://feishu.cn/file/EQvXbRO6foDK3OxAXP1cveAUn4J)



---

## 记忆治理：让 Agent 自己「整理笔记」

自动提取解决了「记」的问题，但光记不整理，用久了记忆目录会变成这样：三个文件都在说「不要自动 push」，两个文件关于项目截止日期但一个写的是去年的，还有一条记忆说「数据库用 MySQL」但项目早就换成了 PostgreSQL。

这跟你的笔记本一个道理：只管往里写不管翻回去整理，最后翻开全是重复和过时的内容，有用的信息反而被淹没了。更实际的问题是，`MEMORY.md` 索引有 200 行的上限，垃圾记忆占满了配额，真正有价值的新记忆就没地方放了。

MewCode 的解决方案是在后台定期跑一次「记忆治理」：fork 一个子 Agent，让它回顾现有记忆，合并重复的，删掉过时的，修正矛盾的，顺便整理一下索引。这个机制在 Claude Code 里叫 autoDream，名字很有意思，像是 Agent 在「做梦」的时候整理白天学到的东西。

![图片展示了两种记忆管理方式。左侧为“只记不整理”状态，笔记本上贴着多张写有“不要push”字样的便签，还有标注“MySQL”“PostgreSQL”的便签，显得杂乱。右侧为“定期整理”状态，笔记本上只有一张写有“不要自动push”的便签，整洁有序。图片与上下文紧密相关，直观呈现了MewCode在后台定期跑“记忆治理”机制，即fork子Agent回顾、整理记忆的思路，与只记不整理、记忆混乱的情况形成对比。](https://feishu.cn/file/XuwrbHVJOoGL2rxO4XqcpvTbnOb)

### 什么时候触发

记忆治理不需要每轮都跑，跑一次的成本不低（要调一次 LLM），所以用一串门控条件层层过滤：目录是否存在、距上次整理是否超过 24 小时、10 分钟内是否已经扫描过、累积的会话是否达到 5 个、能否拿到锁。五道关卡从轻到重依次排列，前面的通不过就不走后面更重的检查，绝大多数时候在第一两道就直接返回了。

检查的时机跟自动提取一样，都挂在每轮 Agent Loop 完成后。但检查本身很便宜，一次 stat 读一下锁文件的修改时间就知道距离上次整理过了多久，绝大多数时候在时间门这一步就直接返回了。只有时间门通过后才去扫描会话目录数会话数，而且扫描本身也有 10 分钟的节流，不会每轮都扫。

```
function MaybeRun():
    if 记忆目录不存在: return
    if 距上次整理 < 24 小时: return        // 时间门
    if 10 分钟内已经扫描过: return          // 扫描节流
    if 累积会话数 < 5: return              // 会话门
    if 获取锁失败: return                  // 防并发
    后台执行整理()

```

为什么不做成定时任务？因为 MewCode 是命令行工具，不是常驻进程，没有 cron 可以挂。只能在 Agent 活跃的时候顺带检查一下，这种「懒检查」的方式足够了。

![该图片展示了Agent完成Loop后触发记忆整理检查的流程：首先进入名为“时间门”的判断环节，检查距离上次整理是否达到24小时，若未达标则直接跳过整理流程；若满足时间条件，则进入“会话门”环节，判断累积会话数是否达到5个，未达标准同样跳过流程。只有同时满足时间门与会话门的条件，才会执行获取锁、后台整理的操作，该流程对应文档中所述的Agent活跃时的“懒检查”记忆治理机制，契合命令行工具的运行特点。](https://feishu.cn/file/AZ2xbu32hoCeFhxpy9jcr30Gnue)

### 防并发：锁文件

如果你同时开了两个终端窗口跑 MewCode，两个进程可能同时触发整理。两个子 Agent 同时改同一批记忆文件，结果就乱了。

MewCode 用一个锁文件 `.mewcode/memory/.consolidate-lock` 来防止这种情况。这个文件干两件事：文件内容存持有者的 PID，文件的 mtime 在获取锁时被刷新——写入 PID 的同时 mtime 更新为当前时间，整理完成后子进程退出不再修改文件，mtime 就停留在那个时间点，近似等于上次整理完成的时间。一个文件两个用途，不需要额外的状态文件。

获取锁的流程不复杂：读锁文件的 mtime 和 PID，如果文件存在、mtime 距今不到 1 小时、而且 PID 对应的进程还活着，说明别人正在整理，放弃。否则写入自己的 PID，回读一次确认没被抢。

如果整理失败了，用 utimes 把 mtime 改回获取前的值，这样时间门下次还能通过，不会因为一次失败就卡住。如果进程崩溃了，PID 残留在那里，下次启动时检测到进程已死，自动回收锁。超过 1 小时没释放的锁也视为过期，即使 PID 复用了也不会永久阻塞。

### 整理过程

整理由一个后台 fork 出的子 Agent 执行，不阻塞用户当前的交互。子 Agent 拿到的工具集做了限制：Bash 只能用只读命令（ls、grep、cat 这些），文件写入只允许在记忆目录内。防止整理过程意外改到项目代码。

整理 prompt 分四个阶段引导子 Agent 工作：

**定位阶段**：ls 记忆目录，读 MEMORY.md 索引，浏览现有记忆文件。先搞清楚当前有什么，再决定怎么改。

**收集信号**：grep 最近的会话记录，看有没有新信息值得纳入记忆，或者有没有旧记忆跟现状矛盾的。不全量读会话记录，只针对性地搜。

**整理**：这是核心步骤。合并内容重复的记忆到同一个文件（比如三个都在说「不要 push」的合成一个），删除已经被证伪的旧记忆，修正矛盾（两条记忆说的是相反的事，修正错的那条），把「昨天」「上周」这类相对日期转成绝对日期（否则过两个月再看就不知道说的是哪天了）。

**修剪索引**：更新 MEMORY.md，删掉指向已经不存在或已过时记忆的指针，压缩过长的索引行，把新增的重要记忆加进去，确保索引维持在 200 行 / 25KB 以内。

整个过程是 LLM 驱动的，不是硬编码的规则引擎。什么算「重复」、什么算「过时」、什么值得保留，全由模型判断。这跟自动提取的思路一致：能交给模型做的判断就不自己写规则。

### 提取与治理的协作

自动提取和记忆治理是记忆系统的两只手：提取管「写」，治理管「整理」。提取每轮 Agent Loop 结束后都可能触发，频率高但单次影响小。治理隔天才跑一次，频率低但会批量处理累积的问题。

两者的关系像是记账和月末对账。每天的收支你随手记下来（提取），月末坐下来把账本翻一遍，合并重复的条目，删掉记错的（治理）。

![图片以“记忆系统的两只手”为标题，展示了记忆提取与治理的协作关系。左侧为提取，每轮都可能触发，有管写标识；右侧为治理，隔天才跑一次，有管整理标识。中间是一个手拿笔和放大镜的卡通人物，手拿笔在文件夹中记录，手拿放大镜在文件夹中查找，象征记忆的提取与治理。该图与上下文内容相呼应，形象说明治理隔天才跑一次，频率低但会批量处理累积问题，两者关系类似记账和月末对账。](https://feishu.cn/file/Dhl4bJBlioCgo4xIhLGcqFeNnSh)

整理完成后，如果修改了记忆文件，会在主 Agent 的对话里插入一条通知，类似自动提取完成后的「Memory saved: xxx.md」，让你知道后台整理了哪些记忆。

---

## 本章小结

记忆系统让 Agent 从「每次失忆」变成了「越用越懂你」。两层记忆各司其职：工作记忆（上下文窗口）管当前任务，长期记忆（会话持久化 + 指令文件 + 自动记忆）让项目知识、对话历史和用户偏好跨会话生效。

项目指令文件是最直观的长期记忆：一个 MEWCODE.md，Agent 每次启动都读一遍，相当于员工入职文档。多层优先级加上 `@` 引用模块化，让指令体系既能统一管理又能灵活扩展。

会话持久化选用 JSONL 格式，追加写入 O(1)、崩溃安全、增量恢复，天然适合 Agent 的流式工作模式。恢复时的消息链验证和时间跨度提示，让中断后的体验尽可能平滑。

自动记忆让 Agent 自己「记笔记」，在每轮对话结束后异步提取值得长期记住的信息，写入独立的记忆文件并更新 `MEMORY.md` 索引。跟 MEWCODE.md 用同一套 Markdown 格式和加载管线，去重交给提取 LLM 判断，不需要额外的相似度算法。记忆治理在后台定期整理这些积累的记忆，合并重复、删除过时、修正矛盾，保持索引在合理范围内。提取管「写」，治理管「整理」，两者配合让记忆系统不会随着使用时间变长而退化。

记忆层至此告一段落。但你应该注意到了，MewCode 到现在为止还是个"被动响应"的工具——所有操作都得用自然语言描述，连「清空当前会话」「看一下当前状态」「列出历史会话」这种高频动作也不例外。下一章我们要给 MewCode 加上 Slash Command 内置命令框架，把 `/clear`、`/status`、`/session list` 这些一键直达的入口接进来，让交互层从"全靠对话"过渡到"对话 + 命令并用"。
