package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type Persistence struct {
	CallID string
	Path   string
	Size   int
}
type ResultStore struct {
	root     string
	sequence int
	contents map[string]struct{}
}

func NewResultStore(root, session string) (*ResultStore, error) {
	dir := filepath.Join(root, session, "tool-results")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &ResultStore{root: dir, contents: make(map[string]struct{})}, nil
}

func (s *ResultStore) Persist(result provider.ToolResult) (Persistence, error) {
	if s == nil {
		return Persistence{}, fmt.Errorf("result store is not configured")
	}
	s.sequence++
	path := filepath.Join(s.root, fmt.Sprintf("%03d-%s.txt", s.sequence, safeName(result.CallID)))
	if err := os.WriteFile(path, []byte(result.Content), 0600); err != nil {
		return Persistence{}, err
	}
	s.contents[result.Content] = struct{}{}
	return Persistence{CallID: result.CallID, Path: path, Size: len(result.Content)}, nil
}

func safeName(value string) string {
	if value == "" {
		return "result"
	}
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, value)
}

func (m *Manager) PrepareResults(input []provider.ToolResult) ([]provider.ToolResult, []Persistence, error) {
	results := append([]provider.ToolResult(nil), input...)
	if m.Store == nil {
		return results, nil, nil
	}
	persisted := make([]Persistence, 0)
	replacements := make([]replacement, 0)
	replace := func(index int) error {
		p, err := m.Store.Persist(results[index])
		if err != nil {
			return err
		}
		original := results[index].Content
		results[index].Content = persistedReference(p, preview(original, m.Config.PreviewChars))
		persisted = append(persisted, p)
		replacements = append(replacements, replacement{index: index, persistence: p, original: original})
		return nil
	}
	active := make([]int, 0, len(results))
	total := 0
	for i := range results {
		if m.isReadback(results[i]) {
			continue
		}
		if len(results[i].Content) > m.Config.SingleResultChars {
			if err := replace(i); err != nil {
				return nil, nil, err
			}
			continue
		}
		active = append(active, i)
		total += len(results[i].Content)
	}
	sort.SliceStable(active, func(i, j int) bool { return len(results[active[i]].Content) > len(results[active[j]].Content) })
	for _, index := range active {
		if total <= m.Config.MessageResultChars {
			break
		}
		total -= len(results[index].Content)
		if err := replace(index); err != nil {
			return nil, nil, err
		}
	}
	fitPersistedPreviews(results, replacements, m.Config.MessageResultChars, m.Config.PreviewChars)
	return results, persisted, nil
}

type replacement struct {
	index       int
	persistence Persistence
	original    string
}

func persistedReference(p Persistence, preview string) string {
	return fmt.Sprintf("Tool result persisted (%d chars): %s\nPreview:\n%s", p.Size, p.Path, preview)
}

func preview(content string, limit int) string {
	if len(content) > limit {
		return content[:limit]
	}
	return content
}

func fitPersistedPreviews(results []provider.ToolResult, replacements []replacement, budget, previewLimit int) {
	if budget <= 0 || len(replacements) == 0 || toolResultContentLen(results) <= budget {
		return
	}
	fixed := 0
	for i := range results {
		replaced := false
		for _, item := range replacements {
			if item.index == i {
				replaced = true
				break
			}
		}
		if !replaced {
			fixed += len(results[i].Content)
		}
	}
	emptyRefs := 0
	for _, item := range replacements {
		emptyRefs += len(persistedReference(item.persistence, ""))
	}
	available := budget - fixed - emptyRefs
	if available < 0 {
		return
	}
	perPreview := available / len(replacements)
	if perPreview > previewLimit {
		perPreview = previewLimit
	}
	for _, item := range replacements {
		results[item.index].Content = persistedReference(item.persistence, preview(item.original, perPreview))
	}
}

func toolResultContentLen(results []provider.ToolResult) int {
	total := 0
	for _, result := range results {
		total += len(result.Content)
	}
	return total
}

func (m *Manager) isReadback(result provider.ToolResult) bool {
	if m.Store == nil || result.Name != "read_file" {
		return false
	}
	if _, ok := m.Store.contents[result.Content]; ok {
		return true
	}
	var wrapped struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Content), &wrapped); err != nil || wrapped.Data.Content == "" {
		return false
	}
	_, ok := m.Store.contents[wrapped.Data.Content]
	return ok
}
