# MewCode 纯对话基础 Checklist

> 每一项都通过运行命令、自动化测试或观察终端行为验证。验收不需要访问真实 Anthropic/OpenAI 服务，也不需要真实 API Key。

## 实现完整性

- [ ] **C1 配置入口完整**：程序能够使用系统默认路径，也能接受 `--config` 指定路径。（验证：运行 CLI 配置路径测试，期望两条路径均被正确传给加载器。）
- [ ] **C2 配置模型完整**：YAML 支持 `protocol`、`model`、`base_url`、`api_key`、可选 `max_tokens` 和 thinking 设置，默认 token 为 4096。（验证：运行配置加载测试，期望最小配置和完整配置均通过。）
- [ ] **C3 Provider 抽象可用**：Anthropic 与 OpenAI 均可通过同一对话入口启动流，TUI 不接触供应商 JSON。（验证：运行 factory、conversation 和 TUI 测试，期望同一 fake/统一事件路径覆盖两种 Provider。）
- [ ] **C4 SSE 解码器可复用**：同一解码器可处理两种 Provider 的 event/data 帧。（验证：运行 SSE 与两个 Provider 测试，期望全部通过。）
- [ ] **C5 Conversation 生命周期完整**：开始、增量、完成、失败和取消状态均可观察，且只有完成轮次进入历史。（验证：运行 conversation 测试，期望所有状态与提交断言通过。）
- [ ] **C6 全屏 TUI 组件完整**：消息 viewport、textarea、状态栏和 thinking 面板均可渲染。（验证：运行 TUI View 测试并启动本地 mock 场景，期望四类区域可见。）
- [ ] **C7 CLI 组装完整**：配置、HTTP Client、Provider、Conversation 与 TUI 按顺序组装，初始化错误返回非零。（验证：运行 `go test ./cmd/mewcode -count=1`，期望成功和失败路径均通过。）
- [ ] **C8 用户资料完整**：README、配置示例、忽略规则和 CI workflow 均存在。（验证：运行 `test -f README.md && test -f config.example.yaml && test -f .gitignore && test -f .github/workflows/ci.yml`，期望退出码为 0。）

## 功能验收

- [ ] **C9 默认路径和覆盖路径（AC1）**：未提供参数时使用系统用户配置目录；提供 `--config` 时只读取指定文件。（验证：运行 CLI 自动化测试，期望两种场景记录的加载路径正确。）
- [ ] **C10 缺失核心字段（AC1）**：逐一缺失四个核心字段时，程序在进入 TUI 前失败并指出缺失字段。（验证：运行表驱动配置测试，期望四个场景全部返回 config 错误且 TUI 启动次数为 0。）
- [ ] **C11 严格配置校验（AC2）**：非法 YAML、未知字段、多文档、非法协议、非法 URL、非正数 max tokens 和非法 thinking 预算均被拒绝。（验证：运行 `go test ./internal/config -count=1`，期望全部通过。）
- [ ] **C12 配置失败不发送请求（AC2）**：配置无效时测试 HTTP 服务收到的请求数为 0。（验证：运行配置到启动层的失败集成测试，期望请求计数为 0。）
- [ ] **C13 TUI 基础输入（AC3）**：初始界面包含消息区、输入区和状态区；提交中英文文本后用户消息可见。（验证：运行 TUI 输入/View 测试，期望输出包含原始 UTF-8 文本。）
- [ ] **C14 增量显示（AC4）**：服务端先发送第一段并保持连接时，第一段已经出现在 View；第二段到达后再扩展文本。（验证：运行延迟 SSE 流式测试，期望连接关闭前至少观察到两个不同的中间 View。）
- [ ] **C15 当前进程多轮上下文（AC5）**：第二轮请求包含第一轮用户与完整助手回复，并保持顺序。（验证：运行两轮集成测试，检查测试服务记录的第二个请求体。）
- [ ] **C16 重启不恢复历史（AC5）**：新建应用实例后的第一条请求不包含旧实例历史。（验证：连续创建两个 Conversation/应用实例，期望第二实例首个请求只有当前用户消息。）
- [ ] **C17 Provider 可切换（AC6）**：仅修改 `protocol` 和对应连接信息即可让相同上层流程分别驱动 Anthropic/OpenAI。（验证：对两份临时 YAML 运行同一端到端 harness，期望都完成对话且 TUI 事件类型一致。）
- [ ] **C18 Anthropic 请求与流（AC7）**：请求到达保留 base path 的 `/v1/messages`，认证和版本头正确，thinking/text 增量被完整组合。（验证：运行 `go test ./internal/provider/anthropic -count=1`，期望路径、头、请求体和事件断言全部通过。）
- [ ] **C19 OpenAI 请求与流（AC8）**：请求到达保留 base path 的 `/responses`，Bearer 认证正确，output text 增量被完整组合。（验证：运行 `go test ./internal/provider/openai -count=1`，期望路径、头、请求体和事件断言全部通过。）
- [ ] **C20 Extended Thinking（AC9）**：开启时 thinking 增量实时显示，signature 不显示，完成后自动折叠，`Ctrl+T` 可反复切换。（验证：运行 Anthropic thinking 与 TUI thinking 测试，期望每个阶段的 View 符合状态。）
- [ ] **C21 Thinking 关闭行为（AC9）**：关闭后 Anthropic 请求不含 thinking 配置，界面不创建 thinking 区域。（验证：运行关闭 thinking 场景，检查请求 JSON 和 View。）
- [ ] **C22 Thinking 多轮连续性（AC9）**：完成的 thinking block 与 signature 在下一轮 Anthropic 请求中原样返回。（验证：运行两轮 thinking 测试，对比第一轮收到与第二轮发送的 block 内容。）
- [ ] **C23 错误恢复（AC10）**：401/403、429、5xx、连接中断、malformed SSE 和缺少完成事件均显示安全错误并恢复输入。（验证：运行 Provider 与 TUI 错误矩阵测试，期望每种错误后都可开始下一轮。）
- [ ] **C24 未知事件兼容（AC10）**：流中插入未知事件不崩溃，后续有效 delta 和 completed 仍被处理。（验证：运行两种 Provider 的未知事件测试，期望最终文本完整且终结结果成功。）
- [ ] **C25 生成取消（AC11）**：生成中 `Ctrl+C` 取消 HTTP context、显示 cancelled、恢复输入，并且下一轮不带取消轮次的部分内容。（验证：运行阻塞 SSE 的取消恢复测试，检查服务端连接关闭和下一请求体。）
- [ ] **C26 空闲退出（AC11）**：空闲时 `Ctrl+C` 产生正常退出，且没有活动网络 goroutine。（验证：运行 TUI 退出测试并配合 race 检查，期望退出命令产生且无竞态/泄漏症状。）
- [ ] **C27 滚动、跟随与 resize（AC12）**：长内容可滚动；离开底部后新增文本不拉回；回到底部恢复跟随；窗口缩放后仍可操作。（验证：运行 TUI scroll/resize 测试，覆盖正常和极小尺寸。）
- [ ] **C28 生成后继续输入（AC12）**：完成、失败或取消后 textarea 都重新获得焦点并可提交新问题。（验证：分别注入三种终结结果，期望下一次 Enter 均启动新请求。）

## 非功能验收

- [ ] **C29 三平台构建（AC13）**：Linux、macOS、Windows 目标均可构建。（验证：分别以 `GOOS=linux`、`GOOS=darwin`、`GOOS=windows` 运行 `go build -o` 到临时目录，期望全部成功。）
- [ ] **C30 UTF-8 完整性（AC13）**：中文、英文、emoji 和代码符号经过输入、历史、JSON、分段 SSE 与 View 后不损坏。（验证：运行 UTF-8 端到端测试，对比原始和最终字符串完全相等。）
- [ ] **C31 流期间响应性（AC14）**：流未结束时仍能处理滚动、resize 与取消消息。（验证：在测试流阻塞点依次发送三个 TUI 消息，期望每次 Update 立即返回并改变对应状态。）
- [ ] **C32 异常流稳定性（AC14）**：空流、空事件、超大帧、不完整 JSON、EOF 和 context cancellation 都不产生 panic。（验证：运行 SSE/Provider 错误测试和 `go test -race ./...`，期望全部通过。）
- [ ] **C33 密钥不泄露（AC15）**：配置、Provider、CLI、TUI 错误和 verbose 测试输出均不出现注入的 canary API Key。（验证：运行带 canary 的错误测试并扫描捕获的 stderr/View/错误字符串，期望零匹配。）
- [ ] **C34 明文密钥警告（AC15）**：README 明确说明 API Key 明文存储、限制文件权限及避免提交版本控制。（验证：运行 `rg -n 'API Key|明文|权限|版本控制' README.md`，期望四类说明均存在。）
- [ ] **C35 离线自动测试（AC16）**：测试在未设置任何供应商环境变量时通过，且只连接本地测试服务。（验证：清除相关环境变量后运行 `go test ./... -count=1`，期望全部通过且无外网请求。）
- [ ] **C36 分层与无环依赖（AC16）**：配置、Provider、Conversation、TUI 均有真实调用方，包依赖无环。（验证：运行 `go list -deps ./...` 与 `go test ./...`，期望成功；循环依赖会使命令失败。）

## 编译与质量检查

- [ ] **C37 格式正确**：所有 Go 文件已格式化。（验证：运行 `gofmt -l .`，期望无输出。）
- [ ] **C38 Module 一致**：module path、Go 版本和依赖锁定符合 Plan。（验证：运行 `go mod edit -json` 检查 path/GoVersion；运行 `go mod tidy` 后 `git diff -- go.mod go.sum` 无新增变化。）
- [ ] **C39 依赖校验通过**：下载模块与校验和一致。（验证：运行 `go mod verify`，期望输出 `all modules verified`。）
- [ ] **C40 全部测试通过**：所有单元与集成测试成功。（验证：运行 `go test ./... -count=1`，期望所有包 PASS。）
- [ ] **C41 静态检查通过**：标准 Go 静态检查无问题。（验证：运行 `go vet ./...`，期望无输出且退出码为 0。）
- [ ] **C42 Race 检查通过**：流、取消、通道和测试服务没有数据竞争。（验证：运行 `go test -race ./... -count=1`，期望全部通过且无 DATA RACE。）
- [ ] **C43 当前平台构建通过**：CLI 可从源码构建。（验证：运行 `go build -o /tmp/mewcode-check ./cmd/mewcode`，期望生成可执行文件且退出码为 0。）
- [ ] **C44 CI 定义完整**：workflow 覆盖 ubuntu、macOS、Windows，并包含 test、vet、build、Linux race。（验证：检查 workflow 关键项并在首次 push 后观察 GitHub Actions 全绿。）

## 端到端场景

- [ ] **E1 Anthropic thinking 多轮对话**：临时 YAML 指向本地 Anthropic 模拟服务；用户提交中文问题；thinking 与正文分段显示；完成后 thinking 折叠；第二轮请求携带第一轮完整文本、thinking 和 signature。（验证：运行 `go test ./... -run TestE2EAnthropicThinkingConversation -count=1`，期望 PASS。）
- [ ] **E2 OpenAI Responses 多轮对话**：临时 YAML 指向本地 OpenAI 模拟服务；两轮回复均逐段显示；第二轮 input 包含第一轮完整用户与助手文本。（验证：运行 `go test ./... -run TestE2EOpenAIConversation -count=1`，期望 PASS。）
- [ ] **E3 取消后继续**：模拟服务发送部分文本后阻塞；用户取消；连接关闭、部分回复不提交；随后新问题成功完成。（验证：运行 `go test ./... -run TestE2ECancelAndContinue -count=1`，期望 PASS。）
- [ ] **E4 非法配置安全退出**：以包含 canary 密钥和非法 thinking 预算的 YAML 启动；程序在 TUI 前退出，不请求服务，stderr 指出字段但不含密钥。（验证：运行 `go test ./... -run TestE2EInvalidConfigSafeExit -count=1`，期望 PASS。）

## 验收完成条件

- C1–C44 与 E1–E4 全部勾选。
- 每项记录实际命令输出或可观察结果；不得以“代码看起来正确”代替运行证据。
- 任一失败项修复后，重新执行该项及受影响的完整测试集。
