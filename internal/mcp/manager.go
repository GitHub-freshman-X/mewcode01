package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/GitHub-freshman-X/mewcode01/internal/config"
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
	clients    map[string]*Client
	order      []string
}

func NewManager(httpClient *http.Client, reporter Reporter) *Manager {
	return &Manager{httpClient: httpClient, reporter: reporter, clients: map[string]*Client{}}
}
func (m *Manager) ConnectAndRegister(ctx context.Context, registry *tools.Registry, servers map[string]config.MCPServerConfig) []Diagnostic {
	names := make([]string, 0, len(servers))
	for n := range servers {
		names = append(names, n)
	}
	sort.Strings(names)
	var diagnostics []Diagnostic
	for _, name := range names {
		raw := servers[name]
		server, err := ExpandServer(name, raw, nil)
		if err != nil {
			diagnostics = m.report(diagnostics, Diagnostic{name, "configuration", err})
			continue
		}
		var transport Transport
		switch server.Type {
		case config.MCPTransportStdio:
			st := NewStdioTransport(server.Command, server.Args, server.Env)
			if err := st.Start(ctx); err != nil {
				diagnostics = m.report(diagnostics, Diagnostic{name, "connect", err})
				continue
			}
			transport = st
		case config.MCPTransportHTTP:
			transport = NewHTTPTransport(server.URL, server.Headers, m.httpClient)
		default:
			diagnostics = m.report(diagnostics, Diagnostic{name, "configuration", errors.New("unsupported transport")})
			continue
		}
		client := NewClient(transport)
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
			adapter := NewRemoteToolAdapter(name, remote, client)
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
		if err := m.clients[m.order[i]].Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
