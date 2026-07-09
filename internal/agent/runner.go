package agent

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
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

	mu     sync.Mutex
	active bool
}

func NewRunner(p provider.Provider, session *conversation.Session, registry *tools.Registry, executor *tools.Executor, options Options) *Runner {
	return &Runner{provider: p, session: session, registry: registry, executor: executor, options: options.normalized()}
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
	if mode == ModePlan {
		planHistory = newTaskHistory(r.session.Snapshot())
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

	for iterations < r.options.MaxIterations {
		iterations++
		if !emit(Event{Type: EventProgress, Iteration: iterations, Phase: PhaseCallingModel}) {
			terminal(cancelledEvent(iterations-1, total, hasPartial))
			return
		}
		messages := modelSnapshot()
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
		})
		if err != nil {
			summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
			terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			return
		}
		definitions := prompt.EnhanceDefinitions(prepared.registry.Definitions(), promptMode)
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
		round, err := collectRound(roundCtx, iterations, stream, done, func(event Event) bool {
			if event.Type == EventTextDelta && event.Text != "" {
				hasPartial = true
			}
			return emit(event)
		})
		cancelRound()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				terminal(cancelledEvent(iterations-1, total, hasPartial))
			} else {
				summary := &Summary{Reason: StopStreamError, Iterations: iterations - 1, Usage: total, Partial: hasPartial}
				terminal(Event{Type: EventFailed, Iteration: iterations, Phase: PhaseFinishing, Summary: summary, Err: err})
			}
			return
		}
		total.Add(round.Usage)
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
			return
		}

		if !emit(Event{Type: EventProgress, Iteration: iterations, Phase: PhaseRunningTools}) {
			terminal(cancelledEvent(iterations-1, total, hasPartial))
			return
		}
		scheduler := NewScheduler(prepared.registry, r.executor)
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
