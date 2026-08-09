package envfile

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"
)

type Result struct {
	Loaded, Skipped []string
	Found           bool
}

func Load(path string, lookup func(string) (string, bool), set func(string, string) error) (Result, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Result{}, nil
	}
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	result := Result{Found: true}
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok || !validKey(key) {
			return Result{}, fmt.Errorf("dotenv: invalid declaration at line %d", line)
		}
		if _, exists := lookup(key); exists {
			result.Skipped = append(result.Skipped, key)
			continue
		}
		if err := set(key, value); err != nil {
			return Result{}, err
		}
		result.Loaded = append(result.Loaded, key)
	}
	if err := scanner.Err(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func validKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if !(r == '_' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r))) {
			return false
		}
	}
	return true
}
