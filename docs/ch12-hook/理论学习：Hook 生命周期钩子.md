# 理论学习：Hook 生命周期钩子

![Hook 引擎所在的工具层局部架构图](https://feishu.cn/file/SxoJb3jdKoMSvUxIN3IcRNS0nNd)

> **架构定位**：本章实现 ③ 工具层的 Hook 系统。Hook 在工具执行的生命周期上挂载自动化动作，是工具层和安全层的桥梁。

---

## 那些你一直在重复做的事

用了 MewCode 一段时间之后，你会发现自己养成了一些「习惯」。

每次 Agent 写完一个源代码文件，你手动跑一遍格式化工具。每次 Agent 修改了 `.proto` 文件，你手动跑代码生成器。每次 Agent 准备执行 `rm -rf /` 这样的命令，你紧张地盯着审批弹窗，生怕手滑点了 Allow。每次开始新对话，你都要先敲一句「请先读一下 ARCHITECTURE.md 了解项目结构」。

你做了很多次，但每次都是一样的动作。

![开发者反复手动做同样操作的人肉 CI 场景](https://feishu.cn/file/RZUvbdefkouJG6xemwfc0mqWnbh)

这些操作有一个共同的特点：**触发条件明确，执行动作固定**。「Agent 写了代码文件」就跑格式化。「Agent 要执行危险命令」就拦截。「新对话开始」就注入项目上下文。条件是确定的，动作也是确定的，每次一模一样。

那为什么要让人来做这件事？

这不就是事件驱动吗？某个事件发生了，满足某个条件，自动执行某个动作。不需要你在那盯着，不需要你手动操作。

这就是 Hook 系统要做的事情。**让你在 Agent 的生命周期事件上挂载自动化动作。** 事件发生时，Hook 自动执行。你省下来的精力可以去思考更重要的问题，不用再当人肉 CI。

![手动重复操作与 Hook 自动化的对比图](https://feishu.cn/file/VXbybMkxOoo54ZxsTgUcGsctnaf)

---

## Hook 的三要素

一个 Hook 由三部分组成：事件、条件、动作。用人话说就是「什么时候」「什么情况下」「做什么」。

![Hook 的事件、条件、动作三要素示意图](https://feishu.cn/file/TSdRbNzsNoX3NfxyLr4cZcIUnpb)

直接看一个例子：

```YAML
hooks:
  - event: post_tool_use       # 事件：工具执行之后
    if: tool == "WriteFile"    # 条件：只在写文件时触发
    action:                     # 动作：执行什么
      type: command
      command: "lint $FILE_PATH"

```

这个 Hook 说的是：每当 Agent 用 WriteFile 工具写了一个文件之后，自动跑一下 lint 检查代码质量。`$FILE_PATH` 是一个上下文变量，会被替换成实际的文件路径。

三要素分开来看就很清晰：事件是 `post_tool_use`，工具执行后触发；条件是 `tool == "WriteFile"`，只在写文件时触发；动作是执行 lint 命令。

这三个部分缺一不可吗？条件可以省略。如果不写 `if`，就表示「无条件执行」。比如你想在每次会话开始时都注入项目上下文，不需要任何条件判断。但事件和动作是必须有的。没有事件不知道什么时候触发，没有动作触发了也没用。

那这套 YAML 写在哪儿？MewCode 会在三个地方找 Hook 配置。最常用的是项目根目录下的 `.mewcode/config.yaml`，跟着项目走、随仓库一起被人 review。如果你想在多个项目间共享一份配置，可以放到 `~/.mewcode/config.yaml`，所有项目都能读到它。还有一个 `.mewcode/config.local.yaml`，给你本地的临时改动用，不会被 git 追踪，优先级也最高。三个文件按顺序加载，但和 Skill 那种「找到一个就停」的查找语义不一样：Hook 配置是**追加合并**的，用户级和项目级声明的 Hook 都会同时生效，叠加起来用。

接下来我们深入看每个要素。

---

## 事件：Agent 生命周期中的关键时刻

事件是 Hook 的触发时机。你可以把 Agent 的整个运行过程想象成一条时间线，上面有很多关键节点。每个节点就是一个事件。

MewCode 定义了十几种生命周期事件，覆盖了 Agent 运行的各个阶段。我们按类别来看。

![Agent 生命周期事件总览时间线](https://feishu.cn/file/LNiNbTxrooo3MMxKOZDcDuTYn5d)

先是**会话级事件**。session_start 在新会话开始时触发，session_end 在会话结束时触发。一个会话从开到关，这两个事件各自只发生一次。再细一层是**轮次级事件**。turn_start 在用户发送新消息时触发，标志着一轮对话的开始；turn_end 在 Agent 完成回复时触发，标志着一轮对话的结束。一个会话里通常有很多轮对话，所以这两个事件会反复出现，颗粒度比会话级细很多。

然后是**工具级事件**，这也是最常用的。`pre_tool_use` 在工具执行**之前**触发，`post_tool_use` 在工具执行**之后**触发。注意「之前」和「之后」的区别非常重要，后面会专门讲。

还有**消息级事件**。`pre_send` 在消息发送给 LLM 之前触发，`post_receive` 在收到 LLM 响应之后触发。这两个事件可以用来做消息的预处理和后处理。

最后是**系统级事件**，数量最多。`startup` 和 `shutdown` 分别在 MewCode 启动和退出时触发。`error` 在发生错误时触发。`compact` 在上下文压缩时触发。

还有三个偏场景化的：`permission_request` 在权限审批请求时触发，`file_change` 在文件被修改时触发，`command_execute` 在 Slash Command 执行时触发。

这么多事件，哪些最常用？根据实际使用经验，**pre_tool_use 和 post_tool_use 占了绝大多数场景**。写文件后自动格式化？`post_tool_use`。拦截危险命令？`pre_tool_use`。修改 proto 文件后自动生成代码？`post_tool_use`。

其中 `pre_tool_use` 有一个特殊的能力，是其他所有事件都没有的：**它可以拦截工具的执行**。这一点太重要了，我们单独拿出来说。

![pre_tool_use 是唯一可拦截事件的强调图](https://feishu.cn/file/IlPMbPFkaohfTFxZT9OcPhH7nXf)

---

## pre_tool_use：唯一能说「不」的事件

`pre_tool_use` 是整个 Hook 系统里最有价值的事件。其他事件都是「通知型」的：事情发生了，告诉你一声，你可以做点额外的操作。但 `pre_tool_use` 不一样，它发生在工具执行**之前**，你可以在这个时刻决定：**允许，还是拒绝**。

想象一个场景：你的项目里有些文件不应该被 Agent 修改。`package-lock.json` 应该由包管理工具自动生成，不应该手动改。`.github/workflows/` 里是 CI 配置，改错了可能把整个 CI 搞坏。`vendor/` 或 `node_modules/` 是第三方代码，不应该手动修改。

用权限系统当然也能做这件事，禁止 Agent 写特定路径。但权限系统是静态的，写在配置里就不变了。如果你想根据工具参数的**内容**来决定是否允许呢？比如「允许执行 `rm` 命令，但不允许 `rm -rf /`」。权限系统做不到这么细粒度的判断，Hook 可以。

![权限系统与 Hook 系统控制粒度对比](https://feishu.cn/file/BZlkbwryJoWvquxuWgPcsDPynDb)

```YAML
hooks:
  - event: pre_tool_use
    if: tool == "WriteFile" && args.path ~= "package-lock.json"
    action:
      type: command
      command: "echo 'REJECT: package-lock.json 应该由 npm install 生成，不要手动修改'"
    reject: true

```

看到那个 `reject: true` 了吗？这是 `pre_tool_use` 的特殊标记。当 Hook 设置了 `reject`，工具调用会被取消，Agent 会收到 Hook 输出的文本作为错误信息。Agent 可以根据这个错误信息调整策略，比如改用 `npm install` 来更新依赖。

`reject` 只能用在 `pre_tool_use` 事件上。这是逻辑上的必然。你想想，`post_tool_use` 的时候工具已经执行完了，再说「拒绝」有什么意义？

在代码里，`pre_tool_use` 的处理和其他事件不一样。它必须同步执行、等待结果、检查是否拒绝：

```Plain Text
function runPreToolHooks(hookContext):
    for hook in findMatchingHooks("pre_tool_use", hookContext):
        hook.markExecuted()
        result = executeAction(hook.action, hookContext)
        if hook.reject:
            return ToolRejectedError{
                tool:   hookContext.toolName,
                reason: result.output,
                hookID: hook.id
            }
    return null

```

关键在 `hook.reject` 这个判断。一旦某个 Hook 标记了 reject，引擎立即返回一个 `ToolRejectedError`，工具调用就被取消了。

这里还有个隐含的顺序约定值得说清楚：多个 Hook 匹配同一个事件时，引擎按它们在 YAML 里出现的先后顺序逐个执行；只要前面任何一个 Hook 标记了 reject，后面的 Hook 就完全不会跑。所以**写 Hook 时位置很重要**，把最该兜底的拦截规则放前面，不会被后面更宽松的 Hook 跳过。

Agent Loop 那边收到 ToolRejectedError 之后，把错误信息作为工具调用的结果返回给 LLM。LLM 看到这个错误，会理解「这个操作被拒绝了」，然后想办法换一种方式完成任务。这就形成了一个反馈循环：Hook 拦截了操作 → Agent 收到了原因 → Agent 调整策略。

![ToolRejectedError 触发的反馈循环图](https://feishu.cn/file/CZnmbcimyokdqgxViX3csuG6neb)

再举一个更实用的例子。拦截所有试图执行 `rm -rf /` 的命令：

```YAML
hooks:
  - event: pre_tool_use
    if: tool == "Bash" && args.command =~ /rm\s+-rf\s+\//
    action:
      type: command
      command: "echo '警告：检测到高危删除命令，已拦截'"
    reject: true

```

这里条件用了 `=~` 操作符做正则匹配，能匹配 `rm -rf /`、`rm  -rf  /` 等各种空格写法。比单纯的字符串匹配灵活得多。

---

## 条件语法：灵活但不复杂

条件决定 Hook 在事件触发时是否执行。MewCode 的条件语法复用了权限规则的匹配语法，你不需要学两套东西。

支持四种比较操作符。`==` 精确匹配，`!=` 反向匹配，`=~` 正则匹配，`~=` glob 匹配。支持 `&&` 表示且、`||` 表示或，来组合多个条件。

`=~` 和 `~=` 只是左右反转，第一眼很容易记混。一个小助记：等号在波浪号**前**面的 `=~` 是正则（regex 表达力强、张牙舞爪），波浪号在前的 `~=` 是 glob（glob 是文件路径通配，更温和）。顺便提一句，Go 版本实现里把 glob 操作符写成了 `=*`（参见 source-go.md），各语言实现的语法选择上略有差异，但理论模型本身一致。

这里说一下 glob 匹配，它和正则匹配容易搞混。glob 是文件系统里常用的通配符语法，比正则简单很多：`*` 匹配任意字符但不跨目录分隔符，`**` 匹配任意层级的路径，`?` 匹配单个字符。比如 `*.py` 匹配所有 Python 文件，`src/**/*.go` 匹配 src 下任意深度的 Go 文件。平时在 `.gitignore` 里写的就是 glob 语法。

![glob 通配符匹配目录树示例](https://feishu.cn/file/WcYWbuQ3RoppTCxFKULcQ01untf)

看几个实际的例子就明白了：

```YAML
# 工具名精确匹配
if: tool == "Bash"

# 工具参数的正则匹配
if: tool == "Bash" && args.command =~ /rm\s+-rf/

# 文件路径的 glob 匹配
if: tool == "WriteFile" && args.path ~= "*.py"

# 组合条件
if: tool == "WriteFile" && args.path ~= "*.proto"

# 反向匹配：除了 ReadFile 之外的所有工具
if: tool != "ReadFile"

```

条件里的 `tool` 指的是工具名，`args.xxx` 指的是工具参数中的某个字段。比如 `args.command` 是 Bash 工具的 `command` 参数，`args.path` 是 WriteFile 工具的 `path` 参数。

在实现上，条件表达式不需要一个完整的表达式引擎。写一个简单的解析器就够了：按 `&&` 或 `||` 拆分成多个子条件，每个子条件按空格拆分成 `field operator value` 三部分。组合逻辑也很直观，`and` 模式要求所有子条件都通过，`or` 模式任一通过即可。

![Hook 条件解析拆分流程图](https://feishu.cn/file/QuOHbtwYiovetnxdfb7cYWilnUg)

单个条件的求值就是根据操作符做对应的匹配：

```Plain Text
function Condition.evaluate(ctx):
    fieldValue = ctx.getField(field)
    switch operator:
        case "==": return fieldValue == value
        case "!=": return fieldValue != value
        case "=~": return regexMatch(value, fieldValue)
        case "~=": return globMatch(value, fieldValue)
    return false

```

四种操作符，四行分支，没有额外的魔法。`ctx.getField` 根据字段名从上下文里取值：`tool` 取工具名，`event` 取事件名，`args.xxx` 取工具参数里的对应字段。未知字段返回空字符串，不报错。

有一个设计决策需要说一下：`&&` 和 `||` 不能混用。也就是说，你可以写 `A && B && C`，也可以写 `A || B || C`，但不能写 `A && B || C`。为什么？一旦允许混用就涉及运算符优先级，需要一个完整的表达式解析器。这个复杂度对 Hook 条件来说完全没必要。如果你真的需要复杂逻辑，分成多个 Hook 就行了。

---

## 四种动作执行器

动作是 Hook 触发后执行的操作。MewCode 支持四种执行器，每种适用于不同的场景。

![四种 Hook 动作执行器总览](https://feishu.cn/file/A4dubyX4oofDEhxzIC9ciReynbj)

### command：执行 shell 命令

这是最常用的执行器。命令中可以使用上下文变量，Hook 引擎会在执行前进行变量替换。

```YAML
action:
  type: command
  command: "prettier --write $FILE_PATH"
  timeout: 10s

```

`$FILE_PATH` 会被替换成实际的文件路径。如果 Agent 写了 `src/components/Header.tsx`，那实际执行的就是 `prettier --write src/components/Header.tsx`。

command 执行器在底层就是启动一个 shell 子进程执行命令，捕获输出和退出码。`timeout` 字段控制命令的最长执行时间，超时后引擎会终止子进程并返回超时错误。格式化工具一般很快，但如果你挂的是一个跑测试的命令，设个合理的超时就很有必要了。

![command 执行器正常完成与超时对比](https://feishu.cn/file/WwiUbP7o2oTERlxiUo3cCoewnag)

### prompt：注入提示词

prompt 执行器不跑任何外部命令，它做的事就是给 Agent 塞一段提示词。这里有个容易踩的概念坑：所谓「塞一段」是以 system reminder 的形式（Hook 输出先包进 `<hook-notification>` 标签，再套一层 `<system-reminder>`）作为一条 user 消息追加到对话历史末尾，Agent 在下一轮请求时会读到。这样 system prompt 本身不变，prompt cache 前缀不受影响，同时这段提示词会随着上下文压缩被摘要掉，不会永久占用 token。适合在 `session_start` 或 `turn_start` 时给 Agent 补充上下文。

```YAML
action:
  type: prompt
  message: "请先阅读 ARCHITECTURE.md 了解项目结构，然后再开始工作。"

```

你可能会问：这跟直接在 system prompt 里写有什么区别？区别在于 Hook 是动态的。你可以加条件，比如只在特定项目里注入，或者只在第一次对话时注入，配合 `once: true` 就行。而 system prompt 是静态的，对所有场景生效。

![System Prompt 与 Hook Prompt 注入对比](https://feishu.cn/file/MinebXC9ZosNtNxuSeacJBEPnIg)

### http：发送 HTTP 请求

http 执行器把事件通知发送到外部系统。比如发 Slack 通知、写日志到收集系统、触发监控告警。

```YAML
action:
  type: http
  url: "https://hooks.slack.com/services/xxx"
  method: POST
  body: '{"text": "MewCode: Agent 修改了 $FILE_PATH"}'

```

实现上就是标准的 HTTP 请求，没什么特别的。

### agent：启动子 Agent

这是最强大的执行器。它会**启动另一个 Agent 来处理事件**。前三种执行器都做确定的动作，agent 执行器则把事件交给一个新的 Agent 自主决策。

```YAML
action:
  type: agent
  prompt: "请检查刚才写入的文件 $FILE_PATH 是否有安全漏洞。"

```

想想这意味着什么：每次 Agent 写完文件，自动启动另一个 Agent 来做安全审查。用 AI 来监督 AI，自动化程度拉满。

![agent 执行器让 AI 监督 AI 的示意图](https://feishu.cn/file/ANZbbW1SrocYnCxhQS9cGZdsn70)

不过 agent 执行器要真正跑起来，得依赖下一章（第 13 章 SubAgent）讲的子 Agent 运行时。本章我们把执行器的接口和调用骨架搭好就行：Hook 配置能加载、能校验通过，至于子 Agent 怎么真的被启动、上下文怎么传递，留到 第 13 章再展开。各语言版本在这里的过渡策略略有不同，Go 版本通过 `AgentRunner` 回调由上层注入（保持 Hook 模块零外部依赖），Python 版本则是更彻底的占位 stub，效果上都是「本章先接口，下一章再对接真实运行时」。

---

## 执行控制：once、async 和错误兜底

除了事件、条件、动作这三要素，Hook 的执行还有几个重要的控制机制。

### once：只执行一次

有些 Hook 执行一次就够了。比如 `session_start` 时注入项目上下文，第一次注入了，后面的会话不需要再注入，因为上下文已经在对话历史里了。

```YAML
hooks:
  - event: session_start
    action:
      type: prompt
      message: "项目技术栈：Python 3.12 + FastAPI + Claude API"
    once: true

```

`once: true` 表示这个 Hook 第一次触发后就标记为「已执行」，后续不再触发。实现上就是一个布尔值：`shouldRun()` 检查 `once && executed` 是否同时为真，是的话就跳过；`markExecuted()` 在执行后把 `executed` 设为 true。重启 MewCode 会重置这个标记，不做持久化。这是有意的：`once` 的语义本来就是「本次会话只触发一次」，重启 MewCode 等同于开了个全新会话，配合 `session_start` 这类事件才能正常工作。要做跨进程的「真正只触发一次」，应该用版本检测之类的机制，不是 `once` 该管的事。

![once true 只在第一次触发的时间线](https://feishu.cn/file/OI1lbvsYkotzyrxlR8Ac4vtZnab)

### async：异步执行

有些 Hook 的动作耗时较长，不应该阻塞 Agent 的执行流程。比如发 Slack 通知，网络请求可能要几百毫秒甚至几秒。通知发没发成功不影响 Agent 继续工作，没必要让 Agent 等着。

```YAML
hooks:
  - event: post_tool_use
    if: tool == "WriteFile"
    action:
      type: http
      url: "https://hooks.slack.com/services/xxx"
      body: '{"text": "文件已修改: $FILE_PATH"}'
    async: true

```

`async: true` 表示 Hook 在后台执行，不等待完成。但有一个重要的限制：**pre_tool_use 事件的 Hook 不能设为 async**。原因很明显，`pre_tool_use` 需要同步返回「允许还是拒绝」的决定，如果异步执行了，工具调用都已经开始了，拒绝还有什么意义？配置验证的时候要检查这个约束。

![同步与异步 Hook 执行的时间线对比](https://feishu.cn/file/XqJtbwZ5Po6WpGxLTi7cR8cWn7c)

在引擎里，两种模式的差别就在执行那一步。同步模式直接调用 `executeAction` 等结果，异步模式把 `executeAction` 丢到后台协程里跑，主流程不等它。

### 错误兜底

还有一个重要的设计原则：**Hook 执行出错只记日志，不中断 Agent 主流程**。

为什么？Hook 是辅助机制。格式化失败了，Agent 写的代码还是在的。Slack 通知没发出去，Agent 的工作成果不受影响。如果一个辅助机制的故障能反过来把核心流程搞崩，那就是尾巴摇狗了。所以引擎在捕获到 Hook 执行错误时，只写一行日志，然后继续往下跑。

![Hook 出错但主流程不中断的示意图](https://feishu.cn/file/UBXxbctC2oM6FHxF6uUcO4oMnJd)

---

## 上下文变量：让 Hook 知道发生了什么

Hook 的 command 和 body 里那些 `$FILE_PATH`、`$TOOL_NAME` 是怎么工作的？

每当一个事件触发，Hook 引擎会创建一个 HookContext，里面包含了这个事件的所有上下文信息。执行动作之前，引擎会把命令模板里的变量替换成上下文中的实际值。

HookContext 包含这些字段：`eventName` 事件名、`toolName` 工具名、`toolArgs` 工具参数字典、`filePath` 文件路径、`message` 消息内容、`error` 错误信息。

变量替换的规则很简单：`$EVENT` 替换成事件名，`$TOOL_NAME` 替换成工具名，`$FILE_PATH` 替换成文件路径，`$MESSAGE` 替换成消息内容，`$ERROR` 替换成错误信息。`$TOOL_ARGS.xxx` 比较特殊，它替换成工具参数字典里 `xxx` 字段的字符串表示。

![Hook 上下文变量替换示意图](https://feishu.cn/file/TqcVbdmn9ogY0Sx7pQNcd0Ihnih)

```Plain Text
function HookContext.expand(template):
    result = template
    result = replace(result, "$EVENT", eventName)
    result = replace(result, "$TOOL_NAME", toolName)
    result = replace(result, "$FILE_PATH", filePath)
    result = replace(result, "$MESSAGE", message)
    result = replace(result, "$ERROR", error)
    for key, value in toolArgs:
        result = replace(result, "$TOOL_ARGS." + key, toString(value))
    return result

```

这段逻辑很直白，逐个替换已知变量，再遍历工具参数字典替换 `$TOOL_ARGS.xxx`。未定义的变量会被替换成空字符串，不会报错。这个设计让 Hook 配置的容错性更好，不会因为某个变量在特定事件中不存在就崩掉。

条件求值也需要从 HookContext 里取字段值。`getField` 方法支持三类字段：`tool` 取工具名，`event` 取事件名，`args.xxx` 取工具参数里的对应字段。未知字段同样返回空字符串。

---

## 与 Agent Loop 的集成

Hook 引擎需要嵌入到 Agent Loop 的关键节点。回顾一下 Agent 的核心循环，用伪代码标注 Hook 的插入点：

```Plain Text
function Agent.run(conversation):
    hooks.runHooks("session_start", ctx)
    loop:
        hooks.runHooks("turn_start", ctx)
        hooks.runHooks("pre_send", ctx)
        response = llm.send(messages)
        hooks.runHooks("post_receive", ctx)
        for toolCall in response.toolCalls:
            err = hooks.runPreToolHooks(toolCallCtx)
            if err is ToolRejectedError: continue
            result = executeTool(toolCall)
            hooks.runHooks("post_tool_use", toolCallCtx)
        hooks.runHooks("turn_end", ctx)

```

Agent 结构体里添加一个 `hooks` 字段就行了。注意 `runHooks` 和 `runPreToolHooks` 是两个不同的方法。`runHooks` 用于所有非拦截事件，执行完就完了，不影响主流程。`runPreToolHooks` 用于 `pre_tool_use`，它返回一个错误，如果是 ToolRejectedError 就跳过这次工具调用，并且把拒绝原因作为工具结果返回给 LLM。

![Agent Loop 中 Hook 插入点总览](https://feishu.cn/file/SD6bbVdU2o4dxKxdxuAcmiGKnZb)

---

## 实战配置示例

理论讲完了，来看几个实际的配置场景，感受一下 Hook 系统能解决什么问题。

### 写文件后自动格式化

每次 Agent 写了源代码文件，自动用格式化工具格式化。Agent 生成的代码格式不一定完美，跑一下格式化保证一致性。

```YAML
hooks:
  - id: auto-format
    event: post_tool_use
    if: 'tool == "WriteFile" && args.path ~= "*.py"'
    action:
      type: command
      command: "black $FILE_PATH"

```

### 禁止修改 vendor 目录

`vendor/` 目录应该由包管理工具管理，Agent 不应该直接修改里面的文件。用 Hook 做一个硬性拦截。

```YAML
hooks:
  - id: block-vendor
    event: pre_tool_use
    if: 'tool == "WriteFile" && args.path ~= "vendor/*"'
    action:
      type: command
      command: "echo 'vendor 目录由包管理工具管理，请勿手动修改'"
    reject: true

```

### 新会话时加载项目上下文

每次开始新对话，自动告诉 Agent 项目的基本信息。这样你不用每次都手动提醒它。

```YAML
hooks:
  - id: project-context
    event: session_start
    action:
      type: prompt
      message: |
        项目信息：
        - 技术栈：Python 3.12 + FastAPI + Claude API
        - 代码规范：参见 .flake8
        - 架构文档：参见 ARCHITECTURE.md
    once: true

```

### 拦截高危删除命令

检测到 `rm -rf /` 之类的高危命令时自动拦截，不让 Agent 执行。这是安全兜底，即使 Agent 被 LLM 的幻觉误导，也不会真的执行危险操作。

```YAML
hooks:
  - id: block-dangerous-rm
    event: pre_tool_use
    if: 'tool == "Bash" && args.command =~ /rm\s+-rf\s+\//'
    action:
      type: command
      command: "echo '警告：检测到高危删除命令，已拦截'"
    reject: true

```

### 文件修改后发 Slack 通知

Agent 每次写文件后，自动发一条 Slack 通知。适合团队协作场景，让其他人知道 Agent 改了什么。

```YAML
hooks:
  - id: slack-notify
    event: post_tool_use
    if: 'tool == "WriteFile"'
    action:
      type: http
      url: "https://hooks.slack.com/services/xxx"
      method: POST
      body: '{"text": "MewCode Agent 修改了 $FILE_PATH"}'
    async: true

```

注意这个 Hook 用了 `async: true`，因为 Slack 通知不需要阻塞 Agent 的执行流程。发成功了很好，发失败了也不影响 Agent 继续工作。

---

## 配置加载与校验

Hook 从 YAML 文件加载，加载时引擎需要做两件事：解析条件表达式字符串为 ConditionGroup 结构，校验配置的合法性。

![Hook 配置加载与校验流程图](https://feishu.cn/file/OP7ybYXTvoa8qMxVZzIc7ddDnrh)

校验规则很明确：事件名必须在合法事件列表中，action 类型必须是 command/prompt/http/agent 四者之一，`reject` 只能用在 `pre_tool_use` 上，`async` 不能用在 `pre_tool_use` 上，每种 action 类型要检查必填字段。比如 command 类型必须有 `command` 字段，http 类型必须有 `url` 字段。

非法配置应该给出明确的错误信息并定位到具体是哪个 Hook，这样用户能快速找到写错的地方。这些约束前面各节都讲过了，加载时集中校验一遍，确保 Hook 在运行前就是合法的。

---

## 本章小结

这一章我们给 MewCode 装上了 Hook 系统，让它能够在生命周期的关键时刻自动执行动作。

核心设计很简洁：事件 + 条件 + 动作。四种执行器覆盖了命令执行、提示词注入、HTTP 通知、子 Agent 任务。`pre_tool_use` 的拦截能力让安全策略可以做到基于参数内容的细粒度控制。

Hook 系统的设计哲学是**配置优于编码**。把自动化逻辑从代码里抽出来变成用户可声明的 YAML 规则，每次加新规则不需要改代码、不需要重新编译。
