package conversation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type JournalPurpose string

const (
	JournalPurposeHistory JournalPurpose = "history"
	JournalPurposePlan    JournalPurpose = "plan"
	JournalPurposeUsage   JournalPurpose = "usage"
)

type ToolUseRecord struct {
	ID        string          `json:"tool_use_id"`
	Name      string          `json:"tool_name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolResultRecord struct {
	CallID  string `json:"tool_use_id"`
	Name    string `json:"tool_name"`
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

type UsageRecord struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type JournalRecord struct {
	Role        provider.Role      `json:"role"`
	Content     string             `json:"content"`
	ToolUses    []ToolUseRecord    `json:"tool_uses,omitempty"`
	ToolResults []ToolResultRecord `json:"tool_results,omitempty"`
	Purpose     JournalPurpose     `json:"purpose"`
	Usage       *UsageRecord       `json:"usage,omitempty"`
	Timestamp   int64              `json:"ts"`
}

type Journal interface {
	Append(messages []provider.Message, purpose JournalPurpose) error
}

type UsageJournal interface {
	AppendUsage(provider.Usage) error
}

type JSONLJournal struct {
	mu     sync.Mutex
	writer io.Writer
	now    func() time.Time
}

func NewJSONLJournal(writer io.Writer) *JSONLJournal {
	return &JSONLJournal{writer: writer, now: time.Now}
}

func (j *JSONLJournal) Append(messages []provider.Message, purpose JournalPurpose) error {
	if j.writer == nil {
		return errors.New("journal writer is nil")
	}
	if purpose != JournalPurposeHistory && purpose != JournalPurposePlan {
		return errors.New("journal purpose is invalid")
	}

	var output bytes.Buffer
	timestamp := j.now().Unix()
	for _, message := range messages {
		record := JournalRecord{
			Role:      message.Role,
			Purpose:   purpose,
			Timestamp: timestamp,
		}
		for _, block := range message.Blocks {
			switch block.Type {
			case provider.BlockText:
				record.Content += block.Text
			case provider.BlockToolCall:
				if block.ToolCall != nil {
					record.ToolUses = append(record.ToolUses, ToolUseRecord{
						ID:        block.ToolCall.ID,
						Name:      block.ToolCall.Name,
						Arguments: json.RawMessage(block.ToolCall.Arguments),
					})
				}
			case provider.BlockToolResult:
				if block.ToolResult != nil {
					record.ToolResults = append(record.ToolResults, ToolResultRecord{
						CallID:  block.ToolResult.CallID,
						Name:    block.ToolResult.Name,
						Content: block.ToolResult.Content,
						IsError: block.ToolResult.IsError,
					})
				}
			}
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		output.Write(encoded)
		output.WriteByte('\n')
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	_, err := j.writer.Write(output.Bytes())
	return err
}

func (j *JSONLJournal) AppendUsage(usage provider.Usage) error {
	if j.writer == nil {
		return errors.New("journal writer is nil")
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 {
		return errors.New("usage tokens must be non-negative")
	}
	record := JournalRecord{Purpose: JournalPurposeUsage, Usage: &UsageRecord{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens}, Timestamp: j.now().Unix()}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	_, err = j.writer.Write(append(encoded, '\n'))
	return err
}
