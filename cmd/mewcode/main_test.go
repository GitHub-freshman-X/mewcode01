package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/config"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
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
	runTUI = func(*agent.Runner, *conversation.Session) error { return nil }
	if code := run([]string{"--config", "custom.yaml"}, &bytes.Buffer{}); code != 0 || loaded != "custom.yaml" {
		t.Fatalf("code=%d loaded=%s", code, loaded)
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
