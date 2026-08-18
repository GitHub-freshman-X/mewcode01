package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/skills"
)

const sessionTitleLimit = 48

func DefaultCommands() []Command {
	return []Command{
		{Name: "help", Aliases: []string{"h"}, Description: "显示命令帮助", Usage: "/help [命令]", Kind: KindLocal, Handler: helpCommand},
		{Name: "exit", Description: "退出 MewCode", Usage: "/exit", Kind: KindLocalUI, Handler: exitCommand},
		{Name: "compact", Description: "压缩上下文", Usage: "/compact", Kind: KindPrompt, Handler: requestCommand(agent.ModeCompact, "")},
		{Name: "clear", Description: "新建并切换会话", Usage: "/clear", Kind: KindLocalUI, Handler: clearCommand},
		{Name: "plan", Description: "切换或提交计划", Usage: "/plan [需求]", Kind: KindLocalUI, Handler: planCommand},
		{Name: "do", Description: "执行待执行计划", Usage: "/do", Kind: KindLocalUI, Handler: doCommand},
		{Name: "session", Aliases: []string{"s"}, Description: "管理会话", Usage: "/session [list|new|resume <id>|delete <id>]", ArgPrompt: "用法: /session resume <id>", Kind: KindLocal, Handler: sessionCommand},
		{Name: "memory", Aliases: []string{"m"}, Description: "管理记忆", Usage: "/memory [list|add <类别> <内容>|clear]", ArgPrompt: "用法: /memory add <类别> <内容>", Kind: KindLocal, Handler: memoryCommand},
		{Name: "status", Description: "显示当前状态", Usage: "/status", Kind: KindLocal, Handler: statusCommand},
	}
}

func SkillCommands(directory []skills.Metadata) []Command {
	commands := []Command{{Name: "skills", Description: "管理 Skill", Usage: "/skills reload", Kind: KindLocal, Handler: skillsCommand}}
	for _, metadata := range directory {
		metadata := metadata
		commands = append(commands, Command{Name: metadata.Name, Description: metadata.Description, Usage: "/" + metadata.Name + " [参数]", Kind: KindPrompt, Handler: func(ctx CommandContext) error {
			if ctx.UI == nil {
				return fmt.Errorf("command UI is not configured")
			}
			prompt := "/" + metadata.Name
			if args := strings.TrimSpace(ctx.Args); args != "" {
				prompt += " " + args
			}
			return ctx.UI.StartAgent(agent.Request{Mode: agent.ModeAct, Prompt: prompt, Skill: &agent.SkillInvocation{Name: metadata.Name, Args: ctx.Args}})
		}})
	}
	return commands
}

func skillsCommand(ctx CommandContext) error {
	if strings.TrimSpace(ctx.Args) != "reload" {
		ctx.systemf("用法: /skills reload")
		return nil
	}
	if ctx.Skills == nil {
		return fmt.Errorf("skill service is not configured")
	}
	if err := ctx.Skills.ReloadSkills(); err != nil {
		return err
	}
	if refresher, ok := ctx.UI.(interface{ RefreshCommands() }); ok {
		refresher.RefreshCommands()
	}
	ctx.systemf("Skill 已刷新。")
	return nil
}

func Dispatch(registry *Registry, invocation Invocation, ctx CommandContext) error {
	if !invocation.IsCommand {
		return nil
	}
	if invocation.Name == "" {
		ctx.systemf("可用命令:\n%s", helpList(registry))
		return nil
	}
	command, ok := registry.Find(invocation.Name)
	if !ok {
		ctx.systemf("未知命令 /%s；请输入 /help 查看可用命令。", invocation.Name)
		return nil
	}
	if command.ArgPrompt != "" && needsArgs(command, invocation.Args) {
		ctx.systemf("%s", command.ArgPrompt)
		return nil
	}
	started := time.Now()
	err := command.Handler(CommandContext{Context: ctx.Context, UI: ctx.UI, Registry: registry, Sessions: ctx.Sessions, Memory: ctx.Memory, Skills: ctx.Skills, Logger: ctx.Logger, Args: invocation.Args})
	if ctx.Logger != nil {
		status := "completed"
		if err != nil {
			status = "failed"
		}
		ctx.Logger.Info("command executed", logging.Fields{"stage": "command", "command": command.Name, "kind": string(command.Kind), "status": status, "duration_ms": time.Since(started).Milliseconds()})
	}
	return err
}

func needsArgs(command Command, args string) bool {
	return command.Name == "session" && (strings.HasPrefix(args, "resume") || strings.HasPrefix(args, "delete")) && len(strings.Fields(args)) < 2 || command.Name == "memory" && strings.HasPrefix(args, "add") && len(strings.Fields(args)) < 3
}

func helpList(registry *Registry) string {
	var b strings.Builder
	for _, c := range registry.Visible() {
		fmt.Fprintf(&b, "/%s", c.Name)
		if len(c.Aliases) > 0 {
			fmt.Fprintf(&b, " (/%s)", strings.Join(c.Aliases, ", /"))
		}
		fmt.Fprintf(&b, " — %s\n", c.Description)
	}
	return strings.TrimSpace(b.String())
}
func helpCommand(ctx CommandContext) error {
	registry := ctx.Registry
	if registry == nil {
		registry = DefaultRegistry()
	}
	if ctx.Args == "" {
		ctx.systemf("可用命令:\n%s", helpList(registry))
		return nil
	}
	c, ok := registry.Find(ctx.Args)
	if !ok {
		ctx.systemf("未知命令 /%s", ctx.Args)
		return nil
	}
	ctx.systemf("/%s\n%s\n用法: %s", c.Name, c.Description, c.Usage)
	return nil
}
func exitCommand(ctx CommandContext) error {
	if ctx.UI == nil {
		return fmt.Errorf("command UI is not configured")
	}
	ctx.UI.RequestExit()
	return nil
}
func requestCommand(mode agent.Mode, prompt string) Handler {
	return func(ctx CommandContext) error {
		if ctx.UI == nil {
			return fmt.Errorf("command UI is not configured")
		}
		return ctx.UI.StartAgent(agent.Request{Mode: mode, Prompt: prompt})
	}
}
func clearCommand(ctx CommandContext) error {
	if ctx.Sessions == nil {
		return fmt.Errorf("session service is not configured")
	}
	if err := ctx.Sessions.New(ctx.Context); err != nil {
		return err
	}
	ctx.systemf("已创建新会话。")
	return nil
}
func planCommand(ctx CommandContext) error {
	if ctx.UI == nil {
		return fmt.Errorf("command UI is not configured")
	}
	if ctx.Args == "" {
		ctx.UI.SetPlanMode(!ctx.UI.PlanMode())
		return nil
	}
	ctx.UI.SetPlanMode(true)
	return ctx.UI.StartAgent(agent.Request{Mode: agent.ModePlan, Prompt: ctx.Args})
}
func doCommand(ctx CommandContext) error {
	if ctx.UI == nil {
		return fmt.Errorf("command UI is not configured")
	}
	ctx.UI.SetPlanMode(false)
	return ctx.UI.StartAgent(agent.Request{Mode: agent.ModeDo})
}
func sessionCommand(ctx CommandContext) error {
	if ctx.Sessions == nil {
		return fmt.Errorf("session service is not configured")
	}
	parts := strings.Fields(ctx.Args)
	if len(parts) == 0 {
		m := ctx.Sessions.Current()
		ctx.systemf("当前会话: %s — %s", m.ID, formatSessionTitle(m.Title))
		return nil
	}
	switch parts[0] {
	case "list":
		metas, err := ctx.Sessions.List()
		if err != nil {
			return err
		}
		var lines []string
		for _, m := range metas {
			lines = append(lines, fmt.Sprintf("%s — %s", m.ID, formatSessionTitle(m.Title)))
		}
		ctx.systemf("会话:\n%s", strings.Join(lines, "\n"))
	case "new":
		return ctx.Sessions.New(ctx.Context)
	case "resume":
		return ctx.Sessions.Resume(ctx.Context, parts[1])
	case "delete":
		return ctx.Sessions.Delete(parts[1])
	default:
		ctx.systemf("用法: /session [list|new|resume <id>|delete <id>]")
	}
	return nil
}

func formatSessionTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "（空会话）"
	}
	runes := []rune(title)
	if len(runes) <= sessionTitleLimit {
		return title
	}
	return string(runes[:sessionTitleLimit]) + "…"
}
func memoryCommand(ctx CommandContext) error {
	if ctx.Memory == nil {
		return fmt.Errorf("memory service is not configured")
	}
	parts := strings.Fields(ctx.Args)
	if len(parts) == 0 {
		s, err := ctx.Memory.Summary()
		if err != nil {
			return err
		}
		ctx.systemf("记忆: 用户 %d 条，项目 %d 条", s.UserCount, s.ProjectCount)
		return nil
	}
	switch parts[0] {
	case "list":
		items, err := ctx.Memory.Items()
		if err != nil {
			return err
		}
		var lines []string
		for _, i := range items {
			lines = append(lines, fmt.Sprintf("%s/%s — %s", i.Kind, i.Name, i.Description))
		}
		ctx.systemf("记忆:\n%s", strings.Join(lines, "\n"))
	case "add":
		return ctx.Memory.Add(parts[1], strings.TrimSpace(strings.TrimPrefix(ctx.Args, parts[0]+" "+parts[1])))
	case "clear":
		if ctx.UI == nil || !ctx.UI.MemoryClearPending() {
			if ctx.UI != nil {
				ctx.UI.SetMemoryClearPending(true)
			}
			ctx.systemf("请再次输入 /memory clear 确认清空记忆。")
			return nil
		}
		ctx.UI.SetMemoryClearPending(false)
		if err := ctx.Memory.Clear(); err != nil {
			return err
		}
		ctx.systemf("记忆已清空。")
	default:
		ctx.systemf("用法: /memory [list|add <类别> <内容>|clear]")
	}
	return nil
}
func statusCommand(ctx CommandContext) error {
	usage := ctx.UI.TokenUsage()
	mode := "DEFAULT"
	if ctx.UI.PlanMode() {
		mode = "PLAN"
	}
	ctx.systemf("[%s] tokens in:%d out:%d", mode, usage.InputTokens, usage.OutputTokens)
	return nil
}
func DefaultRegistry() *Registry { r, _ := NewRegistry(DefaultCommands()...); return r }
