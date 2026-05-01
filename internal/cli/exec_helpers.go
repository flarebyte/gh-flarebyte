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
