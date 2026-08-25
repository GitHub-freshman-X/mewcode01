package command

import (
	"fmt"
	"strings"
)

func worktreeCommand(ctx CommandContext) error {
	if ctx.Worktrees == nil {
		return fmt.Errorf("worktree manager is not configured")
	}
	args := strings.Fields(ctx.Args)
	if len(args) == 0 {
		ctx.systemf("用法: /worktree create <name>|list|enter <name>|exit|remove <name> --discard")
		return nil
	}
	switch args[0] {
	case "create":
		if len(args) != 2 {
			ctx.systemf("用法: /worktree create <name>")
			return nil
		}
		wt, err := ctx.Worktrees.Create(ctx.Context, args[1])
		if err != nil {
			return err
		}
		ctx.systemf("Worktree 已就绪：%s\n分支：%s\nHEAD：%s", wt.Path, wt.Branch, wt.HeadCommit)
	case "list":
		items, err := ctx.Worktrees.List(ctx.Context)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			ctx.systemf("没有 Worktree。")
			return nil
		}
		var lines []string
		for _, wt := range items {
			lines = append(lines, fmt.Sprintf("%s — %s (%s)", wt.Name, wt.Path, wt.Branch))
		}
		ctx.systemf("%s", strings.Join(lines, "\n"))
	case "enter":
		if len(args) != 2 {
			ctx.systemf("用法: /worktree enter <name>")
			return nil
		}
		s, err := ctx.Worktrees.Enter(ctx.Context, args[1])
		if err != nil {
			return err
		}
		ctx.systemf("已进入 Worktree：%s", s.WorktreePath)
	case "exit":
		if len(args) != 1 {
			ctx.systemf("用法: /worktree exit")
			return nil
		}
		if err := ctx.Worktrees.Exit(); err != nil {
			return err
		}
		ctx.systemf("已退出 Worktree 会话。")
	case "remove":
		if len(args) < 2 || len(args) > 3 {
			ctx.systemf("用法: /worktree remove <name> --discard")
			return nil
		}
		discard := len(args) == 3 && args[2] == "--discard"
		if len(args) == 3 && !discard {
			ctx.systemf("仅支持 --discard 作为确认参数。")
			return nil
		}
		if err := ctx.Worktrees.Remove(ctx.Context, args[1], discard); err != nil {
			return err
		}
		ctx.systemf("Worktree 已删除。")
	default:
		ctx.systemf("未知 worktree 操作 %q。", args[0])
	}
	return nil
}
