package prompt

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type Environment struct {
	Workspace string
	OS        string
	Shell     string
	Date      string
	Mode      Mode
	Tools     []string
}

func CollectEnvironment(mode Mode, registry *tools.Registry, workspace string, clock Clock) (Environment, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return Environment{}, errors.New("prompt environment requires workspace")
	}
	if registry == nil {
		return Environment{}, errors.New("prompt environment requires tool registry")
	}
	if clock == nil {
		clock = SystemClock{}
	}
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = strings.TrimSpace(os.Getenv("COMSPEC"))
	}
	if shell == "" {
		shell = "unknown"
	}
	defs := registry.Definitions()
	if len(defs) == 0 {
		return Environment{}, errors.New("prompt environment requires at least one tool")
	}
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		if strings.TrimSpace(def.Name) != "" {
			names = append(names, def.Name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return Environment{}, errors.New("prompt environment requires tool names")
	}
	return Environment{
		Workspace: workspace,
		OS:        runtime.GOOS,
		Shell:     shell,
		Date:      clock.Now().Format("2006-01-02"),
		Mode:      mode,
		Tools:     names,
	}, nil
}

func EnvironmentMessage(env Environment) (provider.SystemMessage, error) {
	if strings.TrimSpace(env.Workspace) == "" {
		return provider.SystemMessage{}, errors.New("environment message requires workspace")
	}
	if strings.TrimSpace(env.OS) == "" {
		return provider.SystemMessage{}, errors.New("environment message requires os")
	}
	if strings.TrimSpace(env.Shell) == "" {
		return provider.SystemMessage{}, errors.New("environment message requires shell")
	}
	if strings.TrimSpace(env.Date) == "" {
		return provider.SystemMessage{}, errors.New("environment message requires date")
	}
	if len(env.Tools) == 0 {
		return provider.SystemMessage{}, errors.New("environment message requires tools")
	}
	content := fmt.Sprintf("Workspace: %s\nOS: %s\nShell: %s\nDate: %s\nMode: %s\nAvailable tools: %s",
		env.Workspace, env.OS, env.Shell, env.Date, env.Mode, strings.Join(env.Tools, ", "))
	return provider.SystemMessage{Tag: "mew.environment", Content: content}, nil
}
