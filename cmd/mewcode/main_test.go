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
	newProvider = func(config.Config, *http.Client) (provider.Provider, error) { return stubProvider{}, nil }
	runTUI = func(*agent.Runner, *conversation.Session, *tui.PermissionBridge) error { return nil }
	if code := run([]string{"--config", "custom.yaml"}, &bytes.Buffer{}); code != 0 || loaded != "custom.yaml" {
		t.Fatalf("code=%d loaded=%s", code, loaded)
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
	newProvider = func(config.Config, *http.Client) (provider.Provider, error) { return stubProvider{}, nil }
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
	newProvider = func(config.Config, *http.Client) (provider.Provider, error) { return stubProvider{}, nil }
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
	newProvider = func(config.Config, *http.Client) (provider.Provider, error) { return stubProvider{}, nil }
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
	newProvider = func(config.Config, *http.Client) (provider.Provider, error) { return stubProvider{}, nil }
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
