package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestMemoryPathsMapKindsToTheirStorageLevel(t *testing.T) {
	paths := NewPaths(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "workspace"))

	tests := []struct {
		kind MemoryKind
		want string
	}{
		{MemoryUser, paths.UserMemory},
		{MemoryFeedback, paths.UserMemory},
		{MemoryProject, paths.ProjectMemory},
		{MemoryReference, paths.ProjectMemory},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			got, err := MemoryDirectory(paths, test.kind)
			if err != nil {
				t.Fatalf("MemoryDirectory() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("MemoryDirectory() = %q, want %q", got, test.want)
			}
		})
	}

	if _, err := MemoryDirectory(paths, MemoryKind("other")); err == nil {
		t.Fatal("MemoryDirectory() accepted an unknown kind")
	}
}

func TestLoadIndexesLimitsLinesAndBytes(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, filepath.Join(paths.UserMemory, "MEMORY.md"), strings.Repeat("line\n", maxIndexLines+1))
	writeFile(t, filepath.Join(paths.ProjectMemory, "MEMORY.md"), strings.Repeat("x", maxIndexBytes+1))

	indexes, err := LoadIndexes(paths)
	if err != nil {
		t.Fatalf("LoadIndexes() error = %v", err)
	}
	if len(indexes) != 2 {
		t.Fatalf("LoadIndexes() returned %d indexes, want 2", len(indexes))
	}
	if got := strings.Count(indexes[0], "line\n"); got != maxIndexLines {
		t.Fatalf("line-limited index has %d content lines, want %d", got, maxIndexLines)
	}
	if !strings.Contains(indexes[0], indexTruncationNotice) {
		t.Fatalf("line-limited index missing truncation notice: %q", indexes[0])
	}
	if len([]byte(indexes[1])) <= maxIndexBytes || !strings.Contains(indexes[1], indexTruncationNotice) {
		t.Fatalf("byte-limited index did not retain content and notice: %d bytes", len([]byte(indexes[1])))
	}
}

func TestLoadIndexesTreatsMissingIndexesAsEmpty(t *testing.T) {
	indexes, err := LoadIndexes(testPaths(t))
	if err != nil {
		t.Fatalf("LoadIndexes() error = %v", err)
	}
	if len(indexes) != 2 || indexes[0] != "" || indexes[1] != "" {
		t.Fatalf("LoadIndexes() = %#v, want two empty indexes", indexes)
	}
}

func TestApplyOperationsWritesFrontmatterAndUpdatesIndex(t *testing.T) {
	paths := testPaths(t)
	operations, err := ParseOperations([]byte(`[{"action":"create","kind":"user","name":"prefers-any","description":"Uses any in generic code","content":"Use any instead of interface{}."}]`))
	if err != nil {
		t.Fatalf("ParseOperations() error = %v", err)
	}

	if err := ApplyOperations(paths, operations); err != nil {
		t.Fatalf("ApplyOperations() error = %v", err)
	}

	memoryPath := filepath.Join(paths.UserMemory, "prefers-any.md")
	content, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", memoryPath, err)
	}
	for _, want := range []string{
		"---\nname: prefers-any\ndescription: Uses any in generic code\ntype: user\n---",
		"Use any instead of interface{}.",
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("memory content missing %q: %q", want, content)
		}
	}
	index, err := os.ReadFile(filepath.Join(paths.UserMemory, "MEMORY.md"))
	if err != nil {
		t.Fatalf("ReadFile(index): %v", err)
	}
	if !strings.Contains(string(index), "[prefers-any](prefers-any.md)") {
		t.Fatalf("index missing memory reference: %q", index)
	}
}

func TestApplyOperationsRejectsInvalidInputWithoutWriting(t *testing.T) {
	paths := testPaths(t)
	writeFile(t, filepath.Join(paths.UserMemory, "MEMORY.md"), "existing\n")

	for _, input := range [][]byte{
		[]byte(`not json`),
		[]byte(`[] []`),
		[]byte(`[{"action":"create","kind":"unknown","name":"valid-name","description":"valid","content":"body"}]`),
		[]byte(`[{"action":"create","kind":"user","name":"../escape","description":"valid","content":"body"}]`),
		[]byte(`[{"action":"create","kind":"user","name":"valid-name","description":"bad\nvalue","content":"body"}]`),
	} {
		operations, err := ParseOperations(input)
		if err == nil {
			err = ApplyOperations(paths, operations)
		}
		if err == nil {
			t.Fatalf("invalid input %q was accepted", input)
		}
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(paths.UserMemory), "escape.md")); !os.IsNotExist(err) {
		t.Fatalf("invalid operation wrote outside memory directory: %v", err)
	}
	index, err := os.ReadFile(filepath.Join(paths.UserMemory, "MEMORY.md"))
	if err != nil || string(index) != "existing\n" {
		t.Fatalf("invalid operation changed index: %q, %v", index, err)
	}
}

func TestApplyOperationsNoopDoesNotWrite(t *testing.T) {
	paths := testPaths(t)
	operations, err := ParseOperations([]byte(`[{"action":"noop"}]`))
	if err != nil {
		t.Fatalf("ParseOperations() error = %v", err)
	}
	if err := ApplyOperations(paths, operations); err != nil {
		t.Fatalf("ApplyOperations() error = %v", err)
	}
	if _, err := os.Stat(paths.UserMemory); !os.IsNotExist(err) {
		t.Fatalf("noop created a memory directory: %v", err)
	}
	if _, err := os.Stat(paths.ProjectMemory); !os.IsNotExist(err) {
		t.Fatalf("noop created a memory directory: %v", err)
	}
}

func TestMemoryServiceExtractCreatesMemoryWithoutTools(t *testing.T) {
	paths := testPaths(t)
	caller := &memoryTestCaller{response: `[{"action":"create","kind":"user","name":"prefers-tests","description":"Prefers test-first changes","content":"Write the failing test before implementation."}]`}
	service := NewService(paths, ServiceOptions{Caller: caller, Clock: fixedMemoryClock{now: time.Unix(1, 0).UTC()}})

	transcript := []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "Please add tests first."}}}}
	if err := service.Extract(context.Background(), ModeAct, transcript); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if caller.calls != 1 {
		t.Fatalf("caller invoked %d times, want 1", caller.calls)
	}
	if len(caller.request.Tools) != 0 {
		t.Fatalf("Extract request includes tools: %#v", caller.request.Tools)
	}
	if _, err := os.Stat(filepath.Join(paths.UserMemory, "prefers-tests.md")); err != nil {
		t.Fatalf("Extract() did not create memory file: %v", err)
	}
}

func TestMemoryServiceExtractAppliesCreateUpdateDeleteAndNoop(t *testing.T) {
	paths := testPaths(t)
	operations := `[
		{"action":"create","kind":"user","name":"preference","description":"Initial preference","content":"initial"},
		{"action":"update","kind":"user","name":"preference","description":"Updated preference","content":"updated"},
		{"action":"create","kind":"project","name":"obsolete","description":"Old project fact","content":"old"},
		{"action":"delete","kind":"project","name":"obsolete"},
		{"action":"noop"}
	]`
	service := NewService(paths, ServiceOptions{Caller: &memoryTestCaller{response: operations}, Clock: fixedMemoryClock{now: time.Unix(1, 0).UTC()}})

	if err := service.Extract(context.Background(), ModePlan, nil); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(paths.UserMemory, "preference.md"))
	if err != nil || !strings.Contains(string(content), "updated") || strings.Contains(string(content), "initial\n") {
		t.Fatalf("updated memory content = %q, error = %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(paths.ProjectMemory, "obsolete.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted memory remains: %v", err)
	}
}

func TestMemoryServiceExtractRejectsInvalidResponseWithoutWriting(t *testing.T) {
	paths := testPaths(t)
	service := NewService(paths, ServiceOptions{Caller: &memoryTestCaller{response: `[{"action":"create","kind":"other","name":"bad","description":"bad","content":"bad"}]`}})

	if err := service.Extract(context.Background(), ModeAct, nil); err == nil {
		t.Fatal("Extract() accepted invalid provider response")
	}
	if _, err := os.Stat(paths.UserMemory); !os.IsNotExist(err) {
		t.Fatalf("invalid response wrote user memory: %v", err)
	}
}

func TestMemoryServiceMaybeConsolidateHonorsGatesAndWritesOnlyTargetDirectory(t *testing.T) {
	paths := testPaths(t)
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	writeFile(t, filepath.Join(paths.UserMemory, "MEMORY.md"), "# user\n")
	caller := &memoryTestCaller{response: `[{"action":"create","kind":"user","name":"consolidated","description":"Consolidated preference","content":"value"}]`}
	sessions := &memoryTestSessions{items: make([]SessionMeta, 5)}
	service := NewService(paths, ServiceOptions{Caller: caller, Clock: fixedMemoryClock{now: now}, Sessions: sessions})

	if err := service.MaybeConsolidate(context.Background()); err != nil {
		t.Fatalf("MaybeConsolidate() error = %v", err)
	}
	if caller.calls != 1 || sessions.calls != 1 {
		t.Fatalf("calls = provider %d, sessions %d; want 1, 1", caller.calls, sessions.calls)
	}
	if len(caller.request.Tools) != 0 {
		t.Fatalf("governance request includes tools: %#v", caller.request.Tools)
	}
	if _, err := os.Stat(filepath.Join(paths.UserMemory, "consolidated.md")); err != nil {
		t.Fatalf("governance did not write target memory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.ProjectMemory, "consolidated.md")); !os.IsNotExist(err) {
		t.Fatalf("governance wrote outside target memory: %v", err)
	}

	if err := service.MaybeConsolidate(context.Background()); err != nil {
		t.Fatalf("MaybeConsolidate() throttled call error = %v", err)
	}
	if caller.calls != 1 || sessions.calls != 1 {
		t.Fatalf("throttled call invoked dependencies: provider %d, sessions %d", caller.calls, sessions.calls)
	}
}

func TestMemoryServiceMaybeConsolidateSkipsRecentAndLockedDirectoriesAndExpiresOldLock(t *testing.T) {
	paths := testPaths(t)
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	writeFile(t, filepath.Join(paths.UserMemory, "MEMORY.md"), "# user\n")
	caller := &memoryTestCaller{response: `[{"action":"noop"}]`}
	service := NewService(paths, ServiceOptions{Caller: caller, Clock: fixedMemoryClock{now: now}, Sessions: &memoryTestSessions{items: make([]SessionMeta, 5)}})

	writeFile(t, filepath.Join(paths.UserMemory, consolidationMarkerName), "success")
	if err := os.Chtimes(filepath.Join(paths.UserMemory, consolidationMarkerName), now, now); err != nil {
		t.Fatalf("Chtimes(marker): %v", err)
	}
	if err := service.MaybeConsolidate(context.Background()); err != nil {
		t.Fatalf("MaybeConsolidate() recent error = %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("recent consolidation called provider %d times", caller.calls)
	}

	old := now.Add(-25 * time.Hour)
	if err := os.Chtimes(filepath.Join(paths.UserMemory, consolidationMarkerName), old, old); err != nil {
		t.Fatalf("Chtimes(marker): %v", err)
	}
	writeFile(t, filepath.Join(paths.UserMemory, consolidationLockName), "other-pid")
	if err := os.Chtimes(filepath.Join(paths.UserMemory, consolidationLockName), now.Add(-30*time.Minute), now.Add(-30*time.Minute)); err != nil {
		t.Fatalf("Chtimes(lock): %v", err)
	}
	if err := service.MaybeConsolidate(context.Background()); err != nil {
		t.Fatalf("MaybeConsolidate() locked error = %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("active lock called provider %d times", caller.calls)
	}

	stale := now.Add(-consolidationLockExpiry - time.Minute)
	if err := os.Chtimes(filepath.Join(paths.UserMemory, consolidationLockName), stale, stale); err != nil {
		t.Fatalf("Chtimes(lock): %v", err)
	}
	service = NewService(paths, ServiceOptions{Caller: caller, Clock: fixedMemoryClock{now: now.Add(consolidationScanDelay)}, Sessions: &memoryTestSessions{items: make([]SessionMeta, 5)}})
	if err := service.MaybeConsolidate(context.Background()); err != nil {
		t.Fatalf("MaybeConsolidate() stale lock error = %v", err)
	}
	if caller.calls != 1 {
		t.Fatalf("stale lock did not permit consolidation, provider calls = %d", caller.calls)
	}
}

type memoryTestCaller struct {
	response string
	err      error
	request  provider.ChatRequest
	calls    int
}

func (c *memoryTestCaller) Call(_ context.Context, request provider.ChatRequest) (string, error) {
	c.calls++
	c.request = request
	return c.response, c.err
}

type fixedMemoryClock struct{ now time.Time }

func (c fixedMemoryClock) Now() time.Time { return c.now }

type memoryTestSessions struct {
	items []SessionMeta
	err   error
	calls int
}

func (s *memoryTestSessions) List() ([]SessionMeta, error) {
	s.calls++
	return s.items, s.err
}

func testPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	return NewPaths(filepath.Join(root, "config"), filepath.Join(root, "workspace"))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
