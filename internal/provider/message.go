package provider

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type BlockType string

const (
	BlockText     BlockType = "text"
	BlockThinking BlockType = "thinking"
)

type ContentBlock struct {
	Type      BlockType
	Text      string
	Signature string
}

type Message struct {
	Role   Role
	Blocks []ContentBlock
}

func CloneMessage(m Message) Message {
	m.Blocks = append([]ContentBlock(nil), m.Blocks...)
	return m
}

func CloneMessages(messages []Message) []Message {
	result := make([]Message, len(messages))
	for i, message := range messages {
		result[i] = CloneMessage(message)
	}
	return result
}
