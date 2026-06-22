package sse

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
)

const DefaultMaxEventBytes = 1 << 20

type SSEEvent struct {
	Event string
	Data  []byte
	ID    string
}

type Decoder interface{ Next() (SSEEvent, error) }

type decoder struct {
	r   *bufio.Reader
	max int
}

func NewDecoder(r io.Reader, maxEventBytes int) Decoder {
	if maxEventBytes <= 0 {
		maxEventBytes = DefaultMaxEventBytes
	}
	return &decoder{r: bufio.NewReader(r), max: maxEventBytes}
}

func (d *decoder) Next() (SSEEvent, error) {
	for {
		var event SSEEvent
		var data [][]byte
		size, seen := 0, false
		for {
			line, err := d.r.ReadString('\n')
			size += len(line)
			if size > d.max {
				return SSEEvent{}, fmt.Errorf("SSE event exceeds %d bytes", d.max)
			}
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if line == "" {
				if seen {
					break
				}
				if err != nil {
					return SSEEvent{}, err
				}
				continue
			}
			if !strings.HasPrefix(line, ":") {
				field, value, _ := strings.Cut(line, ":")
				if strings.HasPrefix(value, " ") {
					value = value[1:]
				}
				switch field {
				case "event":
					event.Event = value
					seen = true
				case "data":
					data = append(data, []byte(value))
					seen = true
				case "id":
					event.ID = value
					seen = true
				}
			}
			if err != nil {
				if err == io.EOF && seen {
					break
				}
				return SSEEvent{}, err
			}
		}
		event.Data = bytes.Join(data, []byte("\n"))
		if seen {
			return event, nil
		}
	}
}
