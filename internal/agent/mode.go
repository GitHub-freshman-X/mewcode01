package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type preparedRequest struct {
	prompt   string
	registry *tools.Registry
}

func prepareRequest(req Request, session *conversation.Session, registry *tools.Registry) (preparedRequest, error) {
	if session == nil || registry == nil {
		return preparedRequest{}, errors.New("agent is not configured")
	}
	prompt := strings.TrimSpace(req.Prompt)
	switch req.Mode {
	case ModeAct:
		if prompt == "" {
			return preparedRequest{}, errors.New("prompt is empty")
		}
		return preparedRequest{prompt: prompt, registry: registry}, nil
	case ModePlan:
		if prompt == "" {
			return preparedRequest{}, errors.New("plan task is empty")
		}
		return preparedRequest{
			prompt:   "Explore the workspace using read-only tools only. Produce a concrete implementation plan; do not modify files or run commands.\n\nTask:\n" + prompt,
			registry: registry.FilterBySafety(tools.SafetyReadOnly),
		}, nil
	case ModeDo:
		if prompt != "" {
			return preparedRequest{}, errors.New("/do does not accept a prompt")
		}
		plan, ok := session.LatestPlan()
		if !ok {
			return preparedRequest{}, errors.New("no valid plan is available; run /plan first")
		}
		return preparedRequest{prompt: fmt.Sprintf("Execute the following saved plan using the available tools.\n\nPlan:\n%s", plan), registry: registry}, nil
	default:
		return preparedRequest{}, fmt.Errorf("invalid agent mode %q", req.Mode)
	}
}
