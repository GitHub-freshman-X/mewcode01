# 理论学习：Slash Command 命令框架

> **架构定位**：本章实现的是 交互层的命令框架。它让常用操作绕过 Agent 引擎直接执行，省 Token 又快。

---

## 杀鸡焉用牛刀

用了几章下来，MewCode 已经能跟你对话、调用工具、自主决策了。但你有没有发现一个让人哭笑不得的事情？

比如你想清屏。你在输入框里打了一句 `清除对话`，Agent 认认真真地思考了一下，然后回复你：好的，我帮你清除了对话内容。但实际上它根本没有清屏，因为清屏是 UI 层的操作，LLM 根本做不了。这一来一回，白白消耗了几百个 token。

再比如你想看看当前用了多少 token。你输入 `告诉我现在用了多少 token`，Agent 又发了一次 API 请求，消耗 token 来告诉你还剩多少 token。你品品，这是不是有点讽刺？

又比如 Plan Mode 切换。第四章（ReAct 范式与 Agent Loop）实现了 Plan Mode 的底层逻辑，但切换操作本身就是 UI 层的事，根本不需要走 Agent Loop。因为切换模式这种事，跟 AI 没有半毛钱关系。

这些操作有一个共同特点：它们不需要 AI 参与。清屏就是清屏，查 token 就是查 token，切换模式就是切换模式。让 LLM 来做这些事，就像让一个博士去帮你按电灯开关。

![杀鸡焉用牛刀](https://feishu.cn/file/CgpibVW2poeBzhxJ5iecgTRnnYc)

那怎么办？我们需要一条快车道，让这类操作直接在本地执行，不走 Agent Loop，不消耗 token，响应速度是毫秒级的。

这就是 Slash Command 系统要做的事情。所有以 `/` 开头的输入都会被命令解析器拦截，绕过 LLM，直接在本地处理。

![Slash Command 快车道与 Agent Loop](https://feishu.cn/file/B6nsbOqYpoPuVXx6ZK8crMbNn1g)

---

## 一个命令框架要解决什么问题

你可能觉得，这不就是写一堆 `if input == "/help"` 的条件分支吗？

如果只有两三个命令，确实可以这样硬编码。但 MewCode 要支持 10 个内置命令，以后还会有更多。而且每个命令有自己的别名、参数格式、帮助文本。如果全部用 if-else 硬编码，代码会变得乱成一团，改一个地方就牵连一片。

![if-else 意大利面与命令框架](https://feishu.cn/file/SKT0b2v3RoVXZBx1XfxcP45Sn5b)

所以我们需要一个命令框架。一个好的命令框架要解决三个问题：注册、解析、执行。

### 注册：命令的身份证

每个命令需要声明自己是谁、能干什么、怎么用。这些信息要集中管理，不能散落在代码各处。

用伪代码来描述一个命令的定义：

```Plain Text
Command:
    name        字符串       // 命令名，如 "compact"
    aliases     字符串列表    // 别名，如 ["c"]
    description 字符串       // 简短描述
    usage       字符串       // 用法示例
    type        CommandType  // 命令类型（后面会详细讲）
    argPrompt   字符串       // 参数提示语（可选）
    hidden      布尔值       // 是否在帮助列表中隐藏
    handler     函数         // 执行函数

```

大部分字段从名字就能猜到用途，重点说几个。`hidden` 用来标记那些不想暴露给用户的内部命令，比如调试用的命令，设为 `true` 后 `/help` 就不会列出它。`handler` 是真正干活的函数，框架只管分发，具体逻辑全在 `handler` 里。`type` 区分命令的执行模式，后面会详细展开。

![Command 身份证](https://feishu.cn/file/YAcjbbip5oVWc3xZpUbcvCOCntg)

注册的方式是声明式的，把所有命令定义集中在一个地方。拿 `/help` 举例：

```Plain Text
registry.register(Command{
    name:        "help",
    aliases:     ["h", "?"],
    description: "显示帮助信息",
    type:        LOCAL,
    handler:     handleHelp,
})

```

每个命令填好自己的身份信息，然后交给 `registry` 统一管理。其他命令的注册方式完全一样，只是字段值不同。

看起来是不是很像路由注册？Web 框架里你写 `router.GET("/users", handleUsers)`，这里你写 `registry.register(Command{...})`。思路完全一样，声明式注册，框架负责分发。

![Web Router 与 Command Registry](https://feishu.cn/file/VU1jbmDEVoUMzSx4l22cQKucnec)

还有一个细节值得提前考虑：注册中心内部需要用读写锁保护并发安全。当前所有命令都在启动时注册，看起来用不上。但下一章的 Skill 系统会在运行时动态注册新命令，那时候注册和查找可能同时发生，锁就是必需的了。

![Registry 读写锁](https://feishu.cn/file/Pb24bPckvoLuzoxIOr9c90GYn70)

### 解析：拆出命令名和参数

用户输入 `/compact 保留数据库相关内容`，解析器要能把它拆成两部分：命令名 `compact` 和参数 `保留数据库相关内容`。

```Plain Text
function parseCommand(input):
    input = trimSpace(input)
    if not input.startsWith("/"):
        return "", "", false

    // 去掉 /
    input = input[1:]

    // 用第一个空格分割命令名和参数
    parts = splitOnce(input, " ")
    name = toLower(parts[0])
    args = parts.length > 1 ? trimSpace(parts[1]) : ""

    return name, args, true

```

整个流程一目了然：以 `/` 开头的就是命令，第一个空格之前是命令名，之后是参数。命令名转小写，这样 `/Help` 和 `/help` 效果一样。

![Slash Command 解析拆分](https://feishu.cn/file/Sg2rbUaPfoTaS8xviJPcJ7Ronff)

### 执行：统一的 Handler 签名

命令的执行函数签名是统一的：

```Plain Text
CommandHandler = function(ctx: CommandContext) -> error

CommandContext:
    args         字符串           // 原始参数字符串
    agent        Agent实例        // Agent 实例
    conversation Conversation实例 // 当前对话
    session      Session实例      // 当前会话
    ui           UIController     // UI 控制接口
    config       Config           // 全局配置

```

`CommandContext` 把命令执行所需的一切资源打包在一起。通过它，命令可以操作 Agent、修改对话、更新 UI、读取配置。每个 Handler 函数不需要关心怎么拿到这些依赖，框架已经准备好了。

![CommandContext 背包](https://feishu.cn/file/WbElbWWCool9zBxmU6UcsMiPn2b)

那 `UIController` 是什么？它是 UI 层暴露给命令的操作接口：

```Plain Text
interface UIController:
    addSystemMessage(text)       // 显示一条系统消息
    sendUserMessage(text)        // 将文本作为用户消息发送给 Agent
    setPlanMode(enabled)         // 切换 Plan Mode
    getTokenCount() -> int       // 获取当前 token 数
    refreshStatus()              // 刷新状态栏

```

为什么要抽一个接口出来？因为命令不应该知道 UI 的实现细节。它只需要知道自己能显示一条系统消息、能发一条消息给 Agent 就够了。至于 UI 底层用的是什么框架、怎么渲染，命令完全不关心。

![UIController 接口边界](https://feishu.cn/file/E6gTbR9eOoFs0pxK2TWc9KIGnxe)

---

## 不是所有命令都一个干法

框架搭好了，但有一个问题：不是所有命令的执行方式都一样。

`/clear` 开启新对话，会重置 UI 状态。`/plan` 切换模式，也是本地操作，但它会改变整个 UI 的行为状态。而 `/review` 审查代码则需要把一段精心构造的 prompt 发送给 Agent，让 AI 来干活。

这三种情况差别很大，不能一视同仁。所以我们把命令分成三种类型。

最简单的是 `local`，不走 Agent Loop 的命令。handler 自己把活干完，直接返回结果，以系统消息的形式显示在 UI 中。`/help`、`/status`、/compact 都属于这一类。注意，「不走 Agent Loop」不等于「不碰 LLM」。/compact 内部调了 LLM 来生成摘要，但那个 LLM 调用是 handler 自己管理的，不经过 Agent Loop 的多轮推理，所以仍然是 local。

但有些命令不只是显示一条消息。比如 `/plan` 切换到 Plan Mode 后，状态栏需要更新，工具可用性需要变化，整个 UI 的行为模式都变了。这类命令叫 `local-ui`，同样不走 Agent Loop，但需要渲染交互式 UI 或改变界面状态。

还有一类命令根本不在本地执行。`/review` 审查代码，本质上是把一段预设的 prompt 发送给 Agent，让 AI 来处理。这类叫 `prompt`，命令系统只负责构造消息，实际干活的还是 Agent。

![三种命令类型](https://feishu.cn/file/Bq04bSd1oosj5mxfuJAcfa76nHh)

你可能会问：`prompt` 类型不就是让用户少打几个字吗？为什么还要专门搞一个类型？

没错，本质上就是带快捷方式的 prompt。但这个价值比你想象的大。

比如 `/review` 命令。你不需要手动打 `请审查当前 git diff 中的代码变更，重点关注逻辑错误、安全问题、性能问题、代码风格` 这一长串，只需要输入 `/review`，命令系统会生成一段预设的 prompt 发送给 Agent：

```Plain Text
function handleReview(ctx):
    prompt = "请审查当前 git diff 中的代码变更。重点关注：\n" +
        "1. 逻辑错误\n2. 安全问题\n3. 性能问题\n4. 代码风格"
    if ctx.args != "":
        prompt += "\n\n额外关注：" + ctx.args
    ctx.ui.sendUserMessage(prompt)

```

这类命令会消耗 token，因为要走 Agent Loop，但为用户省去了记忆和输入复杂 prompt 的麻烦。而且 prompt 经过精心设计，质量比用户随手打的要好得多。

![/review prompt 快捷方式](https://feishu.cn/file/IpM8buocRovYNqxsLrkcdCstnse)

---

## 别名：让常用操作更顺手

好的 CLI 工具都支持别名。你用 npm 的时候，`npm i` 比 `npm install` 顺手多了。同样的道理，`/c` 比 `/compact` 少打 6 个字符，`/h` 比 `/help` 少打 3 个字符。别小看这几个字，高频操作省下来的时间是可观的。

![命令别名快捷路径](https://feishu.cn/file/Ctaeb0CuuoYKcxxEUpTcDsQ0nNh)

别名在命令定义时就声明好了，解析在命令查找阶段完成：

```Plain Text
function find(registry, name):
    // 先精确匹配命令名
    if name in registry.commands:
        return registry.commands[name]

    // 再匹配别名
    for cmd in registry.commands.values():
        for alias in cmd.aliases:
            if alias == name:
                return cmd

    return null

```

先按命令名精确匹配，找不到再遍历所有命令的别名。用户输入 `/c`，先找有没有叫 `c` 的命令，没有，再看别名列表，发现 `compact` 的别名里有 `c`，匹配上了。

有一个细节值得注意：如果别名有冲突怎么办？比如两个命令都声明了 `/s` 作为别名。这种情况应该在注册的时候就检测出来。具体做法是启动阶段直接 panic 退出，让开发者在编译期解决，别等到用户用的时候才发现行为不确定。

---

## 参数提示：别让用户猜格式

有些命令需要参数，但用户可能不记得参数格式。比如你把恢复会话做成一个独立命令 `/resume <id>`，用户只输入 `/resume`，系统应该提示他补上会话 ID，别直接甩一个 `缺少参数` 的错误。

这就是 `argPrompt` 字段的作用：

```Plain Text
Command{
    name:      "resume",
    usage:     "/resume <id>",
    argPrompt: "用法：/resume <id>",
    handler:   handleResume,
}

```

当用户输入 `/resume` 不带参数时，如果设置了 `argPrompt`，UI 会显示这段提示信息。用户一看就知道该怎么用了，省得翻文档。

![argPrompt 参数提示](https://feishu.cn/file/BOJhbqGX4oPtCFxTUF7c0Wsvnvc)

---

## Tab 补全：进一步减少输入

除了别名，Tab 补全也是提升效率的利器。在输入框中输入 `/` 后按 Tab，应该显示所有可用命令。输入 `/com` 后按 Tab，应该自动补全为 `/compact`。

```Plain Text
function complete(registry, prefix):
    prefix = removePrefix(prefix, "/")
    matches = []
    for cmd in registry.commands.values():
        if cmd.hidden:
            continue
        if cmd.name.startsWith(prefix):
            matches.append("/" + cmd.name)
        for alias in cmd.aliases:
            if alias.startsWith(prefix):
                matches.append("/" + alias)
    sort(matches)
    return matches

```

补全结果在 UI 中以下拉列表的形式展示。如果只有一个匹配，直接补全到输入框，省得用户再按一下。如果有多个匹配，比如输入 `/s` 同时匹配到 `/session` 和 `/status`，就列出来让用户选。

![Tab 补全建议](https://feishu.cn/file/HfS8b1QJKoZylgxJAR1c18U2n2g)

---

## 命令系统怎么跟 UI 事件循环集成

现在我们有了注册、解析、别名、补全这一套机制，最关键的一步是把它插到 UI 的事件循环里。

命令拦截的时机很重要：必须在消息发送给 Agent 之前。用户按下回车，先判断输入是不是命令，是命令就走命令系统处理，不是命令才发给 Agent。

![输入分流器](https://feishu.cn/file/D3PMbhUw6opjcbx7k4pc3bwin4e)

先看第一步，拦截和解析：

```Plain Text
function handleEnter(input):
    input = trimSpace(input)
    resetInputBox()

    if input == "":
        return

    name, args, isCommand = parseCommand(input)
    if not isCommand:
        sendToAgent(input)
        return

```

输入为空时直接返回，别让用户不小心按了回车就触发一次 API 调用。不是命令的输入直接发给 Agent，命令系统不介入。

如果是命令，接下来是查找和执行：

```Plain Text
    if name == "":
        showCommandList(registry)
        return

    cmd = registry.find(name)
    if cmd == null:
        addSystemMessage("未知命令：/%s，输入 /help 查看可用命令", name)
        return

    if args == "" and cmd.argPrompt != "":
        addSystemMessage(cmd.argPrompt)
        return
    ctx = buildCommandContext(args)
    cmd.handler(ctx)

```

这里有几个处理细节值得注意。

只输入了 `/` 没有命令名时，直接列出可用命令。用户可能只是想看看有哪些命令，给他一个快速入口比报错友好得多。

命令找不到时，错误信息里带上 `/help` 引导，告诉用户去哪查可用命令。这个原则贯穿整个 MewCode：错误提示永远带引导。

有些命令被定义为必须带参数，但用户没提供，这时候显示 `argPrompt` 的提示信息，比冷冰冰的 `参数缺失` 友好多了。

![未知命令引导到 /help](https://feishu.cn/file/AkubbT4vmos5WTxuK1Bcwncjnkd)

状态栏也要配合更新，在底部显示当前模式和快捷命令提示：

```Plain Text
[DEFAULT] tokens: 45,230/200k | /help 查看命令

```

Plan Mode 时切换为：

```Plain Text
[PLAN] tokens: 45,230/200k | /plan 退出 Plan 模式

```

> []

---

## 十个核心命令

理论讲够了，来看看 MewCode v1 到底内置了哪些命令。一共 9 个，按前面说的三种类型来分组介绍。

![MewCode 命令全家福](https://feishu.cn/file/G6f1bqWJ2odxA5xROhNcqxAmnPg)

### 本地命令

先看六个 `local` 类命令，不走 Agent Loop，handler 执行完直接返回结果。

`/help` 是你的入口，简写 `/h` 或 `/?`。不带参数时列出所有可用命令的名称、别名和简要说明：

```Plain
可用命令：
  /help, /h, /?        显示帮助信息
  /compact, /c         压缩上下文
  /clear               清除对话历史
  /plan, /p            切换到 Plan 模式
  /session             会话管理
  /memory              记忆管理
  /status, /s          显示状态信息
  /review              审查代码变更

输入 /help <命令名> 查看详细用法。

```

带参数时显示某个命令的详细用法，比如 `/help session` 会告诉你 session 的子命令和参数格式。

`/compact` 手动触发上下文压缩，简写 `/c`。前面已经实现了压缩逻辑，现在把它正式纳入命令框架。不带参数就执行标准压缩，带参数可以指定保留重点，比如 `/compact 保留数据库相关内容`。如果当前上下文还不到 5000 token，直接提示你 `无需压缩`，免得浪费。

压缩完成后显示前后的 token 对比，让你直观感受到效果。

`/session` 会话管理命令。不带任何参数时显示当前会话的概要，包括会话 ID 和消息数。想接着昨天的工作？`/session list` 看一眼历史清单，找到对应 ID 后 `/session resume <id>` 一键切回去。要彻底换上下文从头开始？`/session new` 起一个干净的会话。某个会话以后不会再用？`/session delete <id>` 收尾。

`/memory` 记忆管理命令。不带参数显示记忆概要，让你随时了解 Agent 当前记住了什么。想看完整的记忆列表用 `/memory list`。如果有一条信息你担心 Agent 自动遗忘，可以 `/memory add <类别> <内容>` 手动钉上去。想清空全部自动记忆从头开始？`/memory clear` 会先弹一个确认提示，免得手滑误删。

`/status` 显示当前的综合状态信息，简写 `/s`：

```Plain Text
MewCode 状态
─────────────
模式：default
Token：45,230 / 200,000（23%）
工具：6 个已启用
记忆：user 3 条，project 5 条
工作目录：/home/user/project
版本：v0.8.0

```

这个命令在调试和确认配置时特别有用。当你觉得 Agent 的行为不太对，先 `/status` 看一眼，很可能一下子就发现问题了，比如权限模式不对，或者 token 快用完了。

### 影响 UI 状态的命令

接下来是两个 local-ui 类命令，它们除了本地执行，还会改变 UI 的行为模式。

`/clear` 开启新对话。当前会话会被关闭并保存到会话历史中，然后创建一个全新的会话。旧对话不会丢失，随时可以通过 `/session list` 查看、`/session resume` 恢复。因为它会重置整个界面状态，所以归为 `local-ui` 类型。

`/plan` 切换 Plan Mode，简写 `/p`。它是一个 toggle：不在 Plan Mode 时输入 /plan 进入，已经在 Plan Mode 时再输入 /plan 退出。带参数则切换模式的同时把参数作为任务描述发送给 Agent。比如 `/plan 设计用户认证模块` 会同时完成两件事：切换到 Plan Mode，并让 Agent 开始规划认证模块的实现方案。

### 转发给 Agent 的命令

最后是 `/review`，唯一的 `prompt` 类命令。输入后命令系统会把预设的代码审查 prompt 发送给 Agent，分析当前 git diff 中的变更。带参数时可以指定额外关注点，比如 `/review 特别注意并发安全`。这个命令会消耗 token，因为要走 Agent Loop。前面讲 `prompt` 类型时的 `handleReview` 伪代码就是它的实现。

---

## 命令系统的边界

这一章我们给 MewCode 装上了一套内置命令框架。现在 MewCode 有了两条处理路径：`/` 开头的输入走命令系统本地处理，普通输入走 Agent Loop 调用 LLM。常用操作不消耗 token，响应速度是毫秒级。命令的行为是确定的，不受模型随机性影响。新增命令只需注册，不影响 Agent 逻辑。

命令系统就像 IDE 里的快捷键。有些操作你用菜单也能做，但快捷键让常用操作快了一个数量级。

不过 Slash Command 有一个局限：它只能执行预定义的、程序化的操作。所有的 Handler 都是硬编码在源码里的，改一个 prompt 就得重新编译。而且用户没法自己添加新命令。如果你想让命令也能利用 AI 的能力，比如一个 `/commit` 命令能自动分析 git diff、生成 commit message、执行提交，那就需要下一章的 Skill 系统了。

这条快车道的注册中心本身是纯本地的，但它能开放给外部。Claude Code 把 MCP 服务器通过 `prompts/list` 暴露的 prompt 自动包装成 `mcp__<server>__<prompt>` 形式的命令，跟内置命令共用同一套分发链路。MewCode 把这种「由外部贡献命令」的能力也放到下一章的 Skill 系统里统一处理。

![Slash Command 到 Skill 系统的边界](https://feishu.cn/file/Gy4fbSpeTot6b5xn3tbctgXhnce)