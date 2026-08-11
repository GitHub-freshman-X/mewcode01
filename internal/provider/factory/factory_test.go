package factory

import (
	"github.com/GitHub-freshman-X/mewcode01/internal/config"
	"net/http"
	"testing"
)

func TestNew(t *testing.T) {
	for _, protocol := range []config.Protocol{config.ProtocolAnthropic, config.ProtocolOpenAI} {
		if p, err := New(config.Config{Protocol: protocol, BaseURL: "http://localhost", APIKey: "x", Model: "x"}, &http.Client{}, nil); err != nil || p == nil {
			t.Fatalf("protocol=%s p=%v err=%v", protocol, p, err)
		}
	}
	if _, err := New(config.Config{Protocol: "other", BaseURL: "http://localhost"}, nil, nil); err == nil {
		t.Fatal("expected error")
	}
}
