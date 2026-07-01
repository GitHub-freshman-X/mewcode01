package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/config"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider/factory"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
	"github.com/GitHub-freshman-X/mewcode01/internal/tui"
)

var (
	defaultConfigPath = config.DefaultPath
	loadConfig        = config.Load
	newProvider       = factory.New
	runTUI            = tui.Run
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
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	registry, err := tools.NewDefaultRegistry(root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	executor := tools.NewExecutor(30 * time.Second)
	session := conversation.NewSession()
	runner := agent.NewRunner(p, session, registry, executor, agent.Options{
		MaxIterations: cfg.Agent.MaxIterations,
		MaxTokens:     cfg.MaxTokens,
		Thinking:      provider.ThinkingOptions{Enabled: cfg.Thinking.Enabled, BudgetTokens: cfg.Thinking.BudgetTokens},
	})
	if err := runTUI(runner, session); err != nil {
		fmt.Fprintln(stderr, "tui:", err)
		return 1
	}
	return 0
}

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 90 * time.Second}}
}
