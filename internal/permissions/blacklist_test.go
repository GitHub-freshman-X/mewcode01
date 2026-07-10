package permissions

import "testing"

func TestDangerousCommandBlacklistRejectsHighRiskCommands(t *testing.T) {
	tests := []string{
		"rm -rf /",
		"sudo rm -rf /var",
		"mkfs.ext4 /dev/sda",
		"chmod -R 777 /",
		"chown -R root /etc",
		"kill -9 -1",
		"pkill -9 -f .",
		":(){ :|:& };:",
	}
	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			match, err := CheckCommandBlacklist(command)
			if err != nil {
				t.Fatalf("CheckCommandBlacklist returned error: %v", err)
			}
			if match == nil {
				t.Fatalf("expected %q to be rejected", command)
			}
			if match.Reason == "" || match.Pattern == "" {
				t.Fatalf("blacklist match lacks diagnostics: %#v", match)
			}
		})
	}
}

func TestDangerousCommandBlacklistAllowsCommonSafeCommands(t *testing.T) {
	tests := []string{
		"git status",
		"go test ./...",
		"rm ./tmp/output.txt",
		"chmod 644 README.md",
		"kill 12345",
	}
	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			match, err := CheckCommandBlacklist(command)
			if err != nil {
				t.Fatalf("CheckCommandBlacklist returned error: %v", err)
			}
			if match != nil {
				t.Fatalf("expected %q to be allowed, got %#v", command, match)
			}
		})
	}
}
