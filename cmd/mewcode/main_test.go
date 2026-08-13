package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/config"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tui"
)

type stubProvider struct{}

func (stubProvider) Stream(_ context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	return nil, nil
}

func TestRunConfigOverride(t *testing.T) {
	origDefault, origLoad, origNew, origTUI := defaultConfigPath, loadConfig, newProvider, runTUI
	defer func() { defaultConfigPath, loadConfig, newProvider, runTUI = origDefault, origLoad, origNew, origTUI }()
	defaultConfigPath = func() (string, error) { return "default", nil }
	var loaded string
	loadConfig = func(path string) (config.Config, error) {
		loaded = path
		return config.Config{Protocol: config.ProtocolOpenAI, Model: "x", BaseURL: "http://localhost", APIKey: "fake", MaxTokens: 1}, nil
	}
	newProvider = func(config.Config, *http.Client, *logging.Logger) (provider.Provider, error) {
		return stubProvider{}, nil
	}
	runTUI = func(*agent.Runner, *conversation.Session, *tui.PermissionBridge) error { return nil }
	if code := run([]string{"--config", "custom.yaml"}, &bytes.Buffer{}); code != 0 || loaded != "custom.yaml" {
		t.Fatalf("code=%d loaded=%s", code, loaded)
	}
}

func TestRunCreatesChapterNineSessionAndInjectsMemoryModules(t *testing.T) {
	origLoad, origNew, origTUI, origPaths, origUserDir := loadConfig, newProvider, runTUI, permissionFilePaths, userConfigDir
	defer func() {
		loadConfig, newProvider, runTUI, permissionFilePaths, userConfigDir = origLoad, origNew, origTUI, origPaths, origUserDir
	}()
	root := t.TempDir()
	userRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mewcode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mewcode", "MEWCODE.md"), []byte("project rule"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(userRoot, "mewcode", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userRoot, "mewcode", "memory", "MEMORY.md"), []byte("user memory"), 0o600); err != nil {
		t.Fatal(err)
	}
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWD)
	userConfigDir = func() (string, error) { return userRoot, nil }
	loadConfig = func(string) (config.Config, error) { return validTestConfig(), nil }
	captured := &captureProvider{}
	newProvider = func(config.Config, *http.Client, *logging.Logger) (provider.Provider, error) { return captured, nil }
	permissionFilePaths = func(string) (permissions.FilePaths, error) { return permissions.FilePaths{}, nil }
	runTUI = func(runner *agent.Runner, _ *conversation.Session, _ *tui.PermissionBridge) error {
		task, err := runner.Start(context.Background(), agent.Request{Mode: agent.ModeAct, Prompt: "hello"})
		if err != nil {
			return err
		}
		for range task.Events {
		}
		return nil
	}
	if code := run([]string{"--config", "custom.yaml"}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("run code=%d", code)
	}
	if len(captured.requests) == 0 || !strings.Contains(captured.requests[0].Prompt.StableSystem, "project rule") || !strings.Contains(captured.requests[0].Prompt.StableSystem, "user memory") {
		t.Fatalf("requests=%+v", captured.requests)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".mewcode", "sessions"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("sessions=%v err=%v", entries, err)
	}
}

type captureProvider struct{ requests []provider.ChatRequest }

func (p *captureProvider) Stream(_ context.Context, request provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	p.requests = append(p.requests, request)
	events := make(chan provider.StreamEvent, 3)
	done := make(chan error, 1)
	events <- provider.StreamEvent{Type: provider.EventStarted}
	events <- provider.StreamEvent{Type: provider.EventTextDelta, Delta: "done"}
	events <- provider.StreamEvent{Type: provider.EventCompleted}
	close(events)
	done <- nil
	close(done)
	return events, done
}

func TestAgentContextConfig(t *testing.T) {
	got := agentContextConfig(config.ContextConfig{
		WindowTokens: 300000, SummaryOutputTokens: 25000, AutoSafetyTokens: 11000,
		ManualSafetyTokens: 4000, SingleResultChars: 60000, MessageResultChars: 250000,
		PreviewChars: 3000, RecentTokens: 12000, RecentMessageMinimum: 6,
	})
	if got.WindowTokens != 300000 || got.SummaryOutputTokens != 25000 || got.AutoSafetyTokens != 11000 || got.ManualSafetyTokens != 4000 || got.SingleResultChars != 60000 || got.MessageResultChars != 250000 || got.PreviewChars != 3000 || got.RecentTokens != 12000 || got.RecentMessageMinimum != 6 {
		t.Fatalf("context=%+v", got)
	}
}

func TestRunPermissionRulesMissingAllowed(t *testing.T) {
	origLoad, origNew, origTUI, origPaths := loadConfig, newProvider, runTUI, permissionFilePaths
	defer func() { loadConfig, newProvider, runTUI, permissionFilePaths = origLoad, origNew, origTUI, origPaths }()
	dir := t.TempDir()
	permissionFilePaths = func(string) (permissions.FilePaths, error) {
		return permissions.FilePaths{
			User:    filepath.Join(dir, "missing-user.yaml"),
			Project: filepath.Join(dir, "missing-project.yaml"),
			Local:   filepath.Join(dir, "missing-local.yaml"),
		}, nil
	}
	loadConfig = func(string) (config.Config, error) { return validTestConfig(), nil }
	newProvider = func(config.Config, *http.Client, *logging.Logger) (provider.Provider, error) {
		return stubProvider{}, nil
	}
	called := false
	runTUI = func(*agent.Runner, *conversation.Session, *tui.PermissionBridge) error {
		called = true
		return nil
	}
	if code := run([]string{"--config", "custom.yaml"}, &bytes.Buffer{}); code != 0 || !called {
		t.Fatalf("code=%d called=%v", code, called)
	}
}

func TestRunIgnoresProjectMCPConfig(t *testing.T) {
	origLoad, origNew, origTUI, origPaths := loadConfig, newProvider, runTUI, permissionFilePaths
	defer func() { loadConfig, newProvider, runTUI, permissionFilePaths = origLoad, origNew, origTUI, origPaths }()
	root := t.TempDir()
	projectConfig := filepath.Join(root, ".mewcode", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projectConfig), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectConfig, []byte("protocol: openai\nmodel: project\nbase_url: http://localhost\napi_key: fake\n"), 0600); err != nil {
		t.Fatal(err)
	}
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWD)
	permissionFilePaths = func(string) (permissions.FilePaths, error) { return permissions.FilePaths{}, nil }
	loadConfig = func(string) (config.Config, error) { return validTestConfig(), nil }
	newProvider = func(config.Config, *http.Client, *logging.Logger) (provider.Provider, error) {
		return stubProvider{}, nil
	}
	called := false
	runTUI = func(*agent.Runner, *conversation.Session, *tui.PermissionBridge) error { called = true; return nil }

	if code := run([]string{"--config", "main.yaml"}, &bytes.Buffer{}); code != 0 || !called {
		t.Fatalf("code=%d called=%v", code, called)
	}
}

func TestRunPermissionInvalidRuleFails(t *testing.T) {
	origLoad, origNew, origTUI, origPaths := loadConfig, newProvider, runTUI, permissionFilePaths
	defer func() { loadConfig, newProvider, runTUI, permissionFilePaths = origLoad, origNew, origTUI, origPaths }()
	dir := t.TempDir()
	bad := filepath.Join(dir, "permissions.yaml")
	if err := os.WriteFile(bad, []byte("rules:\n  run_command: allow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	permissionFilePaths = func(string) (permissions.FilePaths, error) {
		return permissions.FilePaths{Project: bad}, nil
	}
	loadConfig = func(string) (config.Config, error) { return validTestConfig(), nil }
	newProvider = func(config.Config, *http.Client, *logging.Logger) (provider.Provider, error) {
		return stubProvider{}, nil
	}
	runTUI = func(*agent.Runner, *conversation.Session, *tui.PermissionBridge) error { return nil }
	var stderr bytes.Buffer
	if code := run([]string{"--config", "x"}, &stderr); code == 0 || !strings.Contains(stderr.String(), "invalid rule") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunSafeFailure(t *testing.T) {
	orig := loadConfig
	defer func() { loadConfig = orig }()
	loadConfig = func(string) (config.Config, error) {
		return config.Config{}, errors.New("config: invalid field api_key")
	}
	var stderr bytes.Buffer
	if code := run([]string{"--config", "x"}, &stderr); code == 0 || stderr.String() == "" {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunContinuesWhenLoggingInitializationFails(t *testing.T) {
	origLoad, origNew, origTUI, origPaths, origLogger := loadConfig, newProvider, runTUI, permissionFilePaths, newLogger
	defer func() {
		loadConfig, newProvider, runTUI, permissionFilePaths, newLogger = origLoad, origNew, origTUI, origPaths, origLogger
	}()
	root := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWD)
	loadConfig = func(string) (config.Config, error) { return validTestConfig(), nil }
	newProvider = func(config.Config, *http.Client, *logging.Logger) (provider.Provider, error) {
		return stubProvider{}, nil
	}
	permissionFilePaths = func(string) (permissions.FilePaths, error) { return permissions.FilePaths{}, nil }
	newLogger = func(string) (*logging.Logger, error) { return logging.Nop(), errors.New("test logger failure") }
	called := false
	runTUI = func(*agent.Runner, *conversation.Session, *tui.PermissionBridge) error { called = true; return nil }
	var stderr bytes.Buffer
	if code := run([]string{"--config", "x"}, &stderr); code != 0 || !called {
		t.Fatalf("code=%d called=%v stderr=%q", code, called, stderr.String())
	}
	if !strings.Contains(stderr.String(), "logging:") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRunInjectsLoggerIntoRunner(t *testing.T) {
	origLoad, origNew, origTUI, origPaths, origLogger := loadConfig, newProvider, runTUI, permissionFilePaths, newLogger
	defer func() {
		loadConfig, newProvider, runTUI, permissionFilePaths, newLogger = origLoad, origNew, origTUI, origPaths, origLogger
	}()
	root := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWD)
	logger, err := logging.New(root, func() time.Time { return time.Unix(1, 0).UTC() }, 123)
	if err != nil {
		t.Fatal(err)
	}
	loadConfig = func(string) (config.Config, error) { return validTestConfig(), nil }
	newProvider = func(config.Config, *http.Client, *logging.Logger) (provider.Provider, error) {
		return compactProvider{}, nil
	}
	permissionFilePaths = func(string) (permissions.FilePaths, error) { return permissions.FilePaths{}, nil }
	newLogger = func(string) (*logging.Logger, error) { return logger, nil }
	runTUI = func(runner *agent.Runner, _ *conversation.Session, _ *tui.PermissionBridge) error {
		task, err := runner.Start(context.Background(), agent.Request{Mode: agent.ModeCompact})
		if err != nil {
			return err
		}
		for range task.Events {
		}
		return nil
	}
	var stderr bytes.Buffer
	if code := run([]string{"--config", "x"}, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	matches, err := filepath.Glob(filepath.Join(root, "logs", "*", "*", "*", "*.jsonl"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("log files=%v err=%v", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "context compaction completed") {
		t.Fatalf("runner logs missing context compaction: %s", data)
	}
}

type compactProvider struct{}

func (compactProvider) Stream(_ context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	events := make(chan provider.StreamEvent, 4)
	done := make(chan error, 1)
	events <- provider.StreamEvent{Type: provider.EventStarted}
	events <- provider.StreamEvent{Type: provider.EventTextDelta, Delta: "<summary>state</summary>"}
	events <- provider.StreamEvent{Type: provider.EventCompleted}
	close(events)
	done <- nil
	close(done)
	return events, done
}

func validTestConfig() config.Config {
	return config.Config{
		Protocol:    config.ProtocolOpenAI,
		Model:       "x",
		BaseURL:     "http://localhost",
		APIKey:      "fake",
		MaxTokens:   1,
		Permissions: config.PermissionConfig{Mode: config.PermissionModeDefault},
	}
}
