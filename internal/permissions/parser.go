package permissions

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

type Scope string

const (
	ScopeSession Scope = "session"
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
)

type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
	ActionAsk   Action = "ask"
)

type Stage string

const (
	StageBlacklist Stage = "blacklist"
	StageSandbox   Stage = "sandbox"
	StageRule      Stage = "rule"
	StageMode      Stage = "mode"
	StageConfirm   Stage = "confirm"
)

type Rule struct {
	Key     string
	Tool    string
	Pattern string
	Effect  Effect
	Scope   Scope
	Index   int
}

var toolNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ParseRule(key string, effect Effect, scope Scope, index int) (Rule, error) {
	key = strings.TrimSpace(key)
	if effect != EffectAllow && effect != EffectDeny {
		return Rule{}, fmt.Errorf("invalid effect %q", effect)
	}
	open := strings.IndexByte(key, '(')
	close := strings.LastIndexByte(key, ')')
	if open <= 0 || close != len(key)-1 || strings.Contains(key[open+1:close], "(") {
		return Rule{}, fmt.Errorf("invalid rule key %q", key)
	}
	tool := strings.TrimSpace(key[:open])
	pattern := strings.TrimSpace(key[open+1 : close])
	if !toolNamePattern.MatchString(tool) {
		return Rule{}, fmt.Errorf("invalid tool name %q", tool)
	}
	if pattern == "" {
		return Rule{}, errors.New("rule pattern must not be empty")
	}
	if IsGlob(pattern) {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return Rule{}, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
	}
	return Rule{Key: key, Tool: tool, Pattern: pattern, Effect: effect, Scope: scope, Index: index}, nil
}

func IsGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func MatchRule(rule Rule, req Request) (bool, error) {
	if rule.Tool != req.Tool {
		return false, nil
	}
	target := filepath.ToSlash(req.MatchTarget)
	pattern := filepath.ToSlash(rule.Pattern)
	if !IsGlob(pattern) {
		return target == pattern, nil
	}
	return matchGlob(pattern, target)
}

func matchGlob(pattern, target string) (bool, error) {
	if strings.Contains(pattern, "**") {
		return matchDoubleStar(pattern, target)
	}
	return filepath.Match(pattern, target)
}

func matchDoubleStar(pattern, target string) (bool, error) {
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid glob pattern %q: multiple ** segments", pattern)
	}
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")
	if prefix != "" && target != prefix && !strings.HasPrefix(target, prefix+"/") {
		return false, nil
	}
	if suffix == "" {
		return true, nil
	}
	ok, err := filepath.Match(suffix, filepath.Base(target))
	if err != nil || ok {
		return ok, err
	}
	return filepath.Match(suffix, target)
}
