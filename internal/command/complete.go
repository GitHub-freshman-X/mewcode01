package command

func Complete(registry *Registry, input string) []string {
	if registry == nil || len(input) == 0 || input[0] != '/' {
		return nil
	}
	return registry.Complete(input)
}
