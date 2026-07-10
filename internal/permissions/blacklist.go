package permissions

import (
	"fmt"
	"regexp"
	"strings"
)

type BlacklistMatch struct {
	Pattern string
	Reason  string
}

type blacklistRule struct {
	pattern string
	reason  string
	re      *regexp.Regexp
}

var commandBlacklist = []blacklistRule{
	{pattern: `(?i)(^|\s)(sudo\s+)?rm\s+-(?:[a-z]*r[a-z]*f|[a-z]*f[a-z]*r)[a-z]*\s+(?:/|/\S+|~|~/.+)`, reason: "destructive recursive delete of system or home path"},
	{pattern: `(?i)(^|\s)(mkfs|mkfs\.[a-z0-9]+|fdisk|parted|diskutil\s+eraseDisk)\b`, reason: "disk formatting or partitioning command"},
	{pattern: `(?i)(^|\s)chmod\s+-R\s+(?:777|000|[ugoa+\-=rwxXsStT,]+)\s+(?:/|/etc|/bin|/sbin|/usr|/var)(?:\s|$)`, reason: "recursive permission change on system path"},
	{pattern: `(?i)(^|\s)chown\s+-R\s+\S+\s+(?:/|/etc|/bin|/sbin|/usr|/var)(?:\s|$)`, reason: "recursive ownership change on system path"},
	{pattern: `(?i)(^|\s)(killall|pkill)\s+(?:-[0-9]+\s+)?(?:-f\s+)?(?:\.|\*|.+)`, reason: "large process termination command"},
	{pattern: `(?i)(^|\s)kill\s+-9\s+-1(?:\s|$)`, reason: "kill all permitted processes"},
	{pattern: `:\s*\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;?\s*:`, reason: "fork bomb shell function"},
}

func init() {
	for i := range commandBlacklist {
		commandBlacklist[i].re = regexp.MustCompile(commandBlacklist[i].pattern)
	}
}

func CheckCommandBlacklist(commandText string) (*BlacklistMatch, error) {
	normalized := normalizeCommandText(commandText)
	for _, rule := range commandBlacklist {
		if rule.re == nil {
			return nil, fmt.Errorf("blacklist pattern was not compiled: %s", rule.pattern)
		}
		if rule.re.MatchString(normalized) {
			return &BlacklistMatch{Pattern: rule.pattern, Reason: rule.reason}, nil
		}
	}
	return nil, nil
}

func normalizeCommandText(commandText string) string {
	return strings.Join(strings.Fields(commandText), " ")
}
