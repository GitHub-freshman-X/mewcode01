package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxIndexLines         = 200
	maxIndexBytes         = 25 * 1024
	indexTruncationNotice = "<!-- mewcode memory index truncated -->"
)

func LoadIndexes(paths Paths) ([]string, error) {
	userIndex, err := loadIndexForKind(paths, MemoryUser)
	if err != nil {
		return nil, err
	}
	projectIndex, err := loadIndexForKind(paths, MemoryProject)
	if err != nil {
		return nil, err
	}
	return []string{userIndex, projectIndex}, nil
}

func loadIndexForKind(paths Paths, kind MemoryKind) (string, error) {
	path, err := memoryIndexPath(paths, kind)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("cannot read memory index")
	}
	return limitIndex(string(content)), nil
}

func limitIndex(content string) string {
	if content == "" {
		return ""
	}

	remaining := []byte(content)
	limited := make([]byte, 0, min(len(remaining), maxIndexBytes))
	lines := 0
	truncated := false
	for len(remaining) > 0 {
		if lines == maxIndexLines || len(limited) == maxIndexBytes {
			truncated = true
			break
		}
		end := len(remaining)
		if newline := strings.IndexByte(string(remaining), '\n'); newline >= 0 {
			end = newline + 1
		}
		available := maxIndexBytes - len(limited)
		if end > available {
			end = available
			truncated = true
		}
		limited = append(limited, remaining[:end]...)
		remaining = remaining[end:]
		lines++
		if end == available && len(remaining) > 0 {
			truncated = true
			break
		}
	}
	if !truncated {
		return string(limited)
	}
	for len(limited) > 0 && !utf8.Valid(limited) {
		limited = limited[:len(limited)-1]
	}
	return string(limited) + "\n" + indexTruncationNotice
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return errors.New("cannot create memory directory")
	}
	temporary, err := os.CreateTemp(directory, ".mewcode-memory-*")
	if err != nil {
		return errors.New("cannot create memory file")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return errors.New("cannot set memory file permissions")
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return errors.New("cannot write memory file")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errors.New("cannot sync memory file")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("cannot close memory file")
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return errors.New("cannot replace memory file")
	}
	return nil
}
