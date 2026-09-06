package provider

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type BlockType string

const (
	BlockText            BlockType = "text"
	BlockThinking        BlockType = "thinking"
	BlockToolCall        BlockType = "tool_call"
	BlockToolResult      BlockType = "tool_result"
	BlockProviderHistory BlockType = "provider_history"
)

type ContentBlock struct {
	Type            BlockType
	Text            string
	Signature       string
	ToolCall        *ToolCall
	ToolResult      *ToolResult
	ProviderHistory *ProviderHistory
}

type Message struct {
	Role   Role
	Blocks []ContentBlock
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments []byte
}

type ToolResult struct {
	CallID  string
	Name    string
	Content string
	IsError bool
}

type ProviderHistory struct {
	Provider string
	Payload  []byte
}

func CloneMessage(m Message) Message {
	m.Blocks = append([]ContentBlock(nil), m.Blocks...)
	for i := range m.Blocks {
		if m.Blocks[i].ToolCall != nil {
			call := *m.Blocks[i].ToolCall
			call.Arguments = append([]byte(nil), call.Arguments...)
			m.Blocks[i].ToolCall = &call
		}
		if m.Blocks[i].ToolResult != nil {
			result := *m.Blocks[i].ToolResult
			m.Blocks[i].ToolResult = &result
		}
		if m.Blocks[i].ProviderHistory != nil {
			history := *m.Blocks[i].ProviderHistory
			history.Payload = append([]byte(nil), history.Payload...)
			m.Blocks[i].ProviderHistory = &history
		}
	}
	return m
}

func CloneMessages(messages []Message) []Message {
	result := make([]Message, len(messages))
	for i, message := range messages {
		result[i] = CloneMessage(message)
	}
	return result
}
