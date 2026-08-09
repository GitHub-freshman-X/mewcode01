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
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/config"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/envfile"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/mcp"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider/factory"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
	"github.com/GitHub-freshman-X/mewcode01/internal/tui"
)

var (
	defaultConfigPath   = config.DefaultPath
	loadConfig          = config.Load
	newProvider         = factory.New
	runTUI              = tui.RunWithPermissions
	permissionFilePaths = permissions.DefaultFilePaths
	newLogger           = func(root string) (*logging.Logger, error) { return logging.New(root, time.Now, os.Getpid()) }
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
	p, err := newProvider(cfg, newHTTPClient())
	if err != nil {
		fmt.Fprintln(stderr, provider.UserError(err))
		return 1
	}
	registry, err := tools.NewDefaultRegistry(root)
	if err != nil {
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
	paths, err := permissionFilePaths(root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	rules, err := permissions.LoadRuleSet(paths)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	sandbox, err := permissions.NewSandbox(root)
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
	session := conversation.NewSession()
	bridge := tui.NewPermissionBridge()
	runner := agent.NewRunner(p, session, registry, executor, agent.Options{
		MaxIterations: cfg.Agent.MaxIterations,
		MaxTokens:     cfg.MaxTokens,
		Thinking:      provider.ThinkingOptions{Enabled: cfg.Thinking.Enabled, BudgetTokens: cfg.Thinking.BudgetTokens},
		Workspace:     root,
		Permissions:   gate,
		Confirmer:     bridge,
	})
	if err := runTUI(runner, session, bridge); err != nil {
		fmt.Fprintln(stderr, "tui:", err)
		return 1
	}
	return 0
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

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 90 * time.Second}}
}
