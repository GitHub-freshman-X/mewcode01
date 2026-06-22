package testutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

type Request struct {
	Path   string
	Header http.Header
	Body   []byte
}

type StreamServer struct {
	Server      *httptest.Server
	Status      int
	Frames      []string
	BeforeFrame func(int)
	mu          sync.Mutex
	requests    []Request
}

func NewStreamServer(status int, frames ...string) *StreamServer {
	s := &StreamServer{Status: status, Frames: frames}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	return s
}

func (s *StreamServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	s.mu.Lock()
	s.requests = append(s.requests, Request{Path: r.URL.Path, Header: r.Header.Clone(), Body: body})
	s.mu.Unlock()
	status := s.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(status)
	for i, frame := range s.Frames {
		if s.BeforeFrame != nil {
			s.BeforeFrame(i)
		}
		_, _ = io.WriteString(w, frame)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

func (s *StreamServer) URL() string          { return s.Server.URL }
func (s *StreamServer) Client() *http.Client { return s.Server.Client() }
func (s *StreamServer) Close()               { s.Server.Close() }
func (s *StreamServer) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	copy(out, s.requests)
	return out
}
