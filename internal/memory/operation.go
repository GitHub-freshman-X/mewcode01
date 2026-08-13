package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type MemoryOperation struct {
	Action      string     `json:"action"`
	Kind        MemoryKind `json:"kind"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Content     string     `json:"content"`
}

func ParseOperations(data []byte) ([]MemoryOperation, error) {
	data, err := normalizeOperationJSON(data)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte("[")) {
		return nil, errors.New("memory operations must be a JSON array")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var operations []MemoryOperation
	if err := decoder.Decode(&operations); err != nil {
		return nil, errors.New("cannot parse memory operations")
	}
	if decoder.More() {
		return nil, errors.New("cannot parse memory operations")
	}
	for _, operation := range operations {
		if err := operation.validate(); err != nil {
			return nil, err
		}
	}
	return operations, nil
}

func normalizeOperationJSON(data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	if !bytes.HasPrefix(trimmed, []byte("```")) {
		return trimmed, nil
	}
	text := string(trimmed)
	newline := strings.IndexByte(text, '\n')
	if newline < 0 {
		return nil, errors.New("memory operation code block is incomplete")
	}
	language := strings.TrimSpace(text[3:newline])
	if language != "" && !strings.EqualFold(language, "json") {
		return nil, errors.New("memory operation code block is not JSON")
	}
	if !strings.HasSuffix(text, "```") {
		return nil, errors.New("memory operation code block is incomplete")
	}
	return bytes.TrimSpace([]byte(text[newline+1 : len(text)-3])), nil
}

func ApplyOperations(paths Paths, operations []MemoryOperation) error {
	for _, operation := range operations {
		if err := operation.validate(); err != nil {
			return err
		}
		if operation.Action == "noop" {
			continue
		}
		if _, err := MemoryFilePath(paths, operation.Kind, operation.Name); err != nil {
			return err
		}
		if _, err := memoryIndexPath(paths, operation.Kind); err != nil {
			return err
		}
	}

	for _, operation := range operations {
		if operation.Action == "noop" {
			continue
		}
		if err := applyOperation(paths, operation); err != nil {
			return err
		}
	}
	return nil
}

func (operation MemoryOperation) validate() error {
	switch operation.Action {
	case "noop":
		if operation.Kind != "" || operation.Name != "" || operation.Description != "" || operation.Content != "" {
			return errors.New("noop memory operation must not include memory data")
		}
		return nil
	case "create", "update":
		if !validMemoryKind(operation.Kind) || !validSlug(operation.Name) || !validFrontmatterValue(operation.Description) || operation.Content == "" {
			return errors.New("invalid memory operation")
		}
		return nil
	case "delete":
		if !validMemoryKind(operation.Kind) || !validSlug(operation.Name) || operation.Description != "" || operation.Content != "" {
			return errors.New("invalid memory operation")
		}
		return nil
	default:
		return errors.New("unknown memory operation action")
	}
}

func validMemoryKind(kind MemoryKind) bool {
	switch kind {
	case MemoryUser, MemoryFeedback, MemoryProject, MemoryReference:
		return true
	default:
		return false
	}
}

func validFrontmatterValue(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "\r\n\x00")
}

func applyOperation(paths Paths, operation MemoryOperation) error {
	memoryPath, err := MemoryFilePath(paths, operation.Kind, operation.Name)
	if err != nil {
		return err
	}
	indexPath, err := memoryIndexPath(paths, operation.Kind)
	if err != nil {
		return err
	}

	if operation.Action == "delete" {
		if err := os.Remove(memoryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("cannot remove memory file")
		}
	} else if err := atomicWrite(memoryPath, []byte(renderMemory(operation)), 0o600); err != nil {
		return err
	}

	index, err := os.ReadFile(indexPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot read memory index")
	}
	updatedIndex := updateIndex(string(index), operation)
	if err := atomicWrite(indexPath, []byte(updatedIndex), 0o600); err != nil {
		return err
	}
	return nil
}

func renderMemory(operation MemoryOperation) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\ntype: %s\n---\n\n%s\n", operation.Name, frontmatterScalar(operation.Description), operation.Kind, operation.Content)
}

func frontmatterScalar(value string) string {
	if strings.ContainsAny(value, ":#[]{}&,*!|>'\"%@`") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "?") {
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
	return value
}

func updateIndex(index string, operation MemoryOperation) string {
	reference := "(" + operation.Name + ".md)"
	lines := strings.Split(index, "\n")
	kept := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		if !strings.Contains(line, reference) && line != "" {
			kept = append(kept, line)
		}
	}
	if operation.Action != "delete" {
		kept = append(kept, fmt.Sprintf("- [%s](%s.md) — %s", operation.Name, operation.Name, operation.Description))
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n") + "\n"
}
