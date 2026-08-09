package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

var ErrSessionClosed = errors.New("mcp session is closed")

type requestResult struct {
	response Response
	err      error
}

type Session struct {
	transport Transport

	mu        sync.Mutex
	nextID    uint64
	pending   map[uint64]chan requestResult
	closed    bool
	closeErr  error
	closeOnce sync.Once
}

func NewSession(transport Transport) *Session {
	session := &Session{transport: transport, pending: make(map[uint64]chan requestResult)}
	go session.receiveLoop()
	return session
}

func (s *Session) Request(ctx context.Context, method string, params any, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		err := s.closeErr
		s.mu.Unlock()
		if err != nil {
			return err
		}
		return ErrSessionClosed
	}
	s.nextID++
	id := s.nextID
	waiter := make(chan requestResult, 1)
	s.pending[id] = waiter
	s.mu.Unlock()
	defer s.removePending(id, waiter)

	message, err := EncodeRequest(id, method, params)
	if err != nil {
		return err
	}
	if err := s.transport.Send(ctx, message); err != nil {
		return fmt.Errorf("mcp: send request: %w", err)
	}
	select {
	case received := <-waiter:
		if received.err != nil {
			return received.err
		}
		if received.response.Error != nil {
			return fmt.Errorf("mcp: JSON-RPC error %d: %s", received.response.Error.Code, received.response.Error.Message)
		}
		if result == nil {
			return nil
		}
		if err := json.Unmarshal(received.response.Result, result); err != nil {
			return fmt.Errorf("mcp: decode result: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) Notify(ctx context.Context, method string, params any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	closed := s.closed
	err := s.closeErr
	s.mu.Unlock()
	if closed {
		if err != nil {
			return err
		}
		return ErrSessionClosed
	}
	message, err := EncodeNotification(method, params)
	if err != nil {
		return err
	}
	if err := s.transport.Send(ctx, message); err != nil {
		return fmt.Errorf("mcp: send notification: %w", err)
	}
	return nil
}

func (s *Session) Close(ctx context.Context) error {
	var closeErr error
	s.closeOnce.Do(func() {
		closeErr = s.transport.Close(ctx)
		s.failAll(ErrSessionClosed)
	})
	return closeErr
}

func (s *Session) receiveLoop() {
	for inbound := range s.transport.Receive() {
		if inbound.Err != nil {
			s.failAll(fmt.Errorf("mcp transport: %w", inbound.Err))
			return
		}
		response, err := DecodeResponse(inbound.Message)
		if err != nil {
			s.failAll(err)
			return
		}
		s.mu.Lock()
		waiter := s.pending[response.ID]
		s.mu.Unlock()
		if waiter != nil {
			waiter <- requestResult{response: response}
		}
	}
	s.failAll(ErrSessionClosed)
}

func (s *Session) removePending(id uint64, waiter chan requestResult) {
	s.mu.Lock()
	if s.pending[id] == waiter {
		delete(s.pending, id)
	}
	s.mu.Unlock()
}

func (s *Session) failAll(err error) {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.closeErr = err
	}
	pending := s.pending
	s.pending = make(map[uint64]chan requestResult)
	s.mu.Unlock()
	for _, waiter := range pending {
		waiter <- requestResult{err: err}
	}
}
