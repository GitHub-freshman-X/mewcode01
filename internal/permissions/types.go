package permissions

import (
	"encoding/json"

	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type Request struct {
	CallID      string
	Tool        string
	Arguments   json.RawMessage
	Safety      tools.Safety
	MatchTarget string
	Paths       []PathCheck
}

type PathCheck struct {
	Raw       string
	Real      string
	Relative  string
	Parameter string
}
