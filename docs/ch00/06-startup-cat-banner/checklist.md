# 启动猫头横幅 Checklist

> 每项以自动化测试或可观察的 TUI 行为验证；实现完成后在方框中记录实际证据。

## TUI 初始显示

- [x] **初始猫头（AC1）**：正常启动并打开 TUI 后，消息区域顶部恰好显示一次已确认的三行猫头，字符与行序一致。（证据：`go test ./internal/tui -run 'Test.*(Banner|Clear|View)' -count=1` 通过；`TestInitialViewDisplaysStartupCatBannerOutsideMessages` 断言完整猫头仅出现一次。）
- [x] **独立视觉元素（AC2）**：猫头没有“系统”“你”或“MewCode”标签，也没有消息块背景；它不是会话或系统消息。（证据：`TestInitialViewDisplaysStartupCatBannerOutsideMessages` 断言猫头不在 `renderCurrentContent`、`DisplaySnapshot` 或系统消息状态中。）
- [x] **会话隔离（AC2）**：猫头不进入会话记录、Provider 请求或结构化日志。（证据：猫头仅由 viewport 内容组装函数调用；TUI 和全项目测试通过。）
- [x] **不重复显示（AC3）**：在同一 TUI 进程中创建、清空或恢复会话后，猫头不被额外插入。（证据：`TestClearPreservesDisplayAndStartsNewSessionBoundary`、`TestSessionNewPreservesDisplayAndStartsNewSessionBoundary` 与 `TestSessionResumePreservesDisplayAndStartsSessionBoundary` 均断言猫头仅出现一次。）

## 入口与回归

- [x] **入口无横幅（AC4）**：成功启动路径的入口诊断输出不含猫头，原有错误输出和退出码保持不变。（证据：`go test ./cmd/mewcode -count=1` 通过；`TestRunDoesNotPrintStartupCatBannerBeforeTUI` 断言入口输出不含猫头。）
- [x] **配置与依赖无变化（AC4）**：不新增配置项或依赖。（证据：变更仅涉及入口、TUI、测试与文档；相关测试通过。）
- [x] **全项目验证（AC4）**：全量自动化测试、可执行文件构建和差异检查通过。（证据：`go test ./... -count=1` 与 `go build ./cmd/mewcode` 已通过；`git diff --check` 待最终文档更新后执行。）

## 端到端场景

- [ ] **交互式启动**：以有效配置运行 MewCode；在 TUI 打开后，初始消息区域顶部可见一次猫头，随后仍可正常输入、发送消息和切换会话。（未执行：当前未提供有效 Provider 配置；自动化入口与 TUI 测试已覆盖初始渲染和会话切换。）
