package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

const (
	exitCompleted = 0
	exitFailed    = 1
	exitCancelled = 2
	exitTimedOut  = 3
	exitStopped   = 4
)

type runOptions struct {
	configPath string
	prompt     string
	timeout    time.Duration
	jsonOutput bool
}

type runResult struct {
	Status     string         `json:"status"`
	StopReason string         `json:"stop_reason"`
	Error      string         `json:"error"`
	FinalText  string         `json:"final_text"`
	ElapsedMS  int64          `json:"elapsed_ms"`
	Iterations int            `json:"iterations"`
	Usage      provider.Usage `json:"usage"`
}

func runNonInteractive(args []string, stdout, stderr io.Writer) int {
	options, err := parseRunOptions(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "mewcode run:", err)
		return exitFailed
	}
	return launch(options.configPath, &launchOptions{nonInteractive: &options}, stdout, stderr)
}

func parseRunOptions(args []string, stderr io.Writer) (runOptions, error) {
	flags := flag.NewFlagSet("mewcode run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "path to YAML configuration")
	prompt := flags.String("prompt", "", "task prompt")
	promptFile := flags.String("prompt-file", "", "path to task prompt file")
	timeout := flags.Duration("timeout", 30*time.Minute, "maximum task duration (0 disables)")
	jsonOutput := flags.Bool("json", false, "write a JSON result to stdout")
	if err := flags.Parse(args); err != nil {
		return runOptions{}, err
	}
	if flags.NArg() != 0 {
		return runOptions{}, errors.New("unexpected positional arguments")
	}
	if *timeout < 0 {
		return runOptions{}, errors.New("timeout must not be negative")
	}
	hasPrompt, hasPromptFile := false, false
	flags.Visit(func(current *flag.Flag) {
		switch current.Name {
		case "prompt":
			hasPrompt = true
		case "prompt-file":
			hasPromptFile = true
		}
	})
	if hasPrompt == hasPromptFile {
		return runOptions{}, errors.New("exactly one of --prompt or --prompt-file is required")
	}
	text := *prompt
	if *promptFile != "" {
		content, err := os.ReadFile(*promptFile)
		if err != nil {
			return runOptions{}, fmt.Errorf("read prompt file: %w", err)
		}
		text = string(content)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return runOptions{}, errors.New("task prompt must not be empty")
	}
	return runOptions{configPath: *configPath, prompt: text, timeout: *timeout, jsonOutput: *jsonOutput}, nil
}

func executeNonInteractive(runner *agent.Runner, options runOptions, stdout, stderr io.Writer) int {
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignals()
	var cancel context.CancelFunc = func() {}
	if options.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, options.timeout)
	}
	defer cancel()

	started := time.Now()
	task, err := runner.Start(ctx, agent.Request{Mode: agent.ModeAct, Prompt: options.prompt})
	if err != nil {
		return writeRunResult(stdout, stderr, options.jsonOutput, runResult{Status: "failed", Error: "agent task failed", ElapsedMS: time.Since(started).Milliseconds()}, exitFailed)
	}

	var text strings.Builder
	var terminal agent.Event
	for event := range task.Events {
		if event.Type == agent.EventTextDelta && event.Text != "" {
			text.WriteString(event.Text)
			if !options.jsonOutput {
				_, _ = io.WriteString(stdout, event.Text)
			}
		}
		switch event.Type {
		case agent.EventCompleted, agent.EventStopped, agent.EventCancelled, agent.EventFailed:
			terminal = event
		}
	}

	result := runResult{FinalText: text.String(), ElapsedMS: time.Since(started).Milliseconds()}
	if terminal.Summary != nil {
		result.StopReason = string(terminal.Summary.Reason)
		result.Iterations = terminal.Summary.Iterations
		result.Usage = terminal.Summary.Usage
	}
	code := exitFailed
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.Status, result.Error, code = "timed_out", "agent task timed out", exitTimedOut
	case errors.Is(ctx.Err(), context.Canceled) || terminal.Type == agent.EventCancelled:
		result.Status, result.Error, code = "cancelled", "agent task cancelled", exitCancelled
	case terminal.Type == agent.EventCompleted:
		result.Status, code = "completed", exitCompleted
	case terminal.Type == agent.EventStopped:
		result.Status, result.Error, code = "stopped", "agent task stopped safely", exitStopped
	default:
		result.Status, result.Error, code = "failed", "agent task failed", exitFailed
	}
	return writeRunResult(stdout, stderr, options.jsonOutput, result, code)
}

func writeRunResult(stdout, stderr io.Writer, jsonOutput bool, result runResult, code int) int {
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintln(stderr, "write result:", err)
			return exitFailed
		}
	}
	if code != exitCompleted {
		fmt.Fprintln(stderr, result.Error)
	}
	return code
}
