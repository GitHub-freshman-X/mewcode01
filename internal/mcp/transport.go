package mcp

import "context"

type Inbound struct {
	Message []byte
	Err     error
}

type Transport interface {
	Send(context.Context, []byte) error
	Receive() <-chan Inbound
	Close(context.Context) error
}
