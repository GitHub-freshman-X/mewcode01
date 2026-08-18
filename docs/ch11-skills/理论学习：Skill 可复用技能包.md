# 理论学习：Skill 可复用技能包

> **架构定位**：本章继续扩展交互层，实现 Skill 技能包。Skill 把可复用的专业知识和操作流程封装成按需加载的能力模块。

---

## 三个越来越明显的痛点

![反复向 Agent 解释同样的上下文](https://feishu.cn/file/B4XabkLDsoqVZhxcaDFcLvZcnxb)

用 MewCode 久了，你会发现自己在反复向 Agent 解释同样的事情，让它帮忙做代码审查，你得说一遍团队的审查标准、要重点关注哪些维度、报告格式是什么样的，让它帮忙搭一个前端页面，你得重新描述一遍项目的组件结构、状态管理方案、样式规范，让它帮忙写测试，你得再交代一次用的是 Vitest 还是 Jest、偏好集成测试还是单元测试、mock 策略是什么。

这些上下文每次都一样，但每个新会话都得重新说，因为 Agent 不会记得上一次你怎么交代的，这是第一个痛点：**重复解释**，同样的偏好和流程反复输入，浪费时间还容易遗漏。



第二个痛点更隐蔽，回头看看 MEWCODE.md，随着使用时间变长，你会往里面塞越来越多的东西：代码审查的标准、commit message 的格式、部署流程、API 调用的注意事项，这些内容从「事实描述」慢慢长成了「操作流程」，MEWCODE.md 变成了一个大杂烩，问题是它每一轮对话都会完整加载，你只是想问一个简单问题，那段 200 行的部署流程也跟着进了上下文，白白消耗 token。



第三个痛点是**知识流失**，团队踩过的坑往往散落在各人脑子里：某个内部 API 有个不写在文档里的限速规则，deploy 脚本在周五下午跑会触发告警，代码审查要特别留意某个历史遗留模块的并发问题，这些经验 Agent 根本不知道，每次让它帮忙你都得手动把相关上下文贴进去。



Skill 系统一并解决这三个问题：把重复的偏好和流程打包成独立的 Markdown 文件，只在需要时加载不浪费 token，团队的专业知识变成可版本控制、可分发的资产。

---

## Skill 是什么：写给 Agent 的 SOP

![写给 Agent 的 SOP](https://feishu.cn/file/Jjp2bjhoFoSlv0xjDMTcZnYSnKc)

你在公司里大概见过 SOP（Standard Operating Procedure），就是「标准操作流程」。新人入职，给他一份 SOP：「当你要部署时，按照 1、2、3 步骤来」。SOP 是写给人看的，人按照流程去执行。

Skill 就是写给 Agent 的 SOP，它告诉 Agent：「当用户要求做提交/审查/测试时，按照这个流程和标准来执行」，区别在于，SOP 给人看，人来执行，Skill 给 Agent 看，Agent 来执行。

但有一点很关键：人类可以「领会精神」，你写得模糊一点，有经验的人也能理解你的意思，Agent 不行，它只会照字面做，你写「注意安全」，人知道要检查 SQL 注入和 XSS，你写「注意安全」给 Agent，它可能只会在回复里加一句「已注意安全问题」就完事了，所以 Skill 里的指令需要比人类 SOP 更精确、更具体。



Anthropic 把 Skill 定位成一个开放标准（Agent Skills），核心理念是**知识固化和跨平台复用**：团队踩过的坑、定下的规范不再靠口头传授，一份 SKILL.md 可以在 Claude.ai、Claude Code、API 甚至其他 AI 工具里共用。MewCode 的 Skill 设计沿用同样的思路，把可复用的操作流程和专业知识从对话里抽出来，变成可编辑、可分发的 Markdown 资产。

---

## Slash Command 和 Skill 的分工

![Slash Command 与 Skill 的分工](https://feishu.cn/file/PDo6bM7H0o3MPyx2faxcII4hnFh)

上一章实现的 Slash Command 分成两大类：`local` 和 `local-ui` 是纯本地操作，`/help` 查帮助、`/clear` 清屏、`/plan` 切模式，不走 LLM 不消耗 token，确定性百分之百，按下去就执行；`prompt` 类型则会把一段预设的 prompt 转发给 Agent 处理，`/review` 就是它的代表。

Skill 和 `prompt` 类 Slash Command 看起来很像，都是「把一段写好的 prompt 交给 Agent 执行」。区别在于 Skill 多了三个能力。

第一是 **Agent 能主动发现**。Slash Command 只能用户手动输入 `/xxx` 触发，Skill 可以被 Agent 根据用户意图自动匹配和加载，用户不需要记住有哪些 Skill。

第二是**可以携带整套资源**。`prompt` 类 Slash Command 就是一段孤零零的字符串，Skill 是一个目录，可以装参考文档、示例文件、辅助脚本，Agent 按需读取。

第三是 **inline / fork 两种执行模式**。Slash Command 一律在当前对话里执行，Skill 可以选择在独立上下文中执行，适合过程噪声很大且不依赖当前对话细节的场景。MewCode 的内置 `/review` 默认采用 inline，因为先前对话中的需求、偏好和已确认约束本身就是审查标准的一部分。

简单判断：不需要 AI 参与的确定性操作用 Slash Command，需要 AI 判断且流程可复用的任务用 Skill，实际上在 Claude Code 的最新版本中，自定义命令已经和 Skill 合并成了同一套机制。

---

## Markdown 就是定义文件

![Skill 文件结构](https://feishu.cn/file/NpMybG1Xgo632YxbGlnc5DUdnYd)

Skill 的定义格式是 Markdown，因为它同时满足三个需求：人类可以直接阅读，系统可以通过 YAML frontmatter 解析结构化数据，LLM 可以直接理解 prompt body 中的指令。

一个完整的 Skill 文件长这样：

```Markdown
---
name: commit
description: 分析 git diff 并生成规范的 commit
mode: inline
---

# 任务

你需要帮用户创建一个 git commit。

## 步骤

1. 运行 `git status` 查看当前变更状态
2. 运行 `git diff` 和 `git diff --staged` 查看具体变更内容
3. 分析变更，确定 commit 类型和范围：
   - feat: 新功能
   - fix: 修复 bug
   - docs: 文档变更
   - refactor: 重构
   - test: 测试
   - chore: 构建/工具变更
4. 生成 commit message，格式：`type(scope): description`
5. 用 `git add` 添加相关文件（不要添加 .env、credentials 等敏感文件）
6. 执行 `git commit -m "生成的 message"`
7. 如果用户提供了额外说明，纳入 commit message

## 注意事项

- 不要用 `git add -A`，逐个文件添加
- commit message 用英文
- description 不超过 72 个字符
- 如果变更太多，建议用户拆分成多个 commit

```

上半部分是 YAML frontmatter，即两个 `---` 之间的内容，定义了 Skill 的元信息。下半部分是 prompt body，就是实际发送给 LLM 的指令。

这意味着什么？你要调整 commit message 的格式规范，打开这个 `.md` 文件改几行文字就行了，不用碰源代码，不用重新编译。你想加一个新 Skill，创建一个 `.md` 文件就行了。

---

## Frontmatter 里的每个字段

![Skill Frontmatter 身份证](https://feishu.cn/file/T3GQb8qk9ozPrax7lAccCGNlnid)

YAML frontmatter 是 Skill 的「身份证」，让我们逐个看看每个字段是干什么的。

`name` 是必填的，它是 Skill 的唯一标识符，同时也是调用它的命令名。`name: commit` 意味着用户可以通过 `/commit` 来调用这个 Skill。命名规范是小写字母、数字和连字符。注意不能和内置 Slash Command 冲突，你不能定义一个 `name: help` 的 Skill，因为 `/help` 已经被内置命令占了。

`description` 也是必填的，一句话描述 Skill 的功能。它会出现在 `/help` 列表里，也用于自动匹配。

`model` 是可选的，指定 Skill 使用的模型。有些任务简单（比如生成 commit message），可以用便宜一点的模型；有些任务复杂（比如代码审查），需要最强的模型。不指定就用默认模型。

`mode` 是可选的，控制 Skill 的执行模式：`inline`（默认）或 `fork`。这个区别很重要，后面会专门讲。

`context` 也是可选的，只在 fork 模式下生效，决定把多少主对话的上下文带进 fork 会话。可以是 `full`（完整对话的摘要，默认）、`recent`（最近 5 条消息）、`none`（完全隔离）。inline 模式本身就共享对话历史，这个字段会被忽略。

---

## 三个地方找 Skill，谁优先级高

![三层 Skill 搜索路径](https://feishu.cn/file/TbmLbslF7onsKbxaSZecGMmKncd)

Skill 文件可以放在三个位置，按优先级从高到低：

```Plain Text
1. 项目级：{projectDir}/.mewcode/skills/
2. 用户级：~/.mewcode/skills/
3. 内置级：程序自带的 Skill（编译进二进制）

```

这个设计你应该不陌生，跟 npm 的包搜索路径很像：先看项目本地，再看全局，最后看内置。也跟 Git 的 `.gitconfig` 一样：项目级配置覆盖全局配置。

同名 Skill 高优先级覆盖低优先级，这意味着几件很实用的事情。

项目可以定义项目特有的 Skill，比如你的项目有自己的 deploy 流程，写一个 `deploy.md` 放在 `.mewcode/skills/` 下面就行。

用户可以定义个人通用的 Skill。你喜欢 commit message 用中文？写一个自己版本的 `commit.md` 放在 `~/.mewcode/skills/` 下面，它会覆盖内置的英文版。

项目级 Skill 可以提交到版本控制，整个团队共享。团队的最佳实践从口头传授变成了一个个可执行的 Skill 文件。新人入职，clone 仓库，Skill 就自动加载了。

MewCode 自带几个内置 Skill，开箱即用，不需要用户手动安装。

---

## inline vs fork：要不要共享上下文

![inline 与 fork 模式](https://feishu.cn/file/OZJybKsPdotBM7xg4TkcOZfDnbf)

Skill 有两种执行模式，这是理解 Skill 系统最重要的概念之一。

先说 `inline` 模式，它是默认模式。Skill 的 prompt 被注入到当前对话中，和正常的用户消息一样走 Agent Loop。Skill 可以看到之前的对话上下文，执行结果也会留在对话历史中。

```Plain Text
[用户消息 1]
[Agent 回复 1]
[用户消息 2]
[Agent 回复 2]
[用户: /commit]           <- Skill 触发
[系统注入 Skill prompt]   <- inline 注入
[Agent 执行 Skill]        <- 能看到消息 1-2 的上下文
[Skill 完成]
[用户消息 3]              <- 能看到 Skill 的执行结果

```

为什么 `/commit` 适合 inline？因为 Agent 可能在前面的对话中已经帮你做了一些代码修改，Skill 需要知道这些修改的上下文才能正确判断哪些文件该 commit。

再说 `fork` 模式。Skill 在一个独立的上下文中执行，不影响也不受当前对话影响。就像开了一个新的 Agent 会话，执行完后只把结果摘要返回到主对话。

```Plain Text
[主对话]                    [fork 会话]
用户消息 1                  
Agent 回复 1               
用户: /review               
  ────────────>           Skill prompt（独立上下文）
  （主对话暂停）              Agent 执行审查
                             读文件、分析代码...
                             生成审查报告
  <────────────           返回审查报告
Agent 显示审查报告
用户消息 3

```

fork 的价值是隔离执行过程或减少上下文干扰，但不代表所有审查都应忽略聊天。MewCode 的内置 `/review` 采用 inline：用户可能已在对话中说明业务约束、团队封装、允许的依赖和有意保留的取舍，这些信息应成为审查依据。若需要独立的第二意见，可创建 `mode: fork` 的自定义审查 Skill，并按任务选择 `full`、`recent` 或 `none`。

在 frontmatter 中指定执行模式很简单：

```YAML
mode: inline  # 默认
mode: fork    # 独立上下文

```

fork 模式可以通过 `skill.context` 控制带多少主对话的上下文进去。需要完整业务背景的隔离任务可用 `full`；只需近期上下文可用 `recent`；真正不依赖当前对话的独立扫描或第二意见才适合 `none`。内置 `/review` 不使用 fork，Skill 作者应按任务是否依赖已讨论约束来选择。

---

## 自动注册为命令：Skill 和 Slash Command 对用户透明

![Skill 作为 Slash Command 出现在 /help](https://feishu.cn/file/BjbebZc6toYiN0x4cbYcKKUlnye)

Skill 加载后会自动注册为 Slash Command。比如 MewCode 自带的 `skill-creator`，安装好就能通过 `/skill-creator` 调用，出现在 `/help` 列表里，支持 Tab 补全，和内置命令的体验完全一致。

---

## 意图识别：让 Agent 自己选 Skill

`/commit` 这种显式调用很直观，但有个限制：用户得记住有哪些 Skill、对应什么命令。如果用户说「帮我提交一下」或者「我想做个后端面试准备」，Agent 能不能自动识别意图，主动加载对应的 Skill？

这才是 Skill 更常见的使用方式。

### 两阶段加载

![两阶段 Skill 加载](https://feishu.cn/file/NzgbbTAshoSbwfxnynGcpqPBnjc)

关键思路是把 Skill 的加载拆成两个阶段。

**第一阶段：轻量注册。** MewCode 启动时只加载每个 Skill 的 frontmatter（name、description），不加载完整的 prompt body。这些轻量信息拼成一份清单，随首条消息注入对话，告诉 Agent 有哪些 Skill 可用：

```Plain Text
你可以使用以下 Skill：

- commit：分析 git diff 并生成规范的 commit。
- review：审查代码变更。
- backend-interview：模拟后端技术面试，基于候选人简历生成针对性问题。

如果用户的请求匹配某个 Skill，请调用 LoadSkill 工具加载它。

```

**第二阶段：按需加载。** 当 Agent 判断用户意图匹配某个 Skill 时，调用 `LoadSkill` 工具，把 SKILL.md 的完整 SOP 加载到对话中。模型在下一轮迭代时就能看到完整的 SOP 指令，跟着执行。LoadSkill 是只读操作，不会触发权限确认，和 ReadFile、Grep 走同一条通道。

这就是渐进式披露的完整实现。Agent 平时只看到 Skill 的名称和描述，选择压力很低。一旦确定要用某个 Skill，才加载完整 SOP，注意力集中在当前任务上。

### 目录型 Skill

![目录型 Skill 结构](https://feishu.cn/file/DdUHb4ffKo392lxNslscLMTPnXb)

前面介绍的单文件 Skill 只有一个 Markdown 文件，适合简单的流程指令。但有些 Skill 需要携带更多资源：模板文件让 Agent 填充、示例输出展示期望格式、辅助脚本让 Agent 通过 Bash 执行、长文档作为参考资料。一个文件装不下这些东西。所以 Skill 演化成了一个目录：

```
skills/
└── backend-interview/
    ├── SKILL.md               # 入口：frontmatter + SOP 流程
    ├── examples/              # 示例输出
    │   └── sample-questions.md
    └── scripts/               # 辅助脚本
        └── validate.sh
```

`SKILL.md` 是入口，角色和单文件 Skill 一致。目录里的其他文件是附属资源，Agent 通过已有的内置工具（ReadFile、Bash）来访问它们。SKILL.md 里需要引用这些文件，告诉 Agent 每个文件是什么、什么时候该去读。

和单文件 Skill 的区别在于：单文件 Skill 只有 SOP 指令本身，目录型 Skill 可以携带模板、示例、脚本和参考文档。这些附属文件不会在加载 Skill 时全部塞进上下文，Agent 按需读取，只有真正需要时才消耗 token。

目录型 Skill 还带来另一个常被低估的好处：**整套能力可打包移植**。整个 `backend-interview/` 目录是一个自包含单元，SOP、示例、脚本、参考文档全在里面。

同事写好的 Skill 可以推到 GitHub 仓库或者打成 zip 分发，新人 clone 或解压下来扔进 `~/.mewcode/skills/`，下次启动 MewCode 就能直接用 `/backend-interview` 调用。团队内部沉淀的最佳实践不再散落在各人的 prompt 收藏夹里，而是变成可被代码评审、可被打版本号、可在 CI 里自动校验的一份配置资产。

![可分发的 Skill 能力包](https://feishu.cn/file/EZ9cbNHh8opye8xnUQSc1tkDnwf)

### 完整流程

把显式命令和意图识别两种方式串起来看：

```Plain
1. 启动：扫描 Skill 目录，加载所有 Skill 的 frontmatter（轻量）
   ↓
2. 注入 messages：告诉 Agent 有哪些 Skill 可用（只有 name + description）
   ↓
3. 注册命令：同时注册为 Slash Command，支持 /commit 显式调用
   ↓
4. 用户输入：「帮我做个后端系统设计的面试准备」
   ↓
5. 意图识别：Agent 判断匹配 backend-interview Skill
   ↓
6. Agent 调用 LoadSkill("backend-interview")
   ↓
7. 完整 SOP 加载到对话中
   ↓
8. Agent 按 SKILL.md 的 Workflow 执行，按需使用内置工具和 MCP 工具

```

两种触发方式并存：用户可以直接打 `/commit` 显式调用，也可以用自然语言描述需求让 Agent 自动匹配。无论哪种方式触发，后面的执行流程是一样的。

---

## Skill 在工具生态中的位置

![Skill 在工具生态中的位置](https://feishu.cn/file/DjCzbtk3jooklrx0Q8pc9E6Jngh)

到这一章为止，MewCode 围绕工具建立了多层体系。

最底层是 Function Calling，就是前面实现的 Tool 接口，工具调用的原子单位，每次调用做一件具体的事情。MCP 在此之上提供了开放的工具接入协议，让第三方工具标准化接入。Skill 则在更高层面组织这些工具，把一组相关的工具调用编排成任务级工作流，配上 SOP 指令和上下文控制。

三者是互补关系。Function Calling 负责调用，MCP 负责接入，Skill 负责编排。

一个 Skill 可以调用 MCP 工具，比如 commit Skill 调用 GitHub MCP Server 来创建 PR；反过来，MCP Server 的能力也可以通过 Skill 封装成面向用户的任务流程。当工具太多导致模型调不准时，Skill 通过两阶段加载和 SOP 指令把模型的注意力收窄到当前任务；当需要接入外部能力时，MCP 提供标准化通道。它们以 Function Calling 为基座，构成了 Agent 工具协作的生态。

---

## 本章小结

Skill 系统解决了三个问题：重复的偏好和流程打包成独立的 Markdown 文件，不用每次重新解释；MEWCODE.md 里的操作流程拆出来按需加载，不浪费 token；团队的专业知识变成可版本控制、可分发的资产，不再散落在各人脑子里。两阶段加载让 Agent 平时只看到 Skill 的名称和描述，按需加载完整内容，inline 和 fork 两种执行模式覆盖了不同的上下文需求。

MewCode 到这里已经具备了从对话到工具到记忆到技能的核心能力框架，下面几章还会加上 Hook、SubAgent、Worktree 这些让它真正在团队里跑起来的能力。
