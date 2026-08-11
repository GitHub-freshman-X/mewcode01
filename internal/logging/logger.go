package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Fields map[string]any

type Event struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Source  string    `json:"source,omitempty"`
	Message string    `json:"message"`
	Fields  Fields    `json:"fields,omitempty"`
}

type DiagnosticRef struct {
	Path          string
	OriginalChars int
	CapturedChars int
	Truncated     bool
}

type Logger struct {
	state *loggerState
	base  Fields
	root  string
}

type loggerState struct {
	mu                 sync.Mutex
	file               *os.File
	now                func() time.Time
	closed             bool
	diagnosticSequence int
}

func New(root string, now func() time.Time, pid int) (*Logger, error) {
	if now == nil {
		now = time.Now
	}
	timestamp := now()
	dir := filepath.Join(root, "logs", timestamp.Local().Format("2006"), timestamp.Local().Format("01"), timestamp.Local().Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	filenameTime := timestamp.UTC()
	name := fmt.Sprintf("mewcode-%s.%09d-%d.jsonl", filenameTime.Format("20060102T150405"), filenameTime.Nanosecond(), pid)
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	return &Logger{state: &loggerState{file: file, now: now}, root: filepath.Clean(root)}, nil
}

func Nop() *Logger { return &Logger{state: &loggerState{now: time.Now}} }

func (l *Logger) WithFields(fields Fields) *Logger {
	if l == nil {
		return Nop()
	}
	l.state.mu.Lock()
	base := cloneFields(l.base)
	l.state.mu.Unlock()
	for key, value := range fields {
		base[key] = value
	}
	return &Logger{state: l.state, base: base, root: l.root}
}

func (l *Logger) Info(message string, fields Fields) {
	l.log("info", message, fields)
}

func (l *Logger) Error(message string, fields Fields) {
	l.log("error", message, fields)
}

func (l *Logger) CaptureDiagnostic(kind, content string, maxChars int) (DiagnosticRef, error) {
	if l == nil || maxChars <= 0 {
		return DiagnosticRef{}, nil
	}
	l.state.mu.Lock()
	defer l.state.mu.Unlock()
	if l.state.closed || l.state.file == nil || l.root == "" {
		return DiagnosticRef{}, nil
	}
	original := []rune(content)
	captured := original
	if len(captured) > maxChars {
		captured = captured[:maxChars]
	}
	timestamp := l.state.now()
	dir := filepath.Join(l.root, "logs", timestamp.Local().Format("2006"), timestamp.Local().Format("01"), timestamp.Local().Format("02"), "diagnostics")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DiagnosticRef{}, err
	}
	l.state.diagnosticSequence++
	name := fmt.Sprintf("%s-%s.%09d-%d.txt", diagnosticName(kind), timestamp.UTC().Format("20060102T150405"), timestamp.UTC().Nanosecond(), l.state.diagnosticSequence)
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return DiagnosticRef{}, err
	}
	if _, err := file.WriteString(string(captured)); err != nil {
		_ = file.Close()
		return DiagnosticRef{}, err
	}
	if err := file.Close(); err != nil {
		return DiagnosticRef{}, err
	}
	rel, err := filepath.Rel(l.root, path)
	if err != nil {
		return DiagnosticRef{}, err
	}
	return DiagnosticRef{Path: filepath.ToSlash(rel), OriginalChars: len(original), CapturedChars: len(captured), Truncated: len(captured) < len(original)}, nil
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.state.mu.Lock()
	defer l.state.mu.Unlock()
	if l.state.closed || l.state.file == nil {
		return nil
	}
	l.state.closed = true
	return l.state.file.Close()
}

func (l *Logger) log(level, message string, fields Fields) {
	if l == nil {
		return
	}
	l.state.mu.Lock()
	defer l.state.mu.Unlock()
	if l.state.closed || l.state.file == nil {
		return
	}
	merged := cloneFields(l.base)
	for key, value := range fields {
		merged[key] = value
	}
	entry := Event{Time: l.state.now(), Level: level, Message: message, Fields: merged, Source: l.source()}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = l.state.file.Write(append(encoded, '\n'))
}

func (l *Logger) source() string {
	for skip := 3; ; skip++ {
		_, file, line, ok := runtime.Caller(skip)
		if !ok {
			return ""
		}
		rel, err := filepath.Rel(l.root, file)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			wd, wdErr := os.Getwd()
			rel, err = filepath.Rel(wd, file)
			if wdErr != nil || err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return ""
			}
		}
		return filepath.ToSlash(rel) + fmt.Sprintf(":%d", line)
	}
}

func cloneFields(fields Fields) Fields {
	cloned := make(Fields, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}

func diagnosticName(kind string) string {
	name := strings.TrimSpace(kind)
	if name == "" {
		return "diagnostic"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if result := strings.Trim(b.String(), "-"); result != "" {
		return result
	}
	return "diagnostic"
}
