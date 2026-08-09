package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/GitHub-freshman-X/mewcode01/internal/config"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type Diagnostic struct {
	Server string
	Stage  string
	Err    error
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("mcp server %q %s: %v", d.Server, d.Stage, d.Err)
}

type Reporter func(Diagnostic)
type Manager struct {
	httpClient *http.Client
	reporter   Reporter
	logger     *logging.Logger
	clients    map[string]*Client
	order      []string
}

func NewManager(httpClient *http.Client, reporter Reporter, loggers ...*logging.Logger) *Manager {
	return &Manager{httpClient: httpClient, reporter: reporter, logger: normalizedLogger(loggers), clients: map[string]*Client{}}
}
func (m *Manager) ConnectAndRegister(ctx context.Context, registry *tools.Registry, servers map[string]config.MCPServerConfig) []Diagnostic {
	names := make([]string, 0, len(servers))
	for n := range servers {
		names = append(names, n)
	}
	sort.Strings(names)
	var diagnostics []Diagnostic
	for _, name := range names {
		logger := m.logger.WithFields(logging.Fields{"server": name})
		raw := servers[name]
		logger.Info("MCP server configuration started", logging.Fields{"stage": "configuration", "status": "started"})
		server, err := ExpandServer(name, raw, nil)
		if err != nil {
			logger.Error("MCP server configuration failed", logging.Fields{"stage": "configuration", "status": "configuration_failed"})
			diagnostics = m.report(diagnostics, Diagnostic{name, "configuration", err})
			continue
		}
		var transport Transport
		switch server.Type {
		case config.MCPTransportStdio:
			logger.Info("MCP server connection started", logging.Fields{"stage": "connect", "status": "started", "transport": "stdio"})
			st := NewStdioTransport(server.Command, server.Args, server.Env, logger)
			if err := st.Start(ctx); err != nil {
				logger.Error("MCP server connection failed", logging.Fields{"stage": "connect", "status": "connect_failed", "transport": "stdio"})
				diagnostics = m.report(diagnostics, Diagnostic{name, "connect", err})
				continue
			}
			transport = st
		case config.MCPTransportHTTP:
			logger.Info("MCP server connection started", logging.Fields{"stage": "connect", "status": "started", "transport": "http"})
			transport = NewHTTPTransport(server.URL, server.Headers, m.httpClient)
		default:
			diagnostics = m.report(diagnostics, Diagnostic{name, "configuration", errors.New("unsupported transport")})
			continue
		}
		logger.Info("MCP server connected", logging.Fields{"stage": "connect", "status": "connected"})
		client := NewClient(transport, logger)
		if err := client.Initialize(ctx); err != nil {
			_ = client.Close(ctx)
			diagnostics = m.report(diagnostics, Diagnostic{name, "initialize", err})
			continue
		}
		remotes, err := client.ListTools(ctx)
		if err != nil {
			_ = client.Close(ctx)
			diagnostics = m.report(diagnostics, Diagnostic{name, "discover", err})
			continue
		}
		adapters := make([]tools.Tool, 0, len(remotes))
		seen := map[string]bool{}
		conflict := false
		for _, remote := range remotes {
			adapter := NewRemoteToolAdapter(name, remote, client, logger)
			toolName := adapter.Metadata().Name
			if seen[toolName] {
				conflict = true
				break
			}
			seen[toolName] = true
			if _, ok := registry.Get(toolName); ok {
				conflict = true
				break
			}
			adapters = append(adapters, adapter)
		}
		if conflict {
			_ = client.Close(ctx)
			diagnostics = m.report(diagnostics, Diagnostic{name, "register", errors.New("tool name conflict")})
			continue
		}
		for _, adapter := range adapters {
			if err := registry.Register(adapter); err != nil {
				conflict = true
				break
			}
			meta := adapter.Metadata()
			logger.Info("MCP tool registered", logging.Fields{"stage": "register", "status": "registered", "remote_tool": remoteName(meta.Name), "tool": meta.Name})
		}
		if conflict {
			_ = client.Close(ctx)
			diagnostics = m.report(diagnostics, Diagnostic{name, "register", errors.New("tool registration failed")})
			continue
		}
		m.clients[name] = client
		m.order = append(m.order, name)
	}
	return diagnostics
}
func (m *Manager) report(all []Diagnostic, d Diagnostic) []Diagnostic {
	if m.reporter != nil {
		m.reporter(d)
	}
	return append(all, d)
}
func (m *Manager) Close(ctx context.Context) error {
	var errs []error
	for i := len(m.order) - 1; i >= 0; i-- {
		name := m.order[i]
		logger := m.logger.WithFields(logging.Fields{"server": name})
		logger.Info("MCP server close started", logging.Fields{"stage": "close", "status": "started"})
		if err := m.clients[name].Close(ctx); err != nil {
			logger.Error("MCP server close failed", logging.Fields{"stage": "close", "status": "close_failed"})
			errs = append(errs, err)
		} else {
			logger.Info("MCP server closed", logging.Fields{"stage": "close", "status": "closed"})
		}
	}
	return errors.Join(errs...)
}

func normalizedLogger(loggers []*logging.Logger) *logging.Logger {
	if len(loggers) == 0 || loggers[0] == nil {
		return logging.Nop()
	}
	return loggers[0]
}

func remoteName(tool string) string {
	for i := len(tool) - 1; i > 0; i-- {
		if tool[i-1:i+1] == "__" {
			return tool[i+1:]
		}
	}
	return tool
}
