package provider

import (
	"net/url"
	"strings"
)

type ToolSearchMode string

const (
	ToolSearchAuto     ToolSearchMode = "auto"
	ToolSearchEnabled  ToolSearchMode = "enabled"
	ToolSearchDisabled ToolSearchMode = "disabled"
)

func ResolveToolSearch(protocol, model, baseURL string, mode ToolSearchMode) (ToolSearchConfig, error) {
	if mode == "" {
		mode = ToolSearchAuto
	}
	if mode == ToolSearchDisabled {
		return ToolSearchConfig{Mode: ToolSearchFull}, nil
	}
	if !officialEndpoint(protocol, baseURL) || !supportsToolSearch(protocol, model) {
		return ToolSearchConfig{Mode: ToolSearchLocal}, nil
	}
	if protocol == "anthropic" {
		return ToolSearchConfig{Mode: ToolSearchNativeAnthropic}, nil
	}
	return ToolSearchConfig{Mode: ToolSearchNativeOpenAI}, nil
}

func officialEndpoint(protocol, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch protocol {
	case "openai":
		return host == "api.openai.com"
	case "anthropic":
		return host == "api.anthropic.com"
	default:
		return false
	}
}

func supportsToolSearch(protocol, model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if protocol == "anthropic" {
		if strings.Contains(model, "-4-5") || strings.Contains(model, "-4-6") || strings.Contains(model, "-4-7") || strings.Contains(model, "-4-8") {
			return true
		}
		return strings.HasPrefix(model, "claude-fable-5") || strings.HasPrefix(model, "claude-mythos-5") || strings.HasPrefix(model, "claude-opus-5")
	}
	if protocol != "openai" {
		return false
	}
	for _, prefix := range []string{"gpt-6-astra", "gpt-5.6", "gpt-daybreak-red-latest", "gpt-daybreak-blue-latest", "gpt-5.5", "gpt-5.4", "gpt-5.4-pro", "gpt-5.4-mini"} {
		if model == prefix || strings.HasPrefix(model, prefix+"-") {
			return model != "gpt-5.5-pro" && model != "gpt-5.4-nano"
		}
	}
	return false
}
