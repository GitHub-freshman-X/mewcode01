package context

import (
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

const summarySystemPrompt = `You are compressing an agent conversation for future continuation.

Do not call tools. Do not ask the user for missing information. The last user message is an internal compression control instruction, not conversation history: do not quote, paraphrase, list, or include it in any summary section. Preserve facts, constraints, file paths, commands, decisions, and verification state from messages before it only. Use the conversation content only. discard analysis drafts and return only one non-empty <summary>...</summary> block. Return no explanation, Markdown, analysis, or text outside the block. Do not emit a second <summary> tag and do not write the literal <summary> tag inside its contents.

The summary must use these nine sections:
1. User Request / 主要请求与意图
2. Current Goal / 关键技术概念
3. Important Context / 文件和代码
4. Decisions / 错误和修复
5. Progress / 问题解决过程
6. Open Tasks / 所有用户原话
7. Files and Symbols / 待办任务
8. Verification / 当前工作
9. Risks / 可能的下一步`

type SummaryExtraction struct {
	Text           string
	CandidateCount int
	UsedLast       bool
}

type SummaryParseError struct {
	Reason    string
	OpenTags  int
	CloseTags int
	MaxDepth  int
	TagTrace  string
}

func (e *SummaryParseError) Error() string {
	return e.Reason
}

func (m *Manager) BuildSummaryRequest(messages []provider.Message, trigger Trigger) provider.ChatRequest {
	requestMessages := provider.CloneMessages(messages)
	requestMessages = append(requestMessages, provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "Internal compression control instruction for trigger " + string(trigger) + ". Follow the system-defined compression format for the preceding conversation."}}})
	return provider.ChatRequest{
		Prompt: provider.PromptBundle{
			StableSystem: summarySystemPrompt,
		},
		Messages:  requestMessages,
		MaxTokens: m.Config.SummaryOutputTokens,
	}
}

func ExtractSummary(text string) (SummaryExtraction, error) {
	const open = "<summary>"
	cursor := 0
	summary := ""
	candidates := 0
	tracker := summaryTagTracker{}
	for {
		startOffset := strings.Index(text[cursor:], open)
		if startOffset < 0 {
			break
		}
		start := cursor + startOffset
		tracker.open(1)
		body, end, err := parseSummaryCandidate(text, start+len(open), 1, &tracker)
		if err != nil {
			return SummaryExtraction{}, err
		}
		summary = body
		candidates++
		cursor = end
	}
	if candidates == 0 {
		return SummaryExtraction{}, tracker.err("summary tag is missing")
	}
	return SummaryExtraction{Text: summary, CandidateCount: candidates, UsedLast: candidates > 1}, nil
}

func parseSummaryCandidate(text string, cursor, depth int, tracker *summaryTagTracker) (string, int, error) {
	const open = "<summary>"
	const close = "</summary>"
	childCount := 0
	childSummary := ""
	for {
		nextOpen := strings.Index(text[cursor:], open)
		nextClose := strings.Index(text[cursor:], close)
		if nextClose < 0 {
			return "", 0, tracker.err("summary closing tag is missing")
		}
		if nextOpen >= 0 && nextOpen < nextClose {
			tracker.open(depth + 1)
			if strings.TrimSpace(text[cursor:cursor+nextOpen]) != "" {
				return "", 0, tracker.err("summary wrapper contains content")
			}
			childCount++
			if childCount > 1 {
				return "", 0, tracker.err("summary wrapper contains multiple blocks")
			}
			child, end, err := parseSummaryCandidate(text, cursor+nextOpen+len(open), depth+1, tracker)
			if err != nil {
				return "", 0, err
			}
			childSummary = child
			cursor = end
			continue
		}
		if strings.TrimSpace(text[cursor:cursor+nextClose]) != "" {
			if childCount > 0 {
				return "", 0, tracker.err("summary wrapper contains content")
			}
			childSummary = strings.TrimSpace(text[cursor : cursor+nextClose])
		}
		tracker.close()
		if childSummary == "" {
			return "", 0, tracker.err("summary is empty")
		}
		return childSummary, cursor + nextClose + len(close), nil
	}
}

type summaryTagTracker struct {
	openTags  int
	closeTags int
	maxDepth  int
	trace     []string
}

func (t *summaryTagTracker) open(depth int) {
	t.openTags++
	if depth > t.maxDepth {
		t.maxDepth = depth
	}
	t.trace = append(t.trace, "open")
}

func (t *summaryTagTracker) close() {
	t.closeTags++
	t.trace = append(t.trace, "close")
}

func (t *summaryTagTracker) err(reason string) error {
	return &SummaryParseError{Reason: reason, OpenTags: t.openTags, CloseTags: t.closeTags, MaxDepth: t.maxDepth, TagTrace: strings.Join(t.trace, ">")}
}

func (m *Manager) Rebuild(messages []provider.Message, summary string) []provider.Message {
	boundary := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "Previous conversation summary:\n" + strings.TrimSpace(summary) + "\n\nSome details may be omitted by compression. re-read files or prior persisted results before relying on missing details; do not assume unstated facts."}}}
	recent := recentMessages(messages, m.Config.RecentTokens, m.Config.RecentMessageMinimum)
	rebuilt := make([]provider.Message, 0, 1+len(recent))
	rebuilt = append(rebuilt, boundary)
	rebuilt = append(rebuilt, provider.CloneMessages(recent)...)
	return rebuilt
}

func recentMessages(messages []provider.Message, recentTokens, minimum int) []provider.Message {
	if len(messages) == 0 {
		return nil
	}
	groups := messageGroups(messages)
	start := len(groups)
	tokens := 0
	count := 0
	for start > 0 && (count < minimum || tokens < recentTokens) {
		start--
		group := groups[start]
		count += group.end - group.start
		tokens += estimateMessageTokens(messages[group.start:group.end])
	}
	return messages[groups[start].start:]
}

type messageGroup struct {
	start int
	end   int
}

func messageGroups(messages []provider.Message) []messageGroup {
	groups := make([]messageGroup, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		end := i + 1
		if messageHasToolCall(messages[i]) && end < len(messages) && messageHasToolResult(messages[end]) {
			end++
		}
		groups = append(groups, messageGroup{start: i, end: end})
		i = end - 1
	}
	return groups
}

func messageHasToolCall(message provider.Message) bool {
	if message.Role != provider.RoleAssistant {
		return false
	}
	for _, block := range message.Blocks {
		if block.Type == provider.BlockToolCall && block.ToolCall != nil {
			return true
		}
	}
	return false
}

func messageHasToolResult(message provider.Message) bool {
	if message.Role != provider.RoleUser {
		return false
	}
	for _, block := range message.Blocks {
		if block.Type == provider.BlockToolResult && block.ToolResult != nil {
			return true
		}
	}
	return false
}

func estimateMessageTokens(messages []provider.Message) int {
	chars := 0
	for _, message := range messages {
		for _, block := range message.Blocks {
			chars += len(block.Text)
			if block.ToolCall != nil {
				chars += len(block.ToolCall.ID) + len(block.ToolCall.Name) + len(block.ToolCall.Arguments)
			}
			if block.ToolResult != nil {
				chars += len(block.ToolResult.CallID) + len(block.ToolResult.Name) + len(block.ToolResult.Content)
			}
		}
	}
	return (chars + 3) / 4
}
