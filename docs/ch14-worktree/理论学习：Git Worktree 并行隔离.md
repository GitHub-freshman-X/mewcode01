# 理论学习：Git Worktree 并行隔离

> **架构定位**：本章实现 安全层的文件系统隔离。通过 Git Worktree 让多个 Agent 在独立目录中工作，补上安全层在文件系统维度的最后一块拼图。

---

## 并行工作时，文件系统怎么隔离？

上一章我们实现了 SubAgent。主 Agent 可以把任务分派给子 Agent，子 Agent 在隔离的上下文中执行。消息隔离了，权限隔离了，文件缓存隔离了。但章末其实留了一个尾巴——还有一个维度没有隔离：文件系统。这一章就是来补上这个缺口的。

前台子 Agent 不需要担心这个问题。主 Agent 会等它完成后才继续工作，同一时刻只有一个 Agent 在操作文件，不会冲突。但**后台子 Agent** 和 **Agent Team 的队员**（下一章会讲到）就不一样了。它们和主 Agent 或其他队员并行运行，共享同一个文件系统。

假设主 Agent 正在修改 `server.py`，同时一个后台子 Agent 也在修改 `server.py`。会发生什么？

灾难。

两个 Agent 可能读到对方写了一半的文件。或者互相覆盖对方的修改。Agent A 写了前半段，Agent B 写了后半段，最后文件变成了一个拼接的怪物。

![并发 Agent 文件编辑冲突](https://feishu.cn/file/PlA6bMr8cog0b2xcnW5ct8G7njd)

这不是 Agent 特有的问题。它本质上就是并行开发的文件冲突问题，和两个程序员同时改同一个文件完全一样。程序员们用 Git 分支来解决这个问题：每人一个分支，改完了再合并。

但分支真的够用吗？

---

## 为什么分支不够

你可能会想：给每个子 Agent 创建一个 Git 分支不就行了？子 Agent 在自己的分支上工作，完事了合并回来。

听起来合理，但仔细想想有个致命问题。

Git 分支解决的是版本管理问题，管不了文件隔离。当你切换到 feature-a 分支的时候，工作目录里的文件变成了 feature-a 分支的内容。你再切到 feature-b，工作目录又变了。整个过程中只有一个工作目录。

```Plain Text
分支方案的问题：

时间点1：Agent 在 main 分支，server.py 内容为 A
时间点2：切到 feature 分支，server.py 内容变成 B
时间点3：此时 main 分支的工作目录已经变了！

主 Agent 和子 Agent 不能同时在不同分支上工作，
因为只有一个工作目录。

```

换句话说，分支提供的是时间维度的隔离，记录不同时间点的代码快照。但我们需要的是空间维度的隔离，让同一时间存在多个独立工作区。

![分支隔离 vs Worktree 空间隔离](https://feishu.cn/file/Vnhjbwyn3o2pbaxwx9wcpgTXnGf)

还有一个实际的问题。切分支的时候，会改变工作目录里分支间有差异的文件的修改时间戳。这些文件 mtime 被刷新后，依赖追踪型的构建工具会把它们及其下游全部判定为「需要重新构建」——本来只需要重编一个文件的增量构建，可能扩散成大半个项目的重编。

我们需要一个方案，能在同一时间拥有多个独立的工作目录，每个目录对应不同的分支，互不干扰。

这个方案就是 Git Worktree。

---

## Git Worktree：共享仓库，隔离文件

Git Worktree 是 Git 2.5 引入的功能。一句话概括它做的事情：允许你在同一个仓库中创建多个独立的工作目录。

通常一个 Git 仓库只有一个工作目录。你切分支的时候，这个工作目录里的文件会跟着变。Worktree 改变了这一点：

```Bash
# 在当前仓库旁边创建一个新的工作目录
git worktree add ../my-project-feature-a feature-a

# 现在有两个工作目录：
# ./my-project/          -> main 分支
# ./my-project-feature-a/ -> feature-a 分支

```

执行完这条命令后，你的文件系统里多了一个目录。这个目录里是 feature-a 分支的完整文件。原来的目录不受影响，还是 main 分支的文件。

两个目录完全独立。你可以同时在两个目录里改代码、编译、跑测试，互不干扰。但它们共享同一个 `.git` 仓库，所以版本历史是统一的。在一个 Worktree 里提交的 commit，在另一个 Worktree 里执行 `git log --all` 也能看到。

**共享仓库，隔离文件。** 这正是我们需要的。

对比一下：

```Plain Text
Worktree 方案：

./project/            -> main 分支，server.py 内容为 A
./project-feature/    -> feature 分支，server.py 内容为 B

两个 Agent 同时工作，互不影响。

```

完美解决了并行 Agent 的文件冲突问题。主 Agent 在主目录工作，子 Agent 在 Worktree 目录工作。子 Agent 随便怎么改文件，都不会影响主 Agent 正在操作的文件。

![共享 Git 仓库、独立工作目录](https://feishu.cn/file/TzfKbEBMcoWv4XxYYCqcp4G7nHd)

---

## MewCode 的 Worktree 管理

理解了 Git Worktree 的原理，接下来要做的就是在 MewCode 里封装一层管理逻辑。我们需要处理 Worktree 的完整生命周期：创建、进入、退出、删除。

### WorktreeManager 的设计

先看 WorktreeManager 自身需要维护哪些状态：

```Plain Text
WorktreeManager:
    repoRoot: string                        // 主仓库路径
    worktreeDir: string                     // Worktree 存放目录
    lock: Mutex                             // 并发保护
    active: Map<string, Worktree>           // name -> Worktree
    fileCache: FileCache                    // 用于进入/退出时清理
    currentSession: WorktreeSession | null  // 当前活跃的 Worktree 会话

```

WorktreeManager 持有一个 fileCache 引用。为什么要绑在 Manager 上？因为缓存清理是 Worktree 生命周期不可分割的一部分，放在 Manager 内部可以保证每次进入和退出时一定会清理，不依赖调用方记得传参。

![WorktreeManager 结构](https://feishu.cn/file/X1T9bSDgUog0a4xuiCfcp1bRnKe)

每个 Worktree 实例记录的信息也很简洁：

```Plain Text
Worktree:
    name: string
    path: string
    branch: string
    basedOn: string
    headCommit: string
    created: timestamp

```

当 Agent 进入某个 Worktree 时，还需要一组会话信息来记录进入前的状态，退出时才能恢复回去：

```Plain Text
WorktreeSession:
    originalCwd: string          // 进入前的工作目录
    worktreePath: string         // Worktree 路径
    worktreeName: string         // Slug 名称
    originalBranch: string       // 进入前所在的分支
    originalHeadCommit: string   // 进入时的 HEAD commit SHA
    sessionId: string            // 会话 ID
    hookBased: bool              // 是否由 Hook 创建

```

`currentSession` 会被持久化到 `.mewcode/worktree_session.json`。如果 MewCode 进程意外退出，下次启动时可以从配置中恢复会话状态，直接回到上次使用的 Worktree，跳过重新创建的过程。这是 `--resume` 恢复能力的基础。

![Worktree 会话持久化与恢复](https://feishu.cn/file/DIGpbKVbronWOSx81JrcqYiUn4T)

### Slug 安全验证

在创建 Worktree 之前，有一件事必须先做：验证名称的安全性。

为什么？因为 Worktree 的名称会被用作文件系统的目录名和 Git 的分支名。LLM 生成的输入不可信，如果不做验证，可能传入类似 `../../etc/passwd` 的名称。一旦你把它拼到路径里，Worktree 就可能被创建到系统目录下。这是经典的路径遍历攻击。

![Slug 校验防止路径遍历](https://feishu.cn/file/VrpCbPnW4o7ZgJxNjlKcJNuInoe)

验证规则：只允许大小写字母、数字、点号、连字符和下划线，总长度不超过 64。名称可以包含 `/` 作为嵌套分隔符（比如 Agent Team 的 `team-refactor/alice`），按 `/` 分割后每一段独立校验。

![Slug 转换为 Worktree 分支名](https://feishu.cn/file/OAZtblUvooSB7YxttIac7oeInZf)

### 创建 Worktree

创建一个 Worktree 要经过几个阶段。先看前半部分，从验证到构建路径：

```Plain Text
function createWorktree(manager, ctx, name, baseBranch):
    // 1. 验证名称
    err = validateSlug(name)
    if err: return err

    // 2. 锁内检查是否已存在
    if name in manager.active:
        return error("worktree already exists: " + name)

    // 3. 构建路径和分支名
    flatSlug = replace(name, "/", "+")
    wtPath = joinPath(manager.worktreeDir, flatSlug)
    branchName = "worktree-" + flatSlug

```

分支名加 `worktree-` 前缀，在 `git branch` 的输出里一眼就能看出哪些是 MewCode 创建的。嵌套 slug 的 `/` 替换成 `+`，比如 slug `team-refactor/alice` 对应分支名 `worktree-team-refactor+alice`。

Worktree 目录统一放在仓库内部的 `.mewcode/worktrees/` 下。`.mewcode/` 已经在 `.gitignore` 里了，不会被 Git 追踪。

接下来是一个性能优化。如果 Worktree 的目录已经存在，能不能跳过 `git worktree add` 直接复用？

```Plain Text
    // 3.5 快速恢复：纯文件系统读取，不调 git 子进程
    headSha = readWorktreeHeadSha(wtPath)
    if headSha is not null:
        return existingWorktree(wtPath, branchName, headSha)

```

这就是快速恢复。不调用 `git` 子进程，直接读文件系统：先读 Worktree 目录下的 `.git` 指针文件得到 `gitdir` 路径，再读 `HEAD` 文件。如果 HEAD 是符号引用 `ref: refs/heads/...`，继续读 `refs/` 目录还原出 commit SHA。整个过程只有几次文件读取，大约 3ms。

作为对比，`git worktree add` 在大仓库上需要数秒（检出全量文件树 + 写入工作目录）。Agent 反复进出同一个 Worktree 的场景下，这个优化省下的时间非常可观。

![快速恢复 vs git fetch 耗时对比](https://feishu.cn/file/AX0jbTQDGoutgsxsygXcNpqrnJe)

如果没有可恢复的 Worktree，就正常创建：

```Plain Text
    // 4. 执行 git worktree add
    env = {"GIT_TERMINAL_PROMPT": "0", "GIT_ASKPASS": ""}
    run("git", "worktree", "add",
        "-B", branchName, wtPath, baseBranch,
        workDir=manager.repoRoot,
        env=env, stdin="ignore")

    // 5. 创建后设置（只对新建执行，快速恢复跳过）
    performPostCreationSetup(manager.repoRoot, wtPath)

```

`GIT_TERMINAL_PROMPT=0` 必须在所有 Git 命令中设置。如果创建过程中 Git 需要输入凭证，没有这个环境变量，进程会挂起等待用户输入。同时设置 `GIT_ASKPASS=''` 和 `stdin: 'ignore'` 作为双重保险。

用 `-B` 而不是小写 `-b`，是因为 `-B` 可以重置已存在的分支。如果上次创建后没清理干净，留下了孤儿分支，`-b` 会报错，`-B` 直接覆盖。

最后记录状态并持久化：

```Plain Text
    // 6. 记录状态
    wt = new Worktree(
        name, wtPath, branchName, baseBranch,
        resolveHead(wtPath), now())

    manager.active[name] = wt
    saveWorktreeSession(manager.currentSession)
    return wt

```

![创建 Worktree 的六步流程](https://feishu.cn/file/UqVsbBNLKoj9XGxn7KYc3uHcncf)

### 创建后设置

`git worktree add` 创建了一个干净的工作目录，但这个目录缺少主仓库里已有的一些运行时依赖。直接让 Agent 进去工作，很多东西会报错或行为不一致。

都缺什么？

![创建后的四项初始化动作](https://feishu.cn/file/TXB3bWtXHo2PVSx2QeIckCq5nHC)

**A. 本地配置文件。** `settings.local.json` 存放密钥和环境变量等不入库的配置。Worktree 是一个全新的目录，不会自动拥有这些文件，需要从主仓库复制过去。

**B. Git Hooks 配置。** Worktree 共享主仓库的 `.git` 目录，但 `core.hooksPath` 配置不会自动继承到 Worktree 的工作区。如果主仓库用了 husky 或自定义 hooks 目录，Worktree 里的 `git commit` 不会触发 pre-commit 检查、lint、格式化这些流程。需要检测主仓库的 hooks 路径，优先检查 `.husky/`，回退到 `.git/hooks/`，然后显式设置到 Worktree 的 git config 中。

**C. 软链接大目录。** `node_modules`、`.venv`、`vendor` 这些依赖目录可能占几百 MB。每个 Worktree 都复制一份会迅速耗尽磁盘。软链接到主仓库的对应目录，所有 Worktree 共享同一份依赖。需要软链接的目录列表从 `settings.worktree.symlinkDirectories` 配置读取，不同项目的依赖目录结构不一样，不能写死在代码里。

这里有个 Node.js 项目要特别注意的坑：Node 默认会把 symlink 解析到真实路径，包内部用 `__dirname` 拿到的是主仓库路径，而不是 Worktree 路径。webpack、TypeScript 这些工具的模块 resolve 默认也是同样行为。多数项目无碍，但依赖 `__dirname` 做路径计算的包，或者用了 `npm link` 风格的 monorepo，可能会拿到错的路径——表现为构建产物指向了主仓库的源文件。遇到这种情况，要么开 `--preserve-symlinks` / `resolve.symlinks: false`，要么在 Worktree 里独立装一份依赖。symlink 是 best-effort 策略，不是万能的。

**D. 复制被忽略但需要的文件。** 有些文件被 `.gitignore` 忽略了，`git worktree add` 不会复制它们。但 Worktree 可能需要这些文件才能正常运行，最典型的是 `.env`。项目根目录下的 `.worktreeinclude` 文件使用 gitignore 语法定义哪些被忽略的文件需要复制。用 `git ls-files --others --ignored --exclude-standard --directory` 列出所有被忽略的文件，再用 `.worktreeinclude` 的模式过滤出需要的那些。这个步骤是 best-effort 的：模式匹配失败或文件不存在只记录警告，不中断创建流程。

### 进入 Worktree

Worktree 创建好了，怎么让 Agent 切进去工作？

最直接的想法是：把进程的工作目录 `chdir` 到 Worktree 路径，后面 Agent 的所有文件操作自然就发生在 Worktree 里了。听起来很顺，但 MewCode 实际上选了另一条路——**不动进程级 cwd，而是把 Worktree 路径记到会话状态里，让每次工具调用自己显式取 cwd**。

为什么不直接 `chdir`？

进程级 cwd 是全局可变状态。Bash 工具偶尔 `cd` 一下，子 goroutine 看到的 cwd 又是另一个，并发组件（后台子 Agent、Agent Team 的队员）之间还会撞上时序问题——谁的 `chdir` 先谁的后，结果可能完全不同。一旦把 Worktree 切换绑在进程 cwd 上，cwd 就变成了所有并发组件的同步点，复杂度直线上升。

显式 cwd 把这件事倒过来：

```Plain Text
function enterWorktree(ctx, name) -> error:
    wt = active[name]
    if wt is null: return error("not found: " + name)

    // 记录会话状态：原 cwd、Worktree 路径、原分支、原 HEAD
    currentSession = new WorktreeSession(
        originalCwd: getCurrentDirectory(),
        worktreePath: wt.path,
        worktreeBranch: wt.branch,
        originalBranch: ..., originalHeadCommit: ...,
        sessionID: ...)

    // 持久化到 .mewcode/worktree_session.json
    saveWorktreeSession(currentSession)
    return nil

```

注意里面**没有 `chdir`，也没有清缓存**。

后续 Agent 调用 Bash、Read、Write 这些工具时，工具自己从 `currentSession.WorktreePath` 取出路径，作为本次调用的 cwd 传给子进程或文件操作。进程 cwd 还在 `originalCwd`，每次工具调用是**显式声明"在 Worktree 里跑"**的。

这种「记一笔位置 + 工具显式取」的模式比改全局状态更可控：

- 主 Agent 同时在调多个工具？没事，每次调用都从 session 单独取当前 worktree。
- 子 Agent 又开了新 Worktree？它有自己的 session，互不干扰。
- 工具调用栈深？每一层都从 session 自己取，不依赖中间某次 `chdir` 没被 reset。

至于文件缓存——既然 cache key 用绝对路径（`/repo/server.py` 和 `/repo/.mewcode/worktrees/feature/server.py` 是不同的 key），Worktree 里的 `server.py` 和主目录里的 `server.py` 自然撞不到一起，**根本不需要清缓存**。这是 explicit cwd 模式自带的一个好处。

### 退出 Worktree

Agent 在 Worktree 里完成工作后，需要切回主目录。退出时有一个选择：保留这个 Worktree 还是删除它？

如果选择删除，还要考虑一个问题：Worktree 里有没有未保存的修改？

这就是退出时的**变更保护**。如果 Worktree 里有未提交的文件修改或新增的 commit，直接删除意味着丢失工作成果。LLM 可能不理解这些修改的价值，一个错误的 remove 调用就能把子 Agent 几个小时的工作彻底抹掉。所以 `discardChanges` 参数要求调用方显式确认丢弃。

![退出 Worktree 的变更保护](https://feishu.cn/file/O8iYbPPzdod1M7xplJXc2SXXnnh)

```Plain Text
function exitWorktree(manager, ctx, name, action, discard):
    wt = manager.active[name]

    // 1. 变更保护
    if action == "remove" and not discard:
        changes = countWorktreeChanges(wt.path, wt.headCommit)
        if changes.uncommitted > 0 or changes.newCommits > 0:
            return error("worktree has changes, "
                + "set discardChanges=true to force")

```

检查通过后，切回原来的工作目录并恢复状态：

```Plain Text
    // 2. 切回原 cwd 兜底
    //    Worktree session 期间工具调用走 explicit cwd，进程 cwd 理论上
    //    一直在 originalCwd。这里再 chdir 一次是兜底——防止 session 期间
    //    某个 Bash 调用偶尔改过进程 cwd 留下残留。
    changeDirectory(session.originalCwd)

    // 3. 清 session + 持久化清除
    currentSession = null
    saveWorktreeSession(repoRoot, null)

```

**为什么 session 要清掉并持久化为 null？** 进程下次启动 `--resume` 的时候会读 `.mewcode/worktree_session.json`：如果还留着这条记录，恢复逻辑会以为还有未结束的 Worktree 会话，去尝试切回一个已经退出（甚至已被 `remove` 删掉）的目录。退出时及时把这条记录清掉，是 `--resume` 能正确工作的前提。

持久化 session 为 null 也很重要。如果不清，下次启动 MewCode 时 `--resume` 会尝试恢复一个已经不存在的 Worktree。

如果用户选择删除 Worktree：

```Plain Text
    // 5. 可选：删除 Worktree
    if action == "remove":
        run("git", "worktree", "remove", "--force", wt.path,
            workDir=manager.repoRoot)

        sleep(100)     // 等待 git lockfile 释放
        run("git", "branch", "-D", branchName,
            workDir=manager.repoRoot)

        delete manager.active[name]

```

`git worktree remove` 和 `git branch -D` 之间的 `sleep(100)` 看起来不优雅，但有实际作用。两条 git 命令间隔太短，可能因为 git 的 lockfile 还没释放而失败。100ms 的等待是经验值，生产环境更稳妥的做法是带重试的指数退避。

### 自动清理

每次都要手动决定保留还是删除，对子 Agent 场景来说太繁琐了。能不能自动判断？

思路很简单：如果 Worktree 里没有未提交的修改、也没有新增的 commit，说明子 Agent 只是读了一些文件做了分析，没留下什么有价值的东西，直接清掉。如果子 Agent 写了代码或做了 commit，Worktree 留着让主 Agent review。

```Plain Text
function autoCleanup(manager, name, headCommit):
    wt = manager.active[name]
    if hasWorktreeChanges(wt.path, headCommit):
        return {kept: true, path: wt.path, branch: wt.branch}
    else:
        removeWorktree(manager, name)
        return {kept: false}

```

![自动清理：保留还是删除](https://feishu.cn/file/MN7lb9DTsoOUrsxW4OscEafUnZd)

用户通过 `/worktree create` 手动创建的 Worktree 不走自动清理，保留手动控制。

### 过期 Worktree 的后台清理

自动清理解决的是子 Agent 正常退出后的清理。但如果子 Agent 异常退出呢？进程崩溃、被用户强制终止，Worktree 会留在磁盘上成为孤儿。时间一长，`.mewcode/worktrees/` 下会堆积大量无用目录。

怎么区分哪些是该清理的临时 Worktree，哪些是用户手动创建的？靠命名模式。

SubAgent 创建的 Worktree 用 `agent-a` 加 7 位随机 hex 命名（`a` 是固定字面前缀，不是随机 hex 的一部分），比如 `agent-a3f2b1c`。工作流创建的用 `wf_` 前缀加固定格式 hex。用户通过 `/worktree create my-feature` 创建的名称不会匹配这些模式，永远不会被自动清理。

![临时 Worktree 与手动命名的区分](https://feishu.cn/file/B2XmbdwfZoMKzNxMMdPcKIvGnjf)

后台清理的逻辑分三层过滤：

```Plain Text
EPHEMERAL_PATTERNS = [
    "agent-a[0-9a-f]{7}",
    "wf_[0-9a-f]{8}-[0-9a-f]{3}-\d+",
]

function cleanupStaleWorktrees(cutoffDate):
    for wt in listWorktreeDirectories(manager.worktreeDir):
        // 第一层：只清理临时 Worktree
        if not matchesAny(wt.name, EPHEMERAL_PATTERNS):
            continue
        // 第二层：跳过当前使用中和未过期的
        if isCurrentSession(wt) or modTime(wt) > cutoffDate:
            continue

```

通过前两层过滤后，还要做最后一层安全检查：

```Plain Text
        // 第三层：fail-closed 变更检查
        if hasWorktreeChanges(wt.path, wt.headCommit):
            continue

        // 有未推送到远端的 commit 也不删
        unpushed = run("git", "rev-list", "--max-count=1",
            "HEAD", "--not", "--remotes", workDir=wt.path)
        if unpushed.trim() is not empty:
            continue

        removeWorktree(manager, wt.name)

```

即使 Worktree 已经过期，如果里面有未提交的修改或新增 commit，也不删除。

有些子 Agent 可能已经 commit 了代码但还没 push，这种中间状态下直接删除等于丢失工作。宁可多占一些磁盘，也不丢失可能有价值的工作成果。

![过期 Worktree 清理漏斗](https://feishu.cn/file/MX5FbOYvQo0NPJxE7MqckYxrn2d)

---

## 与 SubAgent 的天然配合

Worktree 和 SubAgent 是天生一对。上一章实现的 SubAgent 隔离了上下文：消息、权限、缓存各自独立。Worktree 隔离了文件系统：每个子 Agent 在自己的目录中工作。两者结合，子 Agent 就拥有了真正独立的工作环境。

![SubAgent + Worktree 完整隔离](https://feishu.cn/file/HSgKb7wW2oyJAtxXj2mcg0UInpS)

怎么把它们串起来？通过 Agent 定义中的 `isolation` 字段。

```YAML
# .mewcode/agents/refactor-worker.md
---
name: refactor-worker
description: 在独立工作树中执行重构
disallowedTools:
  - Agent
  - NotebookEdit
maxTurns: 40
isolation: worktree
---

你是一个重构 Agent。在独立的工作树中执行重构任务。
完成后提交你的更改。

```

当 `isolation` 设为 `worktree` 时，SubAgent 的启动流程会自动变成：

1. 创建 Worktree
2. 创建子 Agent，工作目录设为 Worktree 路径
3. 运行子 Agent
4. 子 Agent 完成后，在 Worktree 中提交更改
5. 退出并清理 Worktree
6. 返回结果给主 Agent
7. 主 Agent 决定是否合并

具体到代码层面，关键的实现是 `executeWithWorktree`：

```Plain Text
function executeWithWorktree(ctx, definition, task):
    // 用唯一 ID 命名，避免同类型 Agent 并发冲突
    wtName = "agent-" + generateId()[:8]
    wt = worktreeManager.create(ctx, wtName, "HEAD")

    // 注入 Worktree 上下文通知
    notice = buildWorktreeNotice(getCurrentDirectory(), wt.path)
    task = notice + "\n\n" + task

```

先创建 Worktree，然后在任务文本前面加一段上下文通知。这段通知告诉子 Agent 三件事：你继承了父 Agent 的对话上下文，你当前在一个独立的 Git Worktree 中工作，父 Agent 传来的路径指向的是父目录，你需要翻译成本地路径并在编辑前重新读取文件。

没有这段通知会怎样？子 Agent 不知道自己在隔离副本里。它可能直接使用父 Agent 传来的绝对路径去读写文件，而这些路径指向的是主目录。更隐蔽的问题是：子 Agent 读到了 Worktree 里的文件，却用父 Agent 对话里提到的主目录版本去理解文件内容，产生认知偏差。

![Worktree 上下文通知注入](https://feishu.cn/file/Ad64bHcjrojkuIxui3jcl0xMnDb)

接着创建子 Agent 并执行：

```Plain Text
    subAgent = factory.createFromDefinition(definition)
    subAgent.setWorkDir(wt.path)
    result = subAgent.runToCompletion(ctx, task)

    // 自动清理
    cleanup = autoCleanup(worktreeManager, wtName, wt.headCommit)
    if cleanup.kept:
        result += "\n[Worktree 保留在 " + cleanup.path +
                  "，分支 " + cleanup.branch + "]"
    return result

```

子 Agent 完成后调用 `autoCleanup`：没有修改就自动删除 Worktree，有修改就保留并把路径和分支名追加到返回结果中，让主 Agent 知道去哪里 review。自动提交时可以放心用 `git add -A`，因为 Worktree 是隔离环境，里面只有子 Agent 的改动，不存在主仓库的敏感文件混入问题，和第十一章 Commit Skill 里「逐个文件添加」的场景不同。

![executeWithWorktree 完整生命周期](https://feishu.cn/file/H3bDbbQ79oXC8XxawELcro3fnpc)

最后补上一个辅助函数。`hasWorktreeChanges` 检查两件事：`git status --porcelain` 检测未提交的修改，`git rev-list --count <headCommit>..HEAD` 检测新增的 commit。两个都没有就是干净的。如果 Git 命令本身执行失败，默认返回 true，宁可多保留一个目录也不误删。

---

## 本章小结

Worktree 补上了 SubAgent 缺失的最后一块拼图：文件系统级隔离。

上一章的上下文隔离是逻辑层面的，消息和权限分开了，但文件系统共享。Worktree 提供的是物理层面的隔离，每个子 Agent 在自己的目录中工作，彻底消除文件冲突的可能。

围绕 Worktree 的工程细节不少。创建后需要复制本地配置、配置 git hooks、软链接依赖目录、复制被忽略但需要的文件。进入时不改进程 cwd，而是通过会话状态让工具显式取 Worktree 路径，缓存用绝对路径做 key 所以天然不会冲突。子 Agent 进入时还需要注入上下文通知，告知它在隔离副本中工作。

安全方面，退出时的变更保护防止误删有价值的工作成果。后台过期清理作为安全网，定期扫描并清除孤儿临时 Worktree，清理前会检查未提交修改和未推送的 commit。

最后留一个问题给你想：子 Agent 在 Worktree 里改了 `server.py`，还提交了 commit。主 Agent 想把这些变更合并回主分支，该怎么做？翻一翻 MewCode 的工具列表，会发现没有内置的 merge 工具。那主 Agent 只能用 `Bash` 自己跑 `git merge` 或者 `git cherry-pick`——为什么作者没把 merge 做成内置工具？可以从「主 Agent 应该看到合并冲突」「合并策略因项目而异」「合并失败的恢复路径」几个角度想一想。

有了 SubAgent + Worktree，下一章我们就可以让多个子 Agent 真正并行工作了。

