package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

type FilePaths struct{ User, Project, Local string }

func DefaultFilePaths(workspace string) (FilePaths, error) {
	if strings.TrimSpace(workspace) == "" {
		return FilePaths{}, errors.New("hook workspace is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return FilePaths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	return FilePaths{User: filepath.Join(home, ".mewcode", "config.yaml"), Project: filepath.Join(workspace, ".mewcode", "config.yaml"), Local: filepath.Join(workspace, ".mewcode", "config.local.yaml")}, nil
}

func LoadRuleSet(paths FilePaths) ([]Rule, error) {
	var rules []Rule
	for _, item := range []struct {
		path  string
		label string
	}{{paths.User, "user"}, {paths.Project, "project"}, {paths.Local, "local"}} {
		loaded, err := loadFile(item.path)
		if err != nil {
			return nil, err
		}
		for i := range loaded {
			loaded[i].Index = len(rules)
			if loaded[i].ID == "" {
				loaded[i].ID = fmt.Sprintf("%s-%d", item.label, i+1)
			}
			if err := ValidateRule(&loaded[i]); err != nil {
				return nil, fmt.Errorf("%s: hook %q: %w", item.path, loaded[i].ID, err)
			}
			rules = append(rules, loaded[i])
		}
	}
	return rules, nil
}

func loadFile(path string) ([]Rule, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read hook config %s: %w", path, err)
	}
	var raw struct {
		Hooks []Rule `yaml:"hooks"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse hook config %s: %w", path, err)
	}
	return raw.Hooks, nil
}

func ValidateRule(rule *Rule) error {
	if rule == nil {
		return errors.New("hook is required")
	}
	if !rule.Event.Valid() {
		return fmt.Errorf("unknown event %q", rule.Event)
	}
	if !rule.Action.Type.Valid() {
		return fmt.Errorf("unknown action type %q", rule.Action.Type)
	}
	condition, err := ParseCondition(rule.If)
	if err != nil {
		return err
	}
	rule.Condition = condition
	if rule.Reject && rule.Event != EventPreToolUse {
		return errors.New("reject is only allowed for pre_tool_use")
	}
	if rule.Async && rule.Event == EventPreToolUse {
		return errors.New("async is not allowed for pre_tool_use")
	}
	switch rule.Action.Type {
	case ActionCommand:
		if strings.TrimSpace(rule.Action.Command) == "" {
			return errors.New("command action requires command")
		}
		if rule.Action.RawTimeout != "" {
			timeout, err := time.ParseDuration(rule.Action.RawTimeout)
			if err != nil || timeout <= 0 {
				return fmt.Errorf("invalid command timeout %q", rule.Action.RawTimeout)
			}
			rule.Action.Timeout = timeout
		}
	case ActionPrompt:
		if strings.TrimSpace(rule.Action.Message) == "" {
			return errors.New("prompt action requires message")
		}
	case ActionHTTP:
		if strings.TrimSpace(rule.Action.URL) == "" {
			return errors.New("http action requires url")
		}
	case ActionAgent:
		if strings.TrimSpace(rule.Action.Prompt) == "" {
			return errors.New("agent action requires prompt")
		}
	}
	return nil
}
