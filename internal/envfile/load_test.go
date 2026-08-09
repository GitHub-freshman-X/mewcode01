package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSetsMissingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("# comment\n\nMCP_TEST_ROOT=/tmp/fixtures\n"), 0600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	result, err := Load(path, func(key string) (string, bool) { value, ok := values[key]; return value, ok }, func(key, value string) error { values[key] = value; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || values["MCP_TEST_ROOT"] != "/tmp/fixtures" || len(result.Loaded) != 1 || result.Loaded[0] != "MCP_TEST_ROOT" {
		t.Fatalf("result=%+v values=%v", result, values)
	}
}

func TestLoadSkipsExistingValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("MCP_TEST_ROOT=from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	result, err := Load(path, func(string) (string, bool) { return "system", true }, func(string, string) error { called = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if called || len(result.Skipped) != 1 || result.Skipped[0] != "MCP_TEST_ROOT" {
		t.Fatalf("result=%+v called=%v", result, called)
	}
}
