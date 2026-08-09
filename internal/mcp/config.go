package mcp

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/GitHub-freshman-X/mewcode01/internal/config"
)

var ErrMissingEnvironment = errors.New("mcp environment variable is not set")

var environmentReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func ExpandServer(name string, raw config.MCPServerConfig, lookup func(string) (string, bool)) (config.MCPServerConfig, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	server := cloneServer(raw)
	var err error
	if server.Command, err = expandString(name, server.Command, lookup); err != nil {
		return config.MCPServerConfig{}, err
	}
	if server.URL, err = expandString(name, server.URL, lookup); err != nil {
		return config.MCPServerConfig{}, err
	}
	for i := range server.Args {
		if server.Args[i], err = expandString(name, server.Args[i], lookup); err != nil {
			return config.MCPServerConfig{}, err
		}
	}
	for key, value := range server.Env {
		if server.Env[key], err = expandString(name, value, lookup); err != nil {
			return config.MCPServerConfig{}, err
		}
	}
	for key, value := range server.Headers {
		if server.Headers[key], err = expandString(name, value, lookup); err != nil {
			return config.MCPServerConfig{}, err
		}
	}
	return server, nil
}

func expandString(server, value string, lookup func(string) (string, bool)) (string, error) {
	var missing string
	expanded := environmentReference.ReplaceAllStringFunc(value, func(reference string) string {
		key := reference[2 : len(reference)-1]
		replacement, ok := lookup(key)
		if !ok && missing == "" {
			missing = key
		}
		return replacement
	})
	if missing != "" {
		return "", fmt.Errorf("mcp server %q: %w: %q", server, ErrMissingEnvironment, missing)
	}
	return expanded, nil
}

func cloneServer(raw config.MCPServerConfig) config.MCPServerConfig {
	clone := raw
	clone.Args = append([]string(nil), raw.Args...)
	clone.Env = cloneStrings(raw.Env)
	clone.Headers = cloneStrings(raw.Headers)
	return clone
}

func cloneStrings(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
