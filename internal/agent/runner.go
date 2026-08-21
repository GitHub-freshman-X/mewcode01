package agent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	contextmanager "github.com/GitHub-freshman-X/mewcode01/internal/context"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/hooks"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/memory"
	"github.com/GitHub-freshman-X/mewcode01/internal/prompt"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/skills"
	"github.com/GitHub-freshman-X/mewcode01/internal/subagent"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type Runner struct {
	provider      provider.Provider
	session       *conversation.Session
	registry      *tools.Registry
	executor      *tools.Executor
	options       Options
	context       *contextmanager.Manager
	hooks         *hookNotifications
	notifications *taskNotifications

	mu     sync.Mutex
	active bool

	promptMu     sync.RWMutex
	latestPrompt string
}

func NewRunner(p provider.Provider, session *conversation.Session, registry *tools.Registry, executor *tools.Executor, options Options) *Runner {
	opts := options.normalized()
	if opts.Skills != nil && registry != nil {
		if copy, err := registry.Subset(nil, nil); err == nil {
			if err := copy.Register(skills.NewLoadTool(opts.Skills)); err == nil {
				registry = copy
			}
		}
	}
	var store *contextmanager.ResultStore
	if opts.Workspace != "" && opts.SessionID != "" && session != nil {
		store, _ = contextmanager.NewResultStore(filepath.Join(opts.Workspace, ".mewcode", "context"), opts.SessionID)
	}
	notifications := &hookNotifications{}
	taskQueue := &taskNotifications{}
	if opts.Hooks != nil {
		opts.Hooks.SetPromptSink(notifications)
	}
	runner := &Runner{provider: p, session: session, registry: registry, executor: executor, options: opts, context: contextmanager.NewManager(opts.Context, store), hooks: notifications, notifications: taskQueue}
	if opts.SubAgents != nil && opts.SubAgents.Tasks != nil {
		updates, _ := opts.SubAgents.Tasks.Subscribe()
		go func() {
			for update := range updates {
				if update.Task.Status == subagent.TaskCompleted || update.Task.Status == subagent.TaskFailed || update.Task.Status == subagent.TaskCancelled {
					runner.notifications.Add(update.Task)
				}
			}
		}()
	}
	return runner
}

func (r *Runner) Start(ctx context.Context, req Request) (*Task, error) {
	if r == nil || r.provider == nil || r.executor == nil {
		return nil, errors.New("agent runner is not configured")
	}
	if req.Skill != nil {
		if r.options.Skills == nil {
			return nil, errors.New("skill manager is not configured")
		}
		skill, ok := r.options.Skills.Skill(req.Skill.Name)
		if !ok {
			return nil, fmt.Errorf("unknown skill %q", req.Skill.Name)
		}
		if skill.Mode == skills.ModeFork {
			return r.startFork(ctx, req)
		}
		if _, err := r.options.Skills.Activate(req.Skill.Name, req.Skill.Args); err != nil {
			return nil, err
		}
	}
	prepared, err := prepareRequest(req, r.session, r.registry)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.active {
		r.mu.Unlock()
		return nil, errors.New("an agent task is already active")
	}
	r.active = true
	r.mu.Unlock()
	taskCtx, cancel := context.WithCancel(ctx)
	events := make(chan Event, 64)
	go r.run(taskCtx, req.Mode, prepared, events)
	return &Task{Events: events, Cancel: cancel}, nil
}

func (r *Runner) startFork(ctx context.Context, req Request) (*Task, error) {
	snapshot, err := r.options.Skills.SnapshotWithActivation(req.Skill.Name, req.Skill.Args)
	if err != nil {
		return nil, err
	}
	skill := snapshot.Catalog.Skills[req.Skill.Name]
	forkSession := conversation.NewSession()
	switch skill.Context {
	case skills.ContextFull:
		forkSession.ReplaceHistory(r.session.Snapshot())
	case skills.ContextRecent:
		history := r.session.Snapshot()
		if len(history) > 5 {
			history = history[len(history)-5:]
		}
		forkSession.ReplaceHistory(history)
	case skills.ContextNone:
	default:
		return nil, fmt.Errorf("invalid fork context %q", skill.Context)
	}

	names := r.registry.Names()
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if name != skills.LoadToolName {
			filtered = append(filtered, name)
		}
	}
	registry, err := r.registry.Subset(filtered, nil)
	if err != nil {
		return nil, err
	}
	options := r.options
	options.Skills = skills.NewManagerFromSnapshot(snapshot)
	options.SessionStore = nil
	options.SessionID = ""
	options.Memory = nil
	forkRunner := NewRunner(r.provider, forkSession, registry, r.executor, options)

	r.mu.Lock()
	if r.active {
		r.mu.Unlock()
		return nil, errors.New("an agent task is already active")
	}
	r.active = true
	r.mu.Unlock()
	taskCtx, cancel := context.WithCancel(ctx)
	events := make(chan Event, 64)
	go func() {
		defer close(events)
		defer func() { r.mu.Lock(); r.active = false; r.mu.Unlock() }()
		inner, err := forkRunner.Start(taskCtx, Request{Mode: req.Mode, Prompt: req.Prompt})
		if err != nil {
			events <- Event{Type: EventFailed, Phase: PhaseFinishing, Err: err}
			return
		}
		var terminal Event
		for event := range inner.Events {
			if isTerminal(event.Type) {
				terminal = event
			}
		}
		if terminal.Summary != nil {
			if err := r.session.RecordUsage(terminal.Summary.Usage); err != nil {
				events <- Event{Type: EventFailed, Phase: PhaseFinishing, Summary: terminal.Summary, Err: err}
				return
			}
		}
		if terminal.Type == EventCompleted {
			summary := forkSummary(forkSession.DisplaySnapshot())
			if err := r.session.CommitRound(&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: skillInvocationLabel(req.Skill)}}}, provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: summary}}}, nil); err != nil {
				events <- Event{Type: EventFailed, Phase: PhaseFinishing, Summary: terminal.Summary, Err: err}
				return
			}
			terminal.Text = summary
		}
		events <- terminal
	}()
	return &Task{Events: events, Cancel: cancel}, nil
}

func skillInvocationLabel(invocation *SkillInvocation) string {
	if invocation == nil {
		return "/skill"
	}
	if strings.TrimSpace(invocation.Args) == "" {
		return "/" + invocation.Name
	}
	return "/" + invocation.Name + " " + strings.TrimSpace(invocation.Args)
}

func forkSummary(messages []provider.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == provider.RoleAssistant {
			if text := messageText(messages[i]); text != "" {
				return text
			}
		}
	}
	return "Skill 执行完成，但未生成可回流的摘要。"
}

// ReplaceSession atomically moves an idle runner to a persisted session.
func (r *Runner) ReplaceSession(session *conversation.Session, sessionID string) error {
	if r == nil || session == nil || strings.TrimSpace(sessionID) == "" {
		return errors.New("session and session ID are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active {
		return errors.New("cannot switch session while an agent task is active")
	}
	var store *contextmanager.ResultStore
	if r.options.Workspace != "" {
		var err error
		store, err = contextmanager.NewResultStore(filepath.Join(r.options.Workspace, ".mewcode", "context"), sessionID)
		if err != nil {
			return err
		}
	}
	r.session = session
	r.options.SessionID = sessionID
	r.context = contextmanager.NewManager(r.options.Context, store)
	return nil
}

func (r *Runner) SessionStore() *conversation.SessionStore {
	if r == nil {
		return nil
	}
	return r.options.SessionStore
}
func (r *Runner) SessionID() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.options.SessionID
}
func (r *Runner) MemoryService() *memory.Service {
	if r == nil {
		return nil
	}
	return r.options.Memory
}

func (r *Runner) SkillDirectory() []skills.Metadata {
	if r == nil || r.options.Skills == nil {
		return nil
	}
	return r.options.Skills.Directory()
}

func (r *Runner) Hooks() *hooks.Engine {
	if r == nil {
		return nil
	}
	return r.options.Hooks
}

func (r *Runner) ReloadSkills() error {
	if r == nil || r.options.Skills == nil {
		return errors.New("skill manager is not configured")
	}
	return r.options.Skills.Reload()
}

func (r *Runner) ClearSkillActivations() {
	if r != nil && r.options.Skills != nil {
		r.options.Skills.ClearActivations()
	}
}

func (r *Runner) rememberSystemPrompt(value string) {
	if r == nil || value == "" {
		return
	}
	r.promptMu.Lock()
	r.latestPrompt = value
	r.promptMu.Unlock()
}

func (r *Runner) lastSystemPrompt() string {
	if r == nil {
		return ""
	}
	r.promptMu.RLock()
	defer r.promptMu.RUnlock()
	return r.latestPrompt
}

func (r *Runner) SubscribeSubAgentTasks() <-chan subagent.TaskNotification {
	if r == nil || r.options.SubAgents == nil || r.options.SubAgents.Tasks == nil {
		return nil
	}
	updates, _ := r.options.SubAgents.Tasks.Subscribe()
	return updates
}

func (r *Runner) HasForegroundSubAgent() bool {
	return r != nil && r.options.SubAgents != nil && r.options.SubAgents.HasForeground(r)
}

func (r *Runner) BackgroundForegroundSubAgent() bool {
	if r == nil || r.options.SubAgents == nil {
		return false
	}
	return r.options.SubAgents.BackgroundForeground(r)
}

func (r *Runner) run(ctx context.Context, mode Mode, prepared preparedRequest, events chan<- Event) {
	defer close(events)
	defer func() { r.mu.Lock(); r.active = false; r.mu.Unlock() }()
	emit := func(event Event) bool {
		select {
		case events <- event:
			return true
		case <-ctx.Done():
			return false
		}
	}
	terminal := func(event Event) {
		if event.Type == EventFailed && event.Err != nil {
			r.runHooks(ctx, hooks.EventError, hooks.Context{Error: event.Err.Error()})
		}
		events <- event
	}
	r.runHooks(ctx, hooks.EventSessionStart, hooks.Context{Message: prepared.prompt})
	defer r.runHooks(context.Background(), hooks.EventSessionEnd, hooks.Context{})
	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: prepared.prompt}}}
	var planHistory *taskHistory
	manager := r.context
	if mode == ModePlan {
		planHistory = newTaskHistory(r.session.Snapshot())
		manager = contextmanager.NewManager(r.options.Context, r.context.Store)
	}
	modelSnapshot := func() []provider.Message {
		if planHistory != nil {
			return planHistory.Snapshot()
		}
		return r.session.Snapshot()
	}
	commitRound := func(user *provider.Message, assistant provider.Message, results []provider.ToolResult) error {
		if planHistory != nil {
			return planHistory.CommitRound(user, assistant, results)
		}
		return r.session.CommitRound(user, assistant, results)
	}
	var total provider.Usage
	unknownCount := 0
	hasPartial := false
	iterations := 0
	emergencyRetried := false
	replaceHistory := func(messages []provider.Message) {
		if planHistory != nil {
			planHistory.Replace(messages)
			return
		}
		r.session.ReplaceHistory(messages)
	}

	if mode == ModeCompact {
		usage, err := r.compact(ctx, manager, contextmanager.TriggerManual, 1, modelSnapshot, replaceHistory, emit)
		total.Add(usage)
		if err != nil {
			summary := &Summary{Reason: StopStreamError, Iterations: 0, Usage: total}
			terminal(Event{Type: EventFailed, Iteration: 1, Phase: PhaseFinishing, Summary: summary, Err: err})
			return
		}
		summary := &Summary{Reason: StopFinalAnswer, Iterations: 1, Usage: total}
		terminal(Event{Type: EventCompleted, Iteration: 1, Phase: PhaseFinishing, Summary: summary})
		return
	}

	for iterations < r.options.MaxIterations {
		iterations++
		r.runHooks(ctx, hooks.EventTurnStart, hooks.Context{Message: prepared.prompt})
		if !emit(Event{Type: EventProgress, Iteration: iterations, Phase: PhaseCallingModel}) {
			terminal(cancelledEvent(iterations-1, total, hasPartial))
			return
		}
		messages := modelSnapshot()
		messages = append(messages, r.taskMessages()...)
		messages = append(messages, r.hookMessages()...)
		if trigger, ok := manager.Decision(messages, false); ok {
			usage, err := r.compact(ctx, manager, trigger, iterations, modelSnapshot, replaceHistory, emit)
			total.Add(usage)
			if err != nil {
				summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
				terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
				return
			}
			messages = modelSnapshot()
		} else if manager.State.AutomaticFailures >= 3 && manager.Estimate(messages) >= manager.Config.WindowTokens-manager.Config.SummaryOutputTokens-manager.Config.AutoSafetyTokens {
			r.options.Logger.Info("context automatic compaction skipped", logging.Fields{"stage": "context_compaction", "status": "breaker_open", "trigger": string(contextmanager.TriggerAutomatic), "automatic_failures": manager.State.AutomaticFailures})
		}
		var roundUser *provider.Message
		if iterations == 1 {
			messages = append(messages, provider.CloneMessage(user))
			roundUser = &user
		}
		promptMode := toPromptMode(mode)
		visibleRegistry := prepared.registry
		modules := r.options.OptionalModules
		model := r.options.Model
		if r.options.Skills != nil {
			runtime, runtimeErr := skills.RuntimeFor(r.options.Skills.Snapshot())
			if runtimeErr != nil {
				summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
				terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: runtimeErr})
				return
			}
			visibleRegistry, runtimeErr = prepared.registry.Subset(runtime.AllowedTools, []string{skills.LoadToolName})
			if runtimeErr != nil {
				summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
				terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: runtimeErr})
				return
			}
			modules.AvailableSkills = skills.DirectoryPrompt(r.options.Skills.Directory())
			modules.ActiveSkills = append(modules.ActiveSkills, runtime.ActivePrompts...)
			model = runtime.Model
		}
		environment, err := prompt.CollectEnvironment(promptMode, visibleRegistry, r.options.Workspace, r.options.Clock)
		if err != nil {
			summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
			terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			return
		}
		bundle, _, err := prompt.BuildBundle(prompt.BuildContext{
			Environment:     environment,
			Mode:            promptMode,
			Iteration:       iterations,
			InjectionPolicy: r.options.Injection,
			OptionalModules: modules,
		})
		if err != nil {
			summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
			terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			return
		}
		if r.options.SystemPrompt != "" {
			bundle.StableSystem = r.options.SystemPrompt
		}
		r.rememberSystemPrompt(bundle.StableSystem)
		definitions := prompt.EnhanceDefinitions(prepared.registry.Definitions(), promptMode)
		var round roundResult
		for {
			roundCtx, cancelRound := context.WithCancel(ctx)
			r.runHooks(roundCtx, hooks.EventPreSend, hooks.Context{Message: prepared.prompt})
			requestMessages := append([]provider.Message(nil), messages...)
			requestMessages = append(requestMessages, r.hookMessages()...)
			stream, done := r.provider.Stream(roundCtx, provider.ChatRequest{
				Model: model, Prompt: bundle,
				Messages: requestMessages, MaxTokens: r.options.MaxTokens, Thinking: r.options.Thinking,
				Tools: definitions,
			})
			if !emit(Event{Type: EventProgress, Iteration: iterations, Phase: PhaseStreaming}) {
				cancelRound()
				terminal(cancelledEvent(iterations-1, total, hasPartial))
				return
			}
			var err error
			round, err = collectRound(roundCtx, iterations, stream, done, func(event Event) bool {
				if event.Type == EventTextDelta && event.Text != "" {
					hasPartial = true
				}
				return emit(event)
			})
			cancelRound()
			if err == nil {
				break
			}
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				terminal(cancelledEvent(iterations-1, total, hasPartial))
				return
			}
			if isContextTooLongError(err) && !emergencyRetried {
				emergencyRetried = true
				r.options.Logger.Info("context emergency retry started", logging.Fields{"stage": "context_emergency_retry", "status": "started"})
				usage, compactErr := r.compact(ctx, manager, contextmanager.TriggerEmergency, iterations, modelSnapshot, replaceHistory, emit)
				total.Add(usage)
				if compactErr != nil {
					summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
					terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: compactErr})
					return
				}
				messages = modelSnapshot()
				if iterations == 1 {
					messages = append(messages, provider.CloneMessage(user))
				}
				continue
			}
			summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
			terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			return
		}
		total.Add(round.Usage)
		r.runHooks(ctx, hooks.EventPostReceive, hooks.Context{})
		if err := r.session.RecordUsage(round.Usage); err != nil {
			summary := &Summary{Reason: StopStreamError, Iterations: iterations, Usage: total}
			terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			return
		}
		manager.RecordUsage(round.Usage, messages)
		if len(round.ToolCalls) == 0 {
			r.runHooks(ctx, hooks.EventTurnEnd, hooks.Context{})
			text := messageText(round.Assistant)
			if mode == ModePlan && text == "" {
				summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total}
				terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: errors.New("completed plan is empty")})
				return
			}
			if err := commitRound(roundUser, round.Assistant, nil); err != nil {
				summary := &Summary{Reason: StopStreamError, Iterations: iterations, Usage: total}
				terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
				return
			}
			if mode == ModePlan {
				displayUser := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: prepared.displayPrompt}}}
				if err := r.session.CommitPlan(displayUser, round.Assistant, text); err != nil {
					summary := &Summary{Reason: StopStreamError, Iterations: iterations, Usage: total}
					terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
					return
				}
			}
			if mode == ModeDo {
				if err := r.session.ConsumePlans(prepared.plans); err != nil {
					summary := &Summary{Reason: StopStreamError, Iterations: iterations, Usage: total}
					terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
					return
				}
			}
			summary := &Summary{Reason: StopFinalAnswer, Iterations: iterations, Usage: total}
			terminal(Event{Type: EventCompleted, Iteration: iterations, Phase: PhaseFinishing, Summary: summary})
			r.startMemoryTasks(mode)
			return
		}

		if !emit(Event{Type: EventProgress, Iteration: iterations, Phase: PhaseRunningTools}) {
			terminal(cancelledEvent(iterations-1, total, hasPartial))
			return
		}
		scheduler := NewScheduler(visibleRegistry, r.executor, r.options.Permissions, r.options.Confirmer)
		scheduler.Hooks = r.options.Hooks
		scheduleCtx := ctx
		if r.options.SubAgents != nil {
			scheduleCtx = tools.WithSubAgentHost(scheduleCtx, runnerSubAgentHost{runner: r, runtime: r.options.SubAgents})
			if isForkSource(ctx) {
				scheduleCtx = withForkSource(scheduleCtx)
			}
		}
		results, err := scheduler.Execute(scheduleCtx, round.ToolCalls, func(event Event) bool {
			event.Iteration = iterations
			return emit(event)
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				terminal(cancelledEvent(iterations-1, total, hasPartial))
			} else {
				summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
				terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			}
			return
		}
		results, persisted, err := manager.PrepareResults(results)
		if err != nil {
			summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total}
			terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			return
		}
		if len(persisted) > 0 && !emit(Event{Type: EventContextCompaction, Iteration: iterations, Phase: PhaseRunningTools, ContextCompaction: &CompactionEvent{Persisted: persisted}}) {
			terminal(cancelledEvent(iterations-1, total, hasPartial))
			return
		}
		if len(persisted) > 0 {
			r.options.Logger.Info("context tool results persisted", logging.Fields{"stage": "tool_result_persistence", "status": "persisted", "persisted_count": len(persisted), "persisted_bytes": persistedBytes(persisted)})
		}
		if err := commitRound(roundUser, round.Assistant, results); err != nil {
			summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total}
			terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			return
		}
		r.runHooks(ctx, hooks.EventTurnEnd, hooks.Context{})
		unknownLimit := false
		for _, call := range round.ToolCalls {
			if _, ok := visibleRegistry.Get(call.Name); ok {
				unknownCount = 0
			} else {
				unknownCount++
				if unknownCount >= UnknownToolStopThreshold {
					unknownLimit = true
				}
			}
		}
		if unknownLimit {
			summary := &Summary{Reason: StopUnknownToolLimit, Iterations: iterations, Usage: total}
			terminal(Event{Type: EventStopped, Iteration: iterations, Phase: PhaseFinishing, Summary: summary})
			return
		}
		if iterations == r.options.MaxIterations {
			summary := &Summary{Reason: StopIterationLimit, Iterations: iterations, Usage: total}
			terminal(Event{Type: EventStopped, Iteration: iterations, Phase: PhaseFinishing, Summary: summary})
			return
		}
	}
}

type hookNotifications struct {
	mu       sync.Mutex
	messages []string
}

func (n *hookNotifications) AddHookNotification(message string) {
	if n == nil || strings.TrimSpace(message) == "" {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.messages = append(n.messages, message)
}

type taskNotifications struct {
	mu    sync.Mutex
	tasks []subagent.TaskInfo
}

func (n *taskNotifications) Add(task subagent.TaskInfo) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.tasks = append(n.tasks, task)
	n.mu.Unlock()
}

func (r *Runner) taskMessages() []provider.Message {
	if r == nil || r.notifications == nil {
		return nil
	}
	r.notifications.mu.Lock()
	tasks := append([]subagent.TaskInfo(nil), r.notifications.tasks...)
	r.notifications.tasks = nil
	r.notifications.mu.Unlock()
	messages := make([]provider.Message, 0, len(tasks))
	for _, task := range tasks {
		status := string(task.Status)
		result := task.Result
		if result == "" {
			result = task.Failure
		}
		text := fmt.Sprintf("<task-notification>\nid: %s\nname: %s\nstatus: %s\nresult: %s\ntokens_in: %d\ntokens_out: %d\n</task-notification>", task.ID, task.Name, status, result, task.Usage.InputTokens, task.Usage.OutputTokens)
		messages = append(messages, provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: text}}})
	}
	return messages
}

func (r *Runner) hookMessages() []provider.Message {
	if r.hooks == nil {
		return nil
	}
	r.hooks.mu.Lock()
	defer r.hooks.mu.Unlock()
	messages := make([]provider.Message, 0, len(r.hooks.messages))
	for _, notification := range r.hooks.messages {
		messages = append(messages, provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "<hook-notification>\n" + notification + "\n</hook-notification>"}}})
	}
	r.hooks.messages = nil
	return messages
}

func (r *Runner) runHooks(ctx context.Context, event hooks.Event, hookCtx hooks.Context) {
	if r.options.Hooks == nil {
		return
	}
	if r.options.SubAgents != nil {
		ctx = tools.WithSubAgentHost(ctx, runnerSubAgentHost{runner: r, runtime: r.options.SubAgents})
	}
	r.options.Hooks.Run(ctx, event, hookCtx)
}

func (r *Runner) startMemoryTasks(mode Mode) {
	if r.options.Memory == nil || (mode != ModeAct && mode != ModePlan && mode != ModeDo) {
		return
	}
	transcript := r.session.DisplaySnapshot()
	if !r.options.Memory.ShouldExtract(transcript) {
		return
	}
	memoryMode := memory.Mode(mode)
	go func() {
		if err := r.options.Memory.Extract(context.Background(), memoryMode, transcript); err != nil {
			r.options.Logger.Error("memory extraction failed", logging.Fields{"stage": "memory_extract", "status": "failed", "error_type": fmt.Sprintf("%T", err)})
		}
		if err := r.options.Memory.MaybeConsolidate(context.Background()); err != nil {
			r.options.Logger.Error("memory consolidation failed", logging.Fields{"stage": "memory_consolidation", "status": "failed", "error_type": fmt.Sprintf("%T", err)})
		}
	}()
}

func isContextTooLongError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "context length") ||
		strings.Contains(text, "context too long") ||
		strings.Contains(text, "maximum context") ||
		strings.Contains(text, "context window")
}

func (r *Runner) compact(ctx context.Context, manager *contextmanager.Manager, trigger contextmanager.Trigger, iteration int, snapshot func() []provider.Message, replace func([]provider.Message), emit func(Event) bool) (provider.Usage, error) {
	r.runHooks(ctx, hooks.EventCompact, hooks.Context{})
	messages := snapshot()
	before := manager.Estimate(messages)
	r.options.Logger.Info("context compaction started", logging.Fields{"stage": "context_compaction", "status": "started", "trigger": string(trigger), "before_tokens": before, "automatic_failures": manager.State.AutomaticFailures})
	req := manager.BuildSummaryRequest(messages, trigger)
	stream, done := r.provider.Stream(ctx, req)
	round, err := collectRound(ctx, iteration, stream, done, func(Event) bool { return true })
	if err != nil {
		_ = emit(Event{Type: EventContextCompaction, Iteration: iteration, Phase: PhaseCallingModel, ContextCompaction: &CompactionEvent{Trigger: trigger, BeforeTokens: before, Error: err.Error()}})
		if trigger == contextmanager.TriggerAutomatic {
			manager.State.AutomaticFailures++
		}
		r.options.Logger.Error("context compaction failed", logging.Fields{"stage": "context_compaction", "status": "failed", "trigger": string(trigger), "before_tokens": before, "automatic_failures": manager.State.AutomaticFailures})
		return round.Usage, err
	}
	if err := r.session.RecordUsage(round.Usage); err != nil {
		return round.Usage, err
	}
	rawSummary := messageText(round.Assistant)
	extraction, err := contextmanager.ExtractSummary(rawSummary)
	if err != nil {
		_ = emit(Event{Type: EventContextCompaction, Iteration: iteration, Phase: PhaseCallingModel, ContextCompaction: &CompactionEvent{Trigger: trigger, BeforeTokens: before, Error: err.Error()}})
		if trigger == contextmanager.TriggerAutomatic {
			manager.State.AutomaticFailures++
		}
		fields := logging.Fields{"stage": "context_compaction", "status": "summary_invalid", "trigger": string(trigger), "before_tokens": before, "automatic_failures": manager.State.AutomaticFailures, "summary_response_chars": len([]rune(rawSummary))}
		var parseErr *contextmanager.SummaryParseError
		if errors.As(err, &parseErr) {
			fields["parse_reason"] = parseErr.Reason
			fields["summary_open_tags"] = parseErr.OpenTags
			fields["summary_close_tags"] = parseErr.CloseTags
			fields["summary_max_depth"] = parseErr.MaxDepth
			fields["summary_tag_trace"] = parseErr.TagTrace
		}
		fields["summary_response"] = rawSummary
		r.options.Logger.Error("context compaction failed", fields)
		return round.Usage, err
	}
	rebuilt := manager.Rebuild(messages, extraction.Text)
	replace(rebuilt)
	after := manager.Estimate(rebuilt)
	manager.RecordUsage(provider.Usage{InputTokens: after}, rebuilt)
	if trigger == contextmanager.TriggerManual || trigger == contextmanager.TriggerAutomatic {
		manager.State.AutomaticFailures = 0
	}
	// r.options.Logger.Info("context compaction completed", logging.Fields{"stage": "context_compaction", "status": "completed", "trigger": string(trigger), "before_tokens": before, "after_tokens": after, "automatic_failures": manager.State.AutomaticFailures, "summary_candidates": extraction.CandidateCount, "summary_used_last": extraction.UsedLast, "summary_chars": len(extraction.Text), "summary": extraction.Text})
	r.options.Logger.Info("context compaction completed", logging.Fields{"stage": "context_compaction", "status": "completed", "trigger": string(trigger), "before_tokens": before, "after_tokens": after, "automatic_failures": manager.State.AutomaticFailures, "summary_candidates": extraction.CandidateCount, "summary_used_last": extraction.UsedLast, "summary_chars": len(extraction.Text)})
	if !emit(Event{Type: EventContextCompaction, Iteration: iteration, Phase: PhaseCallingModel, ContextCompaction: &CompactionEvent{Trigger: trigger, BeforeTokens: before, AfterTokens: after}}) {
		return round.Usage, context.Canceled
	}
	return round.Usage, nil
}

func persistedBytes(persisted []contextmanager.Persistence) int {
	total := 0
	for _, item := range persisted {
		total += item.Size
	}
	return total
}

func cancelledEvent(iterations int, usage provider.Usage, partial bool) Event {
	return Event{Type: EventCancelled, Iteration: iterations, Phase: PhaseFinishing,
		Summary: &Summary{Reason: StopCancelled, Iterations: iterations, Usage: usage, Partial: partial}}
}

func messageText(message provider.Message) string {
	var parts []string
	for _, block := range message.Blocks {
		if block.Type == provider.BlockText && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}
