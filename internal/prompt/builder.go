package prompt

import "github.com/GitHub-freshman-X/mewcode01/internal/provider"

type BuildContext struct {
	Environment     Environment
	Mode            Mode
	Iteration       int
	InjectionPolicy InjectionPolicy
	OptionalModules OptionalModules
}

type OptionalModules struct {
	CustomInstructions []string
	AvailableSkills    []string
	ActiveSkills       []string
	LongTermMemory     []string
}

func BuildBundle(ctx BuildContext) (provider.PromptBundle, []Module, error) {
	modules := append([]Module{}, fixedModules()...)
	modules = append(modules, optionalModules(ctx.OptionalModules)...)
	stable, renderedModules := renderModules(modules)

	envMessage, err := EnvironmentMessage(ctx.Environment)
	if err != nil {
		return provider.PromptBundle{}, nil, err
	}
	dynamic := []provider.SystemMessage{envMessage}
	injection := ModeInjectionFor(ctx.Mode, ctx.Iteration, ctx.InjectionPolicy)
	if injection.Kind != InjectionNone && injection.Content != "" {
		dynamic = append(dynamic, provider.SystemMessage{
			Tag:       "mew.mode." + string(ctx.Mode),
			Content:   injection.Content,
			Cacheable: injection.Cacheable,
		})
	}
	return provider.PromptBundle{
		StableSystem:  stable,
		DynamicSystem: dynamic,
		CachePolicy: provider.CachePolicy{
			Enable:          true,
			StableSystem:    true,
			StableTools:     true,
			DynamicMessages: false,
		},
	}, renderedModules, nil
}
