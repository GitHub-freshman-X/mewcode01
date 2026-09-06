# 测试运行目录隔离 Checklist

## 自动化验收

- [x] **AC1 源码目录隔离**：从无目标运行目录的状态执行 `go test ./cmd/mewcode -count=1` 后，测试通过，且 `cmd/mewcode/.mewcode`、`cmd/mewcode/logs` 均不存在。
- [x] **AC2 启动测试行为保持有效**：配置覆盖、权限成功、权限失败和配置失败测试仍保持其既有的配置路径、退出码或错误消息断言。
- [x] **AC3 生产语义未改变**：修改仅位于测试文件；`launch` 继续以 CLI 的当前目录作为项目根，不新增配置、环境变量或依赖。

## 集成验收

- [x] `go test ./cmd/mewcode -count=1` 通过。
- [x] `git diff --check` 通过。
- [x] 本轮验证未在源码 `cmd/mewcode/` 目录留下 `.mewcode/` 或 `logs/`。

## 手工复核

- [ ] 从一个用户项目目录手动启动 CLI 时，日志与会话仍按既有设计写入该用户项目目录；该真实运行行为不是本次测试隔离修复的回归对象。
