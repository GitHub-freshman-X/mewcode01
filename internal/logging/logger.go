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
	Time      time.Time `json:"time"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Event     string    `json:"event"`
	Message   string    `json:"message"`
	Fields    Fields    `json:"fields,omitempty"`
	Source    string    `json:"source,omitempty"`
}

type Logger struct {
	state *loggerState
	base  Fields
	root  string
}

type loggerState struct {
	mu     sync.Mutex
	file   *os.File
	now    func() time.Time
	closed bool
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

func (l *Logger) Info(component, event, message string, fields Fields) {
	l.log("info", component, event, message, fields)
}

func (l *Logger) Error(component, event, message string, fields Fields) {
	l.log("error", component, event, message, fields)
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

func (l *Logger) log(level, component, name, message string, fields Fields) {
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
	entry := Event{Time: l.state.now(), Level: level, Component: component, Event: name, Message: message, Fields: merged, Source: l.source()}
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
