package command

import (
	"context"
	"fmt"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/skills"
)

type Kind string

const (
	KindLocal   Kind = "local"
	KindLocalUI Kind = "local_ui"
	KindPrompt  Kind = "prompt"
)

type Command struct {
	Name, Description, Usage, ArgPrompt string
	Aliases                             []string
	Kind                                Kind
	Hidden                              bool
	Handler                             Handler
}

type Invocation struct {
	IsCommand bool
	Name      string
	Args      string
}

type UIController interface {
	AddSystemMessage(string)
	StartAgent(agent.Request) error
	SetPlanMode(bool)
	PlanMode() bool
	RequestExit()
	TokenUsage() provider.Usage
	RefreshStatus()
	MemoryClearPending() bool
	SetMemoryClearPending(bool)
}

type SessionMeta struct {
	ID           string
	Title        string
	MessageCount int
}

type SessionService interface {
	Current() SessionMeta
	List() ([]SessionMeta, error)
	New(context.Context) error
	Resume(context.Context, string) error
	Delete(string) error
}

type MemorySummary struct {
	UserCount, ProjectCount int
}

type MemoryItem struct {
	Kind, Name, Description string
}

type MemoryService interface {
	Summary() (MemorySummary, error)
	Items() ([]MemoryItem, error)
	Add(kind, content string) error
	Clear() error
}

type SkillService interface {
	ReloadSkills() error
	SkillDirectory() []skills.Metadata
}

type CommandContext struct {
	Context  context.Context
	UI       UIController
	Sessions SessionService
	Memory   MemoryService
	Skills   SkillService
	Logger   *logging.Logger
	Args     string
}

type Handler func(CommandContext) error

func (c CommandContext) systemf(format string, args ...any) {
	if c.UI != nil {
		c.UI.AddSystemMessage(fmt.Sprintf(format, args...))
	}
}
