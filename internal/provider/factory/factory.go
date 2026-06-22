package factory

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/GitHub-freshman-X/mewcode01/internal/config"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider/anthropic"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider/openai"
)

func New(cfg config.Config, client *http.Client) (provider.Provider, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, &provider.AppError{Stage: provider.StageConfig, Message: "invalid base_url", Cause: err}
	}
	switch cfg.Protocol {
	case config.ProtocolAnthropic:
		return anthropic.New(anthropic.Options{BaseURL: u, APIKey: cfg.APIKey, Model: cfg.Model, HTTPClient: client}), nil
	case config.ProtocolOpenAI:
		return openai.New(openai.Options{BaseURL: u, APIKey: cfg.APIKey, Model: cfg.Model, HTTPClient: client}), nil
	default:
		return nil, &provider.AppError{Stage: provider.StageConfig, Message: fmt.Sprintf("unsupported protocol %q", cfg.Protocol)}
	}
}
