# 第十二章 Hook 配置被主配置严格解析拒绝

- 状态：已修复，自动化验证通过
- 发现日期：2026-08-21
- 影响范围：在主配置文件中声明 `hooks` 的所有启动方式。

## 现象

执行 `mewcode --config .mewcode/config.yaml` 时，若该文件含 `hooks`，启动在配置解析阶段失败，并报 `field hooks not found in type config.rawConfig`。

## 根因

主配置加载器开启了 YAML 严格字段检查，但其内部 `rawConfig` 未声明 `hooks`。Hook 引擎虽会在后续单独读取同一文件的 `hooks` 字段，却无法到达该步骤。

## 修复方案

在主配置加载器的中间结构中声明并忽略 `hooks`，保留严格检查对其他未知顶层字段的保护；Hook 规则继续由 `internal/hooks` 负责解析和校验。

## 验证方式

2026-08-21：新增 `TestLoadAcceptsHooksForSeparateHookLoader`，验证带合法 Hook 规则的主配置可被加载。随后运行 `go test ./internal/config ./internal/hooks ./cmd/mewcode -count=1`。
