package worktree

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const maxSlugLength = 64

var slugPart = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func ValidateSlug(name string) error {
	if len(name) == 0 || len(name) > maxSlugLength {
		return fmt.Errorf("worktree name must be 1-%d characters", maxSlugLength)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || !slugPart.MatchString(part) {
			return fmt.Errorf("invalid worktree name %q", name)
		}
	}
	return nil
}

func flatSlug(name string) string { return strings.ReplaceAll(name, "/", "+") }

func worktreePath(root, name string) (string, error) {
	if err := ValidateSlug(name); err != nil {
		return "", err
	}
	return filepath.Join(root, ".mewcode", "worktrees", flatSlug(name)), nil
}

func branchName(name string) (string, error) {
	if err := ValidateSlug(name); err != nil {
		return "", err
	}
	return "worktree-" + flatSlug(name), nil
}
