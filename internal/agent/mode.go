package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/prompt"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type preparedRequest struct {
	prompt        string
	displayPrompt string
	registry      *tools.Registry
	plans         []string
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
			prompt:        prompt,
			displayPrompt: "/plan " + prompt,
			registry:      registry.FilterBySafety(tools.SafetyReadOnly),
		}, nil
	case ModeDo:
		if prompt != "" {
			return preparedRequest{}, errors.New("/do does not accept a prompt")
		}
		plans := session.PendingPlans()
		if len(plans) == 0 {
			return preparedRequest{}, errors.New("no valid plan is available; run /plan first")
		}
		var body strings.Builder
		body.WriteString("Execute all pending plans below using the available tools. Preserve every plan's intent. If plans overlap or conflict, inspect the current workspace and use your judgment to choose a coherent implementation; do not silently omit a plan.\n")
		for i, plan := range plans {
			fmt.Fprintf(&body, "\nPlan %d:\n%s\n", i+1, plan)
		}
		return preparedRequest{prompt: strings.TrimSpace(body.String()), registry: registry, plans: plans}, nil
	default:
		return preparedRequest{}, fmt.Errorf("invalid agent mode %q", req.Mode)
	}
}

func toPromptMode(mode Mode) prompt.Mode {
	switch mode {
	case ModePlan:
		return prompt.ModePlan
	case ModeDo:
		return prompt.ModeDo
	default:
		return prompt.ModeAct
	}
}
