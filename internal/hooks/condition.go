package hooks

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type join string

const (
	joinAnd join = "and"
	joinOr  join = "or"
)

type Condition struct {
	Field, Operator, Value string
	regex                  *regexp.Regexp
}
type ConditionGroup struct {
	join       join
	conditions []Condition
}

func ParseCondition(input string) (ConditionGroup, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ConditionGroup{}, nil
	}
	if strings.Contains(input, "&&") && strings.Contains(input, "||") {
		return ConditionGroup{}, fmt.Errorf("condition cannot mix && and ||")
	}
	separator, mode := "", joinAnd
	if strings.Contains(input, "&&") {
		separator = "&&"
	}
	if strings.Contains(input, "||") {
		separator, mode = "||", joinOr
	}
	parts := []string{input}
	if separator != "" {
		parts = strings.Split(input, separator)
	}
	group := ConditionGroup{join: mode, conditions: make([]Condition, 0, len(parts))}
	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) != 3 {
			return ConditionGroup{}, fmt.Errorf("invalid condition %q", part)
		}
		value := strings.Trim(fields[2], "\"'")
		condition := Condition{Field: fields[0], Operator: fields[1], Value: value}
		switch condition.Operator {
		case "==", "!=":
		case "=~":
			pattern := strings.Trim(value, "/")
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return ConditionGroup{}, fmt.Errorf("invalid regular expression %q: %w", value, err)
			}
			condition.regex = compiled
		case "~=":
			if _, err := matchGlob(value, ""); err != nil {
				return ConditionGroup{}, fmt.Errorf("invalid glob %q: %w", value, err)
			}
		default:
			return ConditionGroup{}, fmt.Errorf("unknown condition operator %q", condition.Operator)
		}
		group.conditions = append(group.conditions, condition)
	}
	return group, nil
}

func (g ConditionGroup) Match(ctx Context) bool {
	if len(g.conditions) == 0 {
		return true
	}
	for _, condition := range g.conditions {
		value := ctx.Field(condition.Field)
		matched := false
		switch condition.Operator {
		case "==":
			matched = value == condition.Value
		case "!=":
			matched = value != condition.Value
		case "=~":
			matched = condition.regex.MatchString(value)
		case "~=":
			matched, _ = matchGlob(condition.Value, value)
		}
		if g.join == joinAnd && !matched {
			return false
		}
		if g.join == joinOr && matched {
			return true
		}
	}
	return g.join == joinAnd
}

func matchGlob(pattern, target string) (bool, error) {
	pattern = filepath.ToSlash(pattern)
	target = filepath.ToSlash(target)
	if !strings.Contains(pattern, "**") {
		return filepath.Match(pattern, target)
	}
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid glob %q: multiple ** segments", pattern)
	}
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")
	if prefix != "" && target != prefix && !strings.HasPrefix(target, prefix+"/") {
		return false, nil
	}
	if suffix == "" {
		return true, nil
	}
	if matched, err := filepath.Match(suffix, filepath.Base(target)); err != nil || matched {
		return matched, err
	}
	return filepath.Match(suffix, target)
}
