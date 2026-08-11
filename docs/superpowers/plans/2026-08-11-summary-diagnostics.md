# Generic Diagnostic Capture for Summary Failures Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make malformed summary responses diagnosable without putting response content in JSONL logs, while accepting only unambiguous duplicate summary wrappers.

**Architecture:** `internal/logging` gains a generic bounded diagnostic-artifact writer with no knowledge of summary compaction. The context parser exposes safe structural metadata on parse errors; the agent captures the raw response only when an opt-in character budget is configured, then logs only the artifact reference and structural metadata.

**Tech Stack:** Go, YAML configuration, JSONL logging, standard library file permissions, existing agent/context tests.

## Global Constraints

- JSONL fields must never contain prompts, raw model responses, tool-result bodies, credentials, request headers, or unredacted provider errors.
- Diagnostic capture is disabled by default with `agent.context.debug_summary_response_chars: 0`.
- A captured diagnostic file is bounded to the configured character count and created with `0600` permissions.
- The parser may accept a nested form only when every outer `<summary>` is a whitespace-only wrapper around exactly one child; it returns the innermost non-empty text.
- Any ambiguous nesting, missing close tag, empty leaf, or malformed wrapper remains a failure and leaves history unchanged.
- Do not stage, overwrite, or commit the user-owned dirty `.mewcode/config.example.yaml`; provide its one-line addition in the chapter documentation instead.

---

## File Structure

- Modify: `internal/logging/logger.go` — generic diagnostic artifact API and safe filename/path generation.
- Modify: `internal/context/compact.go` — structural parse metadata and pure-wrapper handling.
- Modify: `internal/context/config.go` — context runtime diagnostic capture budget.
- Modify: `internal/config/config.go`, `internal/config/load.go`, `internal/config/validate.go` — opt-in YAML configuration and validation.
- Modify: `cmd/mewcode/main.go` — pass the config budget into the context runtime config.
- Modify: `internal/context/context_test.go` — parser compatibility and rejection cases.
- Modify: `internal/config/config_test.go`, `cmd/mewcode/main_test.go` — configuration loading, validation, and mapping.
- Modify: `internal/agent/runner.go` and `internal/agent/runner_test.go` — capture parse-failure response and assert JSONL stays metadata-only.
- Modify: `docs/ch08/summary-parser-tolerance-design.md`, `bugs/2026-08-11/003-ch08-summary-multiple-tags.md` — reflect the completed behavior and operational configuration snippet.

### Task 1: Generic bounded diagnostic artifact writer

**Files:**
- Modify: `internal/logging/logger.go`
- Test: `internal/agent/runner_test.go`

**Interfaces:**
- Produces `logging.DiagnosticRef{Path string, OriginalChars int, CapturedChars int, Truncated bool}`.
- Produces `func (l *Logger) CaptureDiagnostic(kind, content string, maxChars int) (DiagnosticRef, error)`.
- `kind` is generic; the logger validates/sanitizes it only for a diagnostic filename and knows nothing about agent/context behavior.

- [ ] **Step 1: Write the failing runner-level artifact test**

```go
cfg := compactTestConfig()
cfg.DebugSummaryResponseChars = 12
// Provider returns a malformed summary response containing a distinctive payload.
// Assert the failure log has diagnostic_path, captured_chars and truncated,
// but does not contain the distinctive payload; read the referenced file and
// assert it contains exactly the first 12 characters and mode 0600.
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run: `go test ./internal/agent -run TestRunnerCapturesMalformedSummaryDiagnostic -count=1`

Expected: FAIL because no diagnostic path/artifact exists.

- [ ] **Step 3: Implement the minimal generic logger API**

```go
type DiagnosticRef struct {
    Path          string
    OriginalChars int
    CapturedChars int
    Truncated     bool
}

func (l *Logger) CaptureDiagnostic(kind, content string, maxChars int) (DiagnosticRef, error) {
    // return a zero ref without writing when maxChars <= 0 or logger is closed
    // write a 0600 bounded file below logs/YYYY/MM/DD/diagnostics/
    // return metadata only; do not write the content through Event fields
}
```

- [ ] **Step 4: Run the focused test and verify it passes**

Run: `go test ./internal/agent -run TestRunnerCapturesMalformedSummaryDiagnostic -count=1`

Expected: PASS; JSONL contains metadata only and the artifact is bounded with `0600` mode.

- [ ] **Step 5: Commit**

```bash
git add internal/logging/logger.go internal/agent/runner_test.go
git commit -m "feat: add bounded diagnostic capture"
```

### Task 2: Structural summary parser diagnostics and safe wrapper compatibility

**Files:**
- Modify: `internal/context/compact.go`
- Modify: `internal/context/context_test.go`

**Interfaces:**
- Produces `SummaryParseError` with safe `Reason`, `OpenTags`, `CloseTags`, `MaxDepth`, and `TagTrace` fields.
- `ExtractSummary` still returns one `SummaryExtraction`; successful pure-wrapper inputs return only the innermost text.

- [ ] **Step 1: Write failing parser tests**

```go
got, err := ExtractSummary("<summary><summary>final state</summary></summary>")
if err != nil || got.Text != "final state" { t.Fatalf("got=%+v err=%v", got, err) }

_, err = ExtractSummary("<summary>draft<summary>final</summary></summary>")
var parseErr *SummaryParseError
if !errors.As(err, &parseErr) || parseErr.Reason != "summary wrapper contains content" {
    t.Fatalf("err=%v", err)
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run: `go test ./internal/context -run 'TestExtractSummary(PureNestedWrapper|RejectsAmbiguousNestedWrapper)' -count=1`

Expected: FAIL because nested summaries are currently rejected unconditionally.

- [ ] **Step 3: Implement stack/recursive structural parsing**

```go
type SummaryParseError struct {
    Reason string
    OpenTags, CloseTags, MaxDepth int
    TagTrace string
}

// Parse each top-level candidate. A candidate containing exactly one child
// summary and whitespace otherwise resolves to that child. Any other nested
// content returns SummaryParseError rather than selecting or concatenating text.
```

- [ ] **Step 4: Run the focused test and existing parser tests**

Run: `go test ./internal/context -run 'TestExtractSummary|TestBuildSummaryRequest' -count=1`

Expected: PASS; independent repeated blocks still select the final block.

- [ ] **Step 5: Commit**

```bash
git add internal/context/compact.go internal/context/context_test.go
git commit -m "fix: accept pure duplicate summary wrappers"
```

### Task 3: Opt-in configuration and compaction integration

**Files:**
- Modify: `internal/context/config.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/load.go`
- Modify: `internal/config/validate.go`
- Modify: `cmd/mewcode/main.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/mewcode/main_test.go`
- Modify: `internal/agent/runner.go`
- Modify: `internal/agent/runner_test.go`

**Interfaces:**
- YAML field `agent.context.debug_summary_response_chars` maps to `context.Config.DebugSummaryResponseChars`.
- Value `0` disables capture; negative values fail configuration validation.
- On summary parse failure, `Runner.compact` calls `CaptureDiagnostic("context-summary-response", rawResponse, budget)` and emits only safe diagnostic metadata in its existing error event.

- [ ] **Step 1: Write failing config and integration tests**

```go
// config load: debug_summary_response_chars: 256 survives defaults and mapping.
// validation: -1 returns the field-specific negative-budget error.
// runner: malformed response creates a ref only with a positive budget;
// zero budget yields no diagnostic file/ref.
```

- [ ] **Step 2: Run the focused tests and verify they fail**

Run: `go test ./internal/config ./cmd/mewcode ./internal/agent -run 'Test(ContextConfig|AgentContextConfig|RunnerCapturesMalformedSummaryDiagnostic)' -count=1`

Expected: FAIL because the field and capture integration do not exist.

- [ ] **Step 3: Implement the smallest end-to-end mapping**

```go
// Add DebugSummaryResponseChars to config.ContextConfig and context.Config.
// rawContextConfig uses *int so explicit zero remains disabled.
// validate only rejects negative values.
// compact logs: parse_reason, summary_open_tags, summary_close_tags,
// summary_max_depth, summary_tag_trace, diagnostic_path, diagnostic_original_chars,
// diagnostic_captured_chars, diagnostic_truncated.
// No raw response or err.Error() is added to logging.Fields.
```

- [ ] **Step 4: Run focused tests and verify they pass**

Run: `go test ./internal/config ./cmd/mewcode ./internal/agent -run 'Test(ContextConfig|AgentContextConfig|RunnerCapturesMalformedSummaryDiagnostic)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/context/config.go internal/config/config.go internal/config/load.go internal/config/validate.go cmd/mewcode/main.go internal/config/config_test.go cmd/mewcode/main_test.go internal/agent/runner.go internal/agent/runner_test.go
git commit -m "feat: diagnose malformed summary responses"
```

### Task 4: Documentation, bug record, and verification

**Files:**
- Modify: `docs/ch08/summary-parser-tolerance-design.md`
- Modify: `bugs/2026-08-11/003-ch08-summary-multiple-tags.md`

- [ ] **Step 1: Update operational documentation**

```yaml
agent:
  context:
    # 0 disables capture; use a small nonzero value only for troubleshooting.
    debug_summary_response_chars: 4096
```

Document that JSONL records a diagnostic reference only, while the raw bounded body lives in a local `0600` artifact.

- [ ] **Step 2: Update the bug record**

Record the nested-wrapper reproducer, exact parser acceptance rule, safe diagnostic path, and real-API retest command. Keep it “待处理” until the live API retry succeeds.

- [ ] **Step 3: Run full verification**

Run:

```bash
go test ./...
go test -race ./internal/context ./internal/agent
go build -o /private/tmp/mewcode-ch08-diagnostics ./cmd/mewcode
```

Expected: each command exits 0.

- [ ] **Step 4: Commit**

```bash
git add docs/ch08/summary-parser-tolerance-design.md bugs/2026-08-11/003-ch08-summary-multiple-tags.md
git commit -m "docs: document summary diagnostics"
```

## Plan Self-Review

- Coverage: generic logging, safe parser compatibility, configurable opt-in capture, JSONL privacy, documentation, bug record, and real-API handoff each have a task.
- No-placeholder scan: no TBD/TODO markers or unspecified code/test steps remain.
- Type consistency: `DebugSummaryResponseChars`, `DiagnosticRef`, `CaptureDiagnostic`, and `SummaryParseError` use the same names in every task.
