package sse

import (
	"io"
	"strings"
	"testing"
)

func TestDecoder(t *testing.T) {
	d := NewDecoder(strings.NewReader(": hi\r\nevent: delta\r\nid: 7\r\ndata: 你\r\ndata: 好\r\nunknown: x\r\n\r\n"), 1024)
	e, err := d.Next()
	if err != nil {
		t.Fatal(err)
	}
	if e.Event != "delta" || e.ID != "7" || string(e.Data) != "你\n好" {
		t.Fatalf("%+v", e)
	}
	if _, err = d.Next(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestDecoderFinalFrameAndLimit(t *testing.T) {
	d := NewDecoder(strings.NewReader("data: final"), 100)
	e, err := d.Next()
	if err != nil || string(e.Data) != "final" {
		t.Fatalf("event=%+v err=%v", e, err)
	}
	d = NewDecoder(strings.NewReader("data: "+strings.Repeat("x", 20)+"\n\n"), 10)
	if _, err := d.Next(); err == nil {
		t.Fatal("expected size error")
	}
}

func TestDecoderSkipsEmpty(t *testing.T) {
	d := NewDecoder(strings.NewReader("\n: ping\n\ndata: ok\n\n"), 100)
	e, err := d.Next()
	if err != nil || string(e.Data) != "ok" {
		t.Fatalf("event=%+v err=%v", e, err)
	}
}
