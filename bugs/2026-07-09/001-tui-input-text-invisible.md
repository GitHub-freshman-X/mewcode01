# 001 TUI 输入框文字在深色光标行中不可见

## 状态

已修复（2026-07-09）

## 用户可见现象

用户在 TUI 输入框中输入内容后，输入区背景显示为黑色，已输入文字也接近黑色，导致几乎看不见输入内容。

## 复现条件

- 运行 MewCode TUI。
- 输入任意文本，例如 `nihao`。
- 在某些终端配色下，textarea focused cursor line 使用深色背景，而正文没有显式前景色，导致文字与背景对比度过低。

## 根因

`internal/tui/model.go` 创建 textarea 后使用 `bubbles/textarea` 默认 dark styles。该默认样式的 focused cursor line 会设置黑色背景，但 focused text 没有显式前景色，会继承终端默认前景色。若终端默认前景色也偏暗，就出现黑底暗字。

## 修复方案

- 新增 `inputStyles()`，为 textarea focused/blurred 状态显式设置正文、光标行、提示符和 placeholder 的前景色。
- 在 `NewModel` 初始化 textarea 时调用 `input.SetStyles(inputStyles())`。
- 新增测试 `TestTextareaFocusedInputHasVisibleStyle`，防止 focused 输入文本再次依赖默认前景色。

## 验证方式

- `go test ./internal/tui -run TextareaFocusedInputHasVisibleStyle`
- `go test ./internal/tui`

## 后续工作

可在真实终端中再次运行 `./mewcode --config <config>`，输入任意文本确认输入区文字为亮色且可读。
