// purpose: Parse repo-related CLI arguments into typed values consumed by repo command handlers.
// responsibilities: Validate repo/audit/update/mine flags; split owner/name repo strings; resolve absolute paths for messages.
// architecture notes: Parsers are strict (unknown-arg rejection) to keep automation behavior deterministic and avoid silently ignored flags.
package cli

import (
	"errors"
	"path/filepath"
	"strings"
)

func splitRepo(repo string) (owner string, name string, err error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid invocation: --repo must use owner/name")
	}
	return parts[0], parts[1], nil
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
