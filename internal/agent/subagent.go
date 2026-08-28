package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	contextmanager "github.com/GitHub-freshman-X/mewcode01/internal/context"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/subagent"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
	"github.com/GitHub-freshman-X/mewcode01/internal/worktree"
)

const forkBoilerplate = `<fork_boilerplate>
You are a forked worker, not the primary agent. Do not fork again, do not ask the user questions, and work only on the assigned task. Finish with a concise report beginning with Scope:.
</fork_boilerplate>`

type SubAgentRuntime struct {
	Definitions       *subagent.Registry
	Tasks             *subagent.TaskManager
	AutoBackground    time.Duration
	Worktrees         *worktree.Manager
	DisableBackground bool

	mu         sync.Mutex
	foreground map[*Runner]*foregroundSubAgent
}

type foregroundSubAgent struct {
	id              string
	background      chan struct{}
	allowBackground bool
	once            sync.Once
}

func NewSubAgentRuntime(definitions *subagent.Registry, tasks *subagent.TaskManager) *SubAgentRuntime {
	if tasks == nil {
		tasks = subagent.NewTaskManager()
	}
	return &SubAgentRuntime{Definitions: definitions, Tasks: tasks, AutoBackground: 120 * time.Second, foreground: make(map[*Runner]*foregroundSubAgent)}
}

type runnerSubAgentHost struct {
	runner  *Runner
	runtime *SubAgentRuntime
}

func (h runnerSubAgentHost) DispatchSubAgent(ctx context.Context, input tools.AgentInput) (tools.Result, error) {
	if h.runtime == nil || h.runner == nil {
		return tools.Result{}, fmt.Errorf("subagent runtime is not configured")
	}
	return h.runtime.dispatch(ctx, h.runner, input)
}

type forkSourceKey struct{}

func withForkSource(ctx context.Context) context.Context {
	return context.WithValue(ctx, forkSourceKey{}, true)
}
func isForkSource(ctx context.Context) bool {
	value, _ := ctx.Value(forkSourceKey{}).(bool)
	return value
}

func (r *SubAgentRuntime) dispatch(ctx context.Context, parent *Runner, input tools.AgentInput) (tools.Result, error) {
	if r == nil || r.Tasks == nil {
		return tools.Result{}, fmt.Errorf("subagent runtime is not configured")
	}
	fork := isForkType(input.SubagentType)
	if fork && isForkSource(ctx) {
		return tools.Failure("agent", tools.ErrorPermission, "forked subagents cannot fork again", nil), nil
	}
	background := input.RunInBackground || fork
	if background && r.DisableBackground {
		return tools.Failure("agent", tools.ErrorPermission, "background subagents are disabled", nil), nil
	}
	child, err := r.newChild(parent, input, fork, background)
	if err != nil {
		return tools.Result{}, err
	}
	launchCtx := ctx
	var foregroundCancel context.CancelFunc
	if background {
		launchCtx = context.WithoutCancel(ctx)
	} else {
		launchCtx, foregroundCancel = foregroundSubAgentContext(ctx)
	}
	info, err := r.Tasks.Launch(launchCtx, subagent.LaunchRequest{
		Name:        taskName(input),
		Description: input.Description,
		Background:  background,
		Worker:      child.worker,
		OnTerminal: func(task subagent.TaskInfo) {
			logSubAgentTerminal(parent.options.Logger, childMode(fork), child.runner.options.Model, task)
		},
	})
	if err != nil {
		if foregroundCancel != nil {
			foregroundCancel()
		}
		return tools.Result{}, err
	}
	parent.options.Logger.Info("subagent launched", logging.Fields{"stage": "subagent", "status": "running", "mode": string(childMode(fork)), "background": background, "model": input.Model})
	if background {
		return asyncResult(info), nil
	}
	front := &foregroundSubAgent{id: info.ID, background: make(chan struct{}), allowBackground: child.isolated}
	r.mu.Lock()
	r.foreground[parent] = front
	r.mu.Unlock()
	defer r.clearForeground(parent, front)
	timeout := r.AutoBackground
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var timerC <-chan time.Time
	if child.isolated && !r.DisableBackground {
		timerC = timer.C
	}
	select {
	case <-front.background:
		r.Tasks.MarkBackground(info.ID)
		return asyncResult(info), nil
	case <-timerC:
		r.Tasks.MarkBackground(info.ID)
		return asyncResult(info), nil
	case <-ctx.Done():
		foregroundCancel()
		return tools.Result{}, ctx.Err()
	case <-waitTask(r.Tasks, info.ID):
		finished, _ := r.Tasks.Get(info.ID)
		return finishedResult(finished), nil
	}
}

func waitTask(tasks *subagent.TaskManager, id string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		_, _ = tasks.Wait(context.Background(), id)
		close(done)
	}()
	return done
}

func (r *SubAgentRuntime) HasForeground(parent *Runner) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	_, ok := r.foreground[parent]
	r.mu.Unlock()
	return ok
}

func (r *SubAgentRuntime) BackgroundForeground(parent *Runner) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	front := r.foreground[parent]
	r.mu.Unlock()
	if front == nil {
		return false
	}
	if !front.allowBackground {
		return false
	}
	front.once.Do(func() { close(front.background) })
	return true
}

func (r *SubAgentRuntime) clearForeground(parent *Runner, want *foregroundSubAgent) {
	r.mu.Lock()
	if r.foreground[parent] == want {
		delete(r.foreground, parent)
	}
	r.mu.Unlock()
}

type childRun struct {
	runner   *Runner
	request  Request
	fork     bool
	isolated bool
	worktree *worktree.Worktree
	manager  *worktree.Manager
}

func (c childRun) worker(ctx context.Context, progress func(subagent.Progress)) subagent.Outcome {
	if c.fork {
		ctx = withForkSource(ctx)
	}
	task, err := c.runner.Start(ctx, c.request)
	if err != nil {
		return subagent.Outcome{Status: subagent.TaskFailed, Failure: safeFailure(err)}
	}
	var outcome subagent.Outcome
	for event := range task.Events {
		if event.Type == EventToolCall {
			outcome.ToolCalls++
		}
		if event.Usage != nil {
			outcome.Usage.Add(*event.Usage)
		}
		if event.Summary != nil {
			outcome.Usage = event.Summary.Usage
		}
		progress(subagent.Progress{Iterations: event.Iteration, ToolCalls: outcome.ToolCalls, Usage: outcome.Usage})
		if !isTerminal(event.Type) {
			continue
		}
		switch event.Type {
		case EventCompleted:
			outcome.Status = subagent.TaskCompleted
			history := c.runner.session.DisplaySnapshot()
			if len(history) > 0 {
				outcome.Result = messageText(history[len(history)-1])
			}
		case EventCancelled:
			outcome.Status, outcome.Failure = subagent.TaskCancelled, "subagent cancelled"
		default:
			outcome.Status, outcome.Failure = subagent.TaskFailed, safeFailure(event.Err)
		}
	}
	if outcome.Status == "" {
		outcome.Status, outcome.Failure = subagent.TaskFailed, "subagent ended without terminal event"
	}
	if c.worktree != nil && c.manager != nil {
		if err := c.manager.AutoCleanup(context.Background(), *c.worktree); err != nil {
			outcome.Result = strings.TrimSpace(outcome.Result + "\n\nWorktree retained: " + c.worktree.Path + " (branch " + c.worktree.Branch + ")")
		}
	}
	return outcome
}

func (r *SubAgentRuntime) newChild(parent *Runner, input tools.AgentInput, fork, background bool) (childRun, error) {
	if parent == nil {
		return childRun{}, fmt.Errorf("parent runner is required")
	}
	var registry *tools.Registry
	var err error
	var definition subagent.Definition
	options := parent.options
	options.SessionStore, options.SessionID, options.Memory = nil, "", nil
	options.SubAgents = r
	options.Context = cloneContextConfig(parent.options.Context)
	options.Permissions = clonePermissions(parent.options.Permissions)
	session := conversation.NewSession()
	prompt := input.Prompt
	isolated := fork || input.Isolation == "worktree"
	var wt *worktree.Worktree
	if fork {
		session.ReplaceHistory(completeForkHistory(parent.session.Snapshot()))
		registry, err = parent.registry.Subset(nil, nil)
		if err != nil {
			return childRun{}, err
		}
		options.SystemPrompt = parent.lastSystemPrompt()
		prompt = forkBoilerplate + "\n\n" + input.Prompt
	} else {
		if r.Definitions == nil {
			return childRun{}, fmt.Errorf("agent definition registry is not configured")
		}
		var ok bool
		definition, ok = r.Definitions.Get(input.SubagentType)
		if !ok {
			return childRun{}, fmt.Errorf("unknown subagent type %q", input.SubagentType)
		}
		isolated = isolated || definition.Isolation == "worktree"
		names, err := subagent.FilterTools(parent.registry.Names(), definition, background)
		if err != nil {
			return childRun{}, err
		}
		registry, err = parent.registry.Subset(names, nil)
		if err != nil {
			return childRun{}, err
		}
		options.SystemPrompt = definition.SystemPrompt
		options.MaxIterations = definition.MaxTurns
		if options.MaxIterations == 0 {
			options.MaxIterations = parent.options.MaxIterations
		}
		if definition.PermissionMode != "" && options.Permissions != nil {
			options.Permissions.Mode = definition.PermissionMode
		}
		if input.Model != "" && input.Model != "inherit" {
			options.Model = input.Model
		}
	}
	if isolated && r.Worktrees == nil {
		return childRun{}, fmt.Errorf("worktree isolation is not configured")
	}
	if isolated {
		name := fmt.Sprintf("tmp-%d", time.Now().UnixNano())
		created, err := r.Worktrees.Create(context.Background(), name)
		if err != nil {
			return childRun{}, err
		}
		wt = &created
		workspace, err := tools.NewWorkspace(created.Path)
		if err != nil {
			return childRun{}, err
		}
		registry, err = tools.NewDefaultRegistryForWorkspace(workspace)
		if err != nil {
			return childRun{}, err
		}
		if err := registry.Register(tools.NewAgentTool()); err != nil {
			return childRun{}, err
		}
		if options.Permissions != nil {
			sandbox, err := permissions.NewSandbox(created.Path)
			if err != nil {
				return childRun{}, err
			}
			options.Permissions.Sandbox = sandbox
			paths, err := permissions.DefaultFilePaths(created.Path)
			if err == nil {
				options.Permissions.Paths = paths
			}
		}
		options.Workspace = created.Path
		prompt = "<worktree_isolation> You are in an isolated Git Worktree. Re-read all local files from this Worktree. Convert any absolute path under the main workspace to a relative path before using tools. </worktree_isolation>\n\n" + prompt
		if !fork {
			names, err := subagent.FilterTools(registry.Names(), definition, background)
			if err != nil {
				return childRun{}, err
			}
			registry, err = registry.Subset(names, nil)
			if err != nil {
				return childRun{}, err
			}
		}
	}
	child := NewRunner(parent.provider, session, registry, parent.executor, options)
	return childRun{runner: child, request: Request{Mode: ModeAct, Prompt: prompt}, fork: fork, isolated: isolated, worktree: wt, manager: r.Worktrees}, nil
}

func clonePermissions(parent *permissions.Engine) *permissions.Engine {
	if parent == nil {
		return nil
	}
	clone := &permissions.Engine{Mode: parent.Mode, Sandbox: parent.Sandbox, Paths: parent.Paths}
	if parent.Rules != nil {
		clone.Rules = permissions.NewRuleStore(parent.Rules.Snapshot())
	}
	return clone
}

func cloneContextConfig(config contextmanager.Config) contextmanager.Config { return config }

func foregroundSubAgentContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(context.WithoutCancel(parent))
}

func isForkType(subagentType string) bool {
	normalized := strings.TrimSpace(subagentType)
	return normalized == "" || strings.EqualFold(normalized, "fork")
}

func childMode(fork bool) subagent.CreationMode {
	if fork {
		return subagent.CreationFork
	}
	return subagent.CreationDefinition
}

func taskName(input tools.AgentInput) string {
	if input.Name != "" {
		return input.Name
	}
	return input.Description
}

func logSubAgentTerminal(logger *logging.Logger, mode subagent.CreationMode, model string, task subagent.TaskInfo) {
	logger.Info("subagent finished", logging.Fields{
		"stage":                 "subagent",
		"status":                string(task.Status),
		"mode":                  string(mode),
		"background":            task.Background,
		"model":                 model,
		"tool_calls":            task.ToolCalls,
		"input_tokens":          task.Usage.InputTokens,
		"output_tokens":         task.Usage.OutputTokens,
		"cache_read_tokens":     task.Usage.CacheReadInputTokens,
		"cache_creation_tokens": task.Usage.CacheCreationInputTokens,
		"duration_ms":           task.EndedAt.Sub(task.StartedAt).Milliseconds(),
	})
}

func asyncResult(info subagent.TaskInfo) tools.Result {
	return tools.Success("agent", map[string]any{"status": "async_launched", "task_id": info.ID, "name": info.Name})
}

func finishedResult(info subagent.TaskInfo) tools.Result {
	if info.Status == subagent.TaskCompleted {
		return tools.Success("agent", map[string]any{"status": string(info.Status), "result": info.Result, "task_id": info.ID})
	}
	return tools.Failure("agent", tools.ErrorExecution, "subagent did not complete", map[string]any{"status": string(info.Status), "task_id": info.ID, "reason": info.Failure})
}

func completeForkHistory(history []provider.Message) []provider.Message {
	pending := make(map[string]provider.ToolCall)
	for _, message := range history {
		for _, block := range message.Blocks {
			if block.ToolCall != nil && block.ToolCall.ID != "" {
				pending[block.ToolCall.ID] = *block.ToolCall
			}
			if block.ToolResult != nil {
				delete(pending, block.ToolResult.CallID)
			}
		}
	}
	if len(pending) == 0 {
		return history
	}
	results := make([]provider.ContentBlock, 0, len(pending))
	for _, call := range pending {
		results = append(results, provider.ContentBlock{Type: provider.BlockToolResult, ToolResult: &provider.ToolResult{CallID: call.ID, Name: call.Name, Content: "fork placeholder for unfinished tool call", IsError: true}})
	}
	history = append(history, provider.Message{Role: provider.RoleUser, Blocks: results})
	return history
}

func safeFailure(err error) string {
	if err == nil {
		return "subagent failed"
	}
	return "subagent failed"
}
