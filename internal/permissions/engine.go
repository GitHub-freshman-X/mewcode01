package permissions

import (
	"context"
	"errors"
	"fmt"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type Mode string

const (
	ModeStrict  Mode = "strict"
	ModeDefault Mode = "default"
	ModeRelaxed Mode = "relaxed"
)

type Choice string

const (
	ChoiceDeny           Choice = "deny"
	ChoiceAllowOnce      Choice = "allow_once"
	ChoiceAllowSession   Choice = "allow_session"
	ChoiceAllowPermanent Choice = "allow_permanent"
	ChoiceCancel         Choice = "cancel"
)

type Decision struct {
	Action       Action
	Stage        Stage
	Reason       string
	Rule         *Rule
	Request      Request
	SuggestedKey string
}

type Confirmation struct {
	Decision Decision
	Choice   Choice
}

type Engine struct {
	Mode    Mode
	Rules   *RuleStore
	Sandbox Sandbox
	Paths   FilePaths
}

func (e *Engine) Decide(ctx context.Context, call provider.ToolCall, tool tools.Tool) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	if e == nil {
		return Decision{Action: ActionAllow, Stage: StageMode, Reason: "permissions disabled"}, nil
	}
	req, err := BuildRequest(call, tool, e.Sandbox)
	if err != nil {
		if errors.Is(err, ErrPathEscapesSandbox) {
			return Decision{Action: ActionDeny, Stage: StageSandbox, Reason: "path escapes workspace sandbox", SuggestedKey: call.Name + "()", Request: Request{CallID: call.ID, Tool: call.Name}}, nil
		}
		return Decision{}, err
	}
	decision := Decision{Request: req, SuggestedKey: SuggestedRuleKey(req)}
	if req.Tool == "run_command" {
		match, err := CheckCommandBlacklist(req.MatchTarget)
		if err != nil {
			return Decision{}, err
		}
		if match != nil {
			decision.Action = ActionDeny
			decision.Stage = StageBlacklist
			decision.Reason = match.Reason
			return decision, nil
		}
	}
	if e.Rules != nil {
		rule, ok, err := e.Rules.Find(req)
		if err != nil {
			return Decision{}, err
		}
		if ok {
			decision.Stage = StageRule
			decision.Rule = &rule
			decision.Reason = fmt.Sprintf("%s rule matched", rule.Scope)
			if rule.Effect == EffectAllow {
				decision.Action = ActionAllow
			} else {
				decision.Action = ActionDeny
			}
			return decision, nil
		}
	}
	decision.Stage = StageMode
	switch normalizeMode(e.Mode) {
	case ModeStrict:
		decision.Action = ActionAsk
		decision.Reason = "strict mode requires confirmation"
	case ModeRelaxed:
		decision.Action = ActionAllow
		decision.Reason = "relaxed mode allows unmatched calls"
	default:
		if tools.NormalizeSafety(req.Safety) == tools.SafetyReadOnly {
			decision.Action = ActionAllow
			decision.Reason = "default mode allows read-only calls"
		} else {
			decision.Action = ActionAsk
			decision.Reason = "default mode requires confirmation for side-effect calls"
		}
	}
	return decision, nil
}

func (e *Engine) ApplyConfirmation(conf Confirmation) error {
	if e == nil {
		return nil
	}
	switch conf.Choice {
	case ChoiceAllowOnce, ChoiceDeny, ChoiceCancel:
		return nil
	case ChoiceAllowSession:
		if e.Rules == nil {
			e.Rules = NewRuleStore(RuleSet{})
		}
		rule, err := ParseRule(conf.Decision.SuggestedKey, EffectAllow, ScopeSession, 0)
		if err != nil {
			return err
		}
		e.Rules.AddSessionRule(rule)
		return nil
	case ChoiceAllowPermanent:
		rule, err := ParseRule(conf.Decision.SuggestedKey, EffectAllow, ScopeProject, 0)
		if err != nil {
			return err
		}
		if err := AppendProjectAllow(e.Paths, rule); err != nil {
			return err
		}
		if e.Rules == nil {
			e.Rules = NewRuleStore(RuleSet{})
		}
		sessionRule := rule
		sessionRule.Scope = ScopeSession
		e.Rules.AddSessionRule(sessionRule)
		return nil
	default:
		return fmt.Errorf("unknown confirmation choice %q", conf.Choice)
	}
}

func normalizeMode(mode Mode) Mode {
	switch mode {
	case ModeStrict, ModeRelaxed:
		return mode
	default:
		return ModeDefault
	}
}
