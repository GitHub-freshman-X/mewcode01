package mcp

import (
	"context"
	"testing"
	"time"
)

func TestStdioTransportSendsAndReceivesJSONLines(t *testing.T) {
	transport := NewStdioTransport("cat", nil, nil)
	if err := transport.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer transport.Close(context.Background())
	if err := transport.Send(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case inbound := <-transport.Receive():
		if inbound.Err != nil || string(inbound.Message) != `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` {
			t.Fatalf("inbound=%+v", inbound)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stdout line")
	}
}
