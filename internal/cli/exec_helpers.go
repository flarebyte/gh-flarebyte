// purpose: Normalize stderr-first command execution failures into user-facing error messages.
// responsibilities: Convert exec errors into concise errors; prefer captured stderr text over generic process errors.
// architecture notes: This keeps shell-backed helpers behaviorally consistent across commands and avoids leaking noisy wrapped exec details.
package cli

import (
	"errors"
	"strings"
)

func commandError(runErr error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = runErr.Error()
	}
	return errors.New(msg)
}
