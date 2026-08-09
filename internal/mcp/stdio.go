package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

type StdioTransport struct {
	command string
	args    []string
	env     map[string]string

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	inbound chan Inbound
	closed  bool
}

func NewStdioTransport(command string, args []string, env map[string]string) *StdioTransport {
	return &StdioTransport{command: command, args: append([]string(nil), args...), env: cloneStrings(env), inbound: make(chan Inbound, 16)}
}

func (t *StdioTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cmd != nil {
		return nil
	}
	if t.command == "" {
		return errors.New("mcp stdio command is required")
	}
	cmd := exec.CommandContext(ctx, t.command, t.args...)
	if len(t.env) > 0 {
		cmd.Env = os.Environ()
		for key, value := range t.env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp stdio stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp stdio stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("mcp stdio stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcp stdio start: %w", err)
	}
	t.cmd, t.stdin = cmd, stdin
	go t.read(stdout)
	go func() { _, _ = io.Copy(io.Discard, io.LimitReader(stderr, 64<<10)) }()
	go func() { _ = cmd.Wait(); t.publish(Inbound{Err: ErrSessionClosed}) }()
	return nil
}

func (t *StdioTransport) Send(ctx context.Context, message []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stdin == nil || t.closed {
		return ErrSessionClosed
	}
	_, err := t.stdin.Write(append(append([]byte(nil), message...), '\n'))
	return err
}

func (t *StdioTransport) Receive() <-chan Inbound { return t.inbound }

func (t *StdioTransport) Close(ctx context.Context) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	cmd, stdin := t.cmd, t.stdin
	t.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return ctx.Err()
}

func (t *StdioTransport) read(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		t.publish(Inbound{Message: append([]byte(nil), scanner.Bytes()...)})
	}
	if err := scanner.Err(); err != nil {
		t.publish(Inbound{Err: fmt.Errorf("mcp stdio read: %w", err)})
	}
}

func (t *StdioTransport) publish(inbound Inbound) {
	select {
	case t.inbound <- inbound:
	default:
	}
}
