package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/config"
	contextmanager "github.com/GitHub-freshman-X/mewcode01/internal/context"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/envfile"
	"github.com/GitHub-freshman-X/mewcode01/internal/hooks"
	"github.com/GitHub-freshman-X/mewcode01/internal/instructions"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/mcp"
	"github.com/GitHub-freshman-X/mewcode01/internal/memory"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/prompt"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider/factory"
	"github.com/GitHub-freshman-X/mewcode01/internal/skills"
	"github.com/GitHub-freshman-X/mewcode01/internal/subagent"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
	"github.com/GitHub-freshman-X/mewcode01/internal/tui"
	"github.com/GitHub-freshman-X/mewcode01/internal/worktree"
)

var (
	defaultConfigPath   = config.DefaultPath
	loadConfig          = config.Load
	newProvider         = factory.New
	runTUI              = tui.RunWithPermissions
	permissionFilePaths = permissions.DefaultFilePaths
	newLogger           = func(root string) (*logging.Logger, error) { return logging.New(root, time.Now, os.Getpid()) }
	userConfigDir       = os.UserConfigDir
)

func main() { os.Exit(run(os.Args[1:], os.Stderr)) }

func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("mewcode", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "path to YAML configuration")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 2
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	logger, err := newLogger(root)
	if err != nil {
		fmt.Fprintln(stderr, "logging:", err)
		logger = logging.Nop()
	}
	defer func() {
		if err := logger.Close(); err != nil {
			fmt.Fprintln(stderr, "logging close:", err)
		}
	}()
	result, envErr := envfile.Load(filepath.Join(root, ".env"), os.LookupEnv, os.Setenv)
	if envErr != nil {
		logger.Error("dotenv load failed", logging.Fields{"status": "failed"})
		fmt.Fprintln(stderr, "dotenv: load failed")
	} else if !result.Found {
		logger.Info("dotenv file not found", logging.Fields{"status": "not_found"})
	} else {
		logger.Info("dotenv file loaded", logging.Fields{"status": "loaded", "variable_count": len(result.Loaded)})
		for _, key := range result.Skipped {
			logger.Info("dotenv variable skipped", logging.Fields{"status": "system_preferred", "variable": key})
		}
	}
	path := *configPath
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	cfg, err := loadConfig(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	p, err := newProvider(cfg, newHTTPClient(), logger)
	if err != nil {
		fmt.Fprintln(stderr, provider.UserError(err))
		return 1
	}
	configRoot, err := userConfigDir()
	if err != nil {
		fmt.Fprintln(stderr, "user config:", err)
		return 1
	}
	instructionPaths := instructions.NewPaths(configRoot, root)
	customInstructions, err := instructions.LoadInstructions(instructionPaths)
	if err != nil {
		fmt.Fprintln(stderr, "instructions:", err)
		return 1
	}
	memoryPaths := memory.NewPaths(configRoot, root)
	memoryIndexes, err := memory.LoadIndexes(memoryPaths)
	if err != nil {
		fmt.Fprintln(stderr, "memory:", err)
		return 1
	}
	sessionStore := conversation.NewSessionStore(instructionPaths.Sessions)
	if _, err := sessionStore.CleanupExpired(time.Now()); err != nil {
		fmt.Fprintln(stderr, "session cleanup:", err)
		return 1
	}
	session, sessionMeta, err := sessionStore.Create()
	if err != nil {
		fmt.Fprintln(stderr, "session create:", err)
		return 1
	}
	definitions, err := subagent.Discover(subagent.DiscoverOptions{
		ProjectDir:              filepath.Join(root, ".mewcode", "agents"),
		UserDir:                 filepath.Join(configRoot, "mewcode", "agents"),
		EnableVerificationAgent: cfg.Agent.EnableVerificationAgent,
	})
	if err != nil {
		fmt.Fprintln(stderr, "agents:", err)
		return 1
	}
	worktreeManager := worktree.NewManager(root)
	worktreeManager.Options = worktree.Options{LocalFiles: cfg.Worktree.LocalFiles, SymlinkDirectories: cfg.Worktree.SymlinkDirectories, Retention: time.Duration(cfg.Worktree.RetentionHours) * time.Hour}
	worktreeManager.Logger = logger
	if _, err := worktreeManager.RecoverSession(); err != nil {
		logger.Error("worktree session recovery failed", logging.Fields{"stage": "worktree_recovery", "status": "failed"})
	}
	if err := worktreeManager.CleanupExpired(context.Background(), time.Now()); err != nil {
		logger.Error("worktree cleanup failed", logging.Fields{"stage": "worktree_cleanup", "status": "failed"})
	}
	subAgentRuntime := agent.NewSubAgentRuntime(definitions, subagent.NewTaskManager())
	subAgentRuntime.Worktrees = worktreeManager
	workspaceRoot := root
	if current := worktreeManager.Current(); current != nil {
		workspaceRoot = current.WorktreePath
	}
	registry, err := tools.NewDefaultRegistry(workspaceRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := registry.Register(tools.NewAgentTool()); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	manager := mcp.NewManager(newHTTPClient(), func(diagnostic mcp.Diagnostic) {
		fmt.Fprintln(stderr, diagnostic)
	}, logger)
	defer func() {
		if err := manager.Close(context.Background()); err != nil {
			fmt.Fprintln(stderr, "mcp close:", err)
		}
	}()
	manager.ConnectAndRegister(context.Background(), registry, cfg.MCPServers)
	skillManager, err := skills.NewManager(skills.DiscoverOptions{ProjectDir: filepath.Join(root, ".mewcode", "skills"), UserDir: filepath.Join(configRoot, "mewcode", "skills"), ToolNames: registry.Names()})
	if err != nil {
		fmt.Fprintln(stderr, "skills:", err)
		return 1
	}
	hookPaths, err := hooks.DefaultFilePaths(root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	hookRules, err := hooks.LoadRuleSet(hookPaths)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	hookEngine := hooks.NewEngine(hookRules, hooks.Executor{HTTPClient: newHTTPClient()}, logger)
	hookEngine.Run(context.Background(), hooks.EventStartup, hooks.Context{})
	defer hookEngine.Run(context.Background(), hooks.EventShutdown, hooks.Context{})
	paths, err := permissionFilePaths(workspaceRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	rules, err := permissions.LoadRuleSet(paths)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	sandbox, err := permissions.NewSandbox(workspaceRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	gate := &permissions.Engine{
		Mode:    permissionMode(cfg.Permissions.Mode),
		Rules:   permissions.NewRuleStore(rules),
		Sandbox: sandbox,
		Paths:   paths,
	}
	executor := tools.NewExecutor(30 * time.Second)
	bridge := tui.NewPermissionBridge()
	runner := agent.NewRunner(p, session, registry, executor, agent.Options{
		MaxIterations:   cfg.Agent.MaxIterations,
		MaxTokens:       cfg.MaxTokens,
		Model:           cfg.Model,
		Thinking:        provider.ThinkingOptions{Enabled: cfg.Thinking.Enabled, BudgetTokens: cfg.Thinking.BudgetTokens},
		Workspace:       workspaceRoot,
		LogDirectory:    filepath.Join(root, "logs"),
		SessionID:       sessionMeta.ID,
		SessionStore:    sessionStore,
		Permissions:     gate,
		Confirmer:       bridge,
		Context:         agentContextConfig(cfg.Agent.Context),
		Logger:          logger,
		Skills:          skillManager,
		Hooks:           hookEngine,
		SubAgents:       subAgentRuntime,
		OptionalModules: prompt.OptionalModules{CustomInstructions: nonEmpty(customInstructions), LongTermMemory: memoryIndexes},
		Memory: memory.NewService(memoryPaths, memory.ServiceOptions{
			Caller: providerMemoryCaller{provider: p}, Sessions: memorySessionLister{store: sessionStore}, Logger: logger,
		}),
	})
	hookEngine.SetAgentRunner(hookSubAgentRunner{})
	if err := runTUI(runner, session, bridge); err != nil {
		fmt.Fprintln(stderr, "tui:", err)
		return 1
	}
	return 0
}

func nonEmpty(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

type providerMemoryCaller struct{ provider provider.Provider }

func (c providerMemoryCaller) Call(ctx context.Context, request provider.ChatRequest) (string, error) {
	events, done := c.provider.Stream(ctx, request)
	var output strings.Builder
	for event := range events {
		if event.Type == provider.EventTextDelta {
			output.WriteString(event.Delta)
		}
	}
	return output.String(), <-done
}

type memorySessionLister struct{ store *conversation.SessionStore }

func (l memorySessionLister) List() ([]memory.SessionMeta, error) {
	metas, err := l.store.List()
	if err != nil {
		return nil, err
	}
	return make([]memory.SessionMeta, len(metas)), nil
}

func agentContextConfig(cfg config.ContextConfig) contextmanager.Config {
	return contextmanager.Config{
		WindowTokens: cfg.WindowTokens, SummaryOutputTokens: cfg.SummaryOutputTokens,
		AutoSafetyTokens: cfg.AutoSafetyTokens, ManualSafetyTokens: cfg.ManualSafetyTokens,
		SingleResultChars: cfg.SingleResultChars, MessageResultChars: cfg.MessageResultChars,
		PreviewChars: cfg.PreviewChars, RecentTokens: cfg.RecentTokens,
		RecentMessageMinimum: cfg.RecentMessageMinimum,
	}
}

func permissionMode(mode config.PermissionMode) permissions.Mode {
	switch mode {
	case config.PermissionModeStrict:
		return permissions.ModeStrict
	case config.PermissionModeRelaxed:
		return permissions.ModeRelaxed
	default:
		return permissions.ModeDefault
	}
}

type hookSubAgentRunner struct{}

func (hookSubAgentRunner) RunHookAgent(ctx context.Context, prompt string) (string, error) {
	host, ok := tools.SubAgentHostFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("subagent runtime is not available for this hook event")
	}
	result, err := host.DispatchSubAgent(ctx, tools.AgentInput{Prompt: prompt, Description: "hook agent task", SubagentType: "general-purpose", RunInBackground: true})
	if err != nil {
		return "", err
	}
	if result.Error != nil {
		return "", fmt.Errorf("%s", result.Error.Message)
	}
	return result.JSON(), nil
}

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 90 * time.Second}}
}
