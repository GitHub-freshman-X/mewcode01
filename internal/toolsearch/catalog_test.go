package toolsearch

import (
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestCatalogResolveAcceptsLocalAndRemoteNames(t *testing.T) {
	catalog := New([]provider.ToolDefinition{{Name: "demo__lookup_fixture", MCPServer: "demo", RemoteName: "lookup_fixture", Description: "fixture", Schema: map[string]any{"type": "object"}}})
	for _, name := range []string{"demo__lookup_fixture", "lookup_fixture"} {
		got, err := catalog.Resolve("demo", name)
		if err != nil || got.Name != "demo__lookup_fixture" {
			t.Fatalf("Resolve(%q) = %+v, %v", name, got, err)
		}
	}
	if _, err := catalog.Resolve("other", "lookup_fixture"); err == nil {
		t.Fatal("Resolve accepted a tool from another server")
	}
}

func TestCatalogLoadRequiresExactDirectoryName(t *testing.T) {
	catalog := New([]provider.ToolDefinition{{Name: "context7__resolve-library-id", MCPServer: "context7", RemoteName: "resolve-library-id", Description: "Resolve documentation library ID", Schema: map[string]any{"type": "object"}}})
	got, err := catalog.Load("context7__resolve-library-id")
	if err != nil || got.Name != "context7__resolve-library-id" {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
	for _, invalid := range []string{"resolve-library-id", "select:context7__resolve-library-id", "documentation library ID"} {
		if _, err := catalog.Load(invalid); err == nil {
			t.Fatalf("Load(%q) unexpectedly succeeded", invalid)
		}
	}
}
