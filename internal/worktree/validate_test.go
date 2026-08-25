package worktree

import "testing"

func TestValidateSlug(t *testing.T) {
	for _, name := range []string{"feature", "team/refactor", "a_b.c-1"} {
		if err := ValidateSlug(name); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"", ".", "..", "a//b", "a/../b", "../x", "a b"} {
		if err := ValidateSlug(name); err == nil {
			t.Fatalf("%q accepted", name)
		}
	}
}
