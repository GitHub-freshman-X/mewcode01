package command

import "strings"

func Parse(input string) Invocation {
	input = strings.TrimSpace(input)
	if input == "" || !strings.HasPrefix(input, "/") {
		return Invocation{}
	}
	rest := strings.TrimSpace(strings.TrimPrefix(input, "/"))
	if rest == "" {
		return Invocation{IsCommand: true}
	}
	parts := strings.Fields(rest)
	return Invocation{IsCommand: true, Name: normalize(parts[0]), Args: strings.Join(parts[1:], " ")}
}
