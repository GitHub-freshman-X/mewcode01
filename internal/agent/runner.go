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
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/memory"
	"github.com/GitHub-freshman-X/mewcode01/internal/prompt"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type Runner struct {
	provider provider.Provider
	session  *conversation.Session
	registry *tools.Registry
	executor *tools.Executor
	options  Options
	context  *contextmanager.Manager

	mu     sync.Mutex
	active bool
}

func NewRunner(p provider.Provider, session *conversation.Session, registry *tools.Registry, executor *tools.Executor, options Options) *Runner {
	opts := options.normalized()
	var store *contextmanager.ResultStore
	if opts.Workspace != "" && opts.SessionID != "" && session != nil {
		store, _ = contextmanager.NewResultStore(filepath.Join(opts.Workspace, ".mewcode", "context"), opts.SessionID)
	}
	return &Runner{provider: p, session: session, registry: registry, executor: executor, options: opts, context: contextmanager.NewManager(opts.Context, store)}
}

func (r *Runner) Start(ctx context.Context, req Request) (*Task, error) {
	if r == nil || r.provider == nil || r.executor == nil {
		return nil, errors.New("agent runner is not configured")
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
	terminal := func(event Event) { events <- event }
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
		if !emit(Event{Type: EventProgress, Iteration: iterations, Phase: PhaseCallingModel}) {
			terminal(cancelledEvent(iterations-1, total, hasPartial))
			return
		}
		messages := modelSnapshot()
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
		environment, err := prompt.CollectEnvironment(promptMode, prepared.registry, r.options.Workspace, r.options.Clock)
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
			OptionalModules: r.options.OptionalModules,
		})
		if err != nil {
			summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
			terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			return
		}
		definitions := prompt.EnhanceDefinitions(prepared.registry.Definitions(), promptMode)
		var round roundResult
		for {
			roundCtx, cancelRound := context.WithCancel(ctx)
			stream, done := r.provider.Stream(roundCtx, provider.ChatRequest{
				Prompt:   bundle,
				Messages: messages, MaxTokens: r.options.MaxTokens, Thinking: r.options.Thinking,
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
		manager.RecordUsage(round.Usage, messages)
		if len(round.ToolCalls) == 0 {
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
		scheduler := NewScheduler(prepared.registry, r.executor, r.options.Permissions, r.options.Confirmer)
		results, err := scheduler.Execute(ctx, round.ToolCalls, func(event Event) bool {
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
		unknownLimit := false
		for _, call := range round.ToolCalls {
			if _, ok := prepared.registry.Get(call.Name); ok {
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
