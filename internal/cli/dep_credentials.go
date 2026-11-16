// purpose: Reuse existing GitHub credentials lazily without exposing them to reports.
// responsibilities: Prefer token environment variables; read gh's credential store through a trusted executable when needed.
// architecture notes: No credential setup, permission expansion, interactive prompt or repository-local executable is allowed.
package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type tokenOutput struct{ buffer bytes.Buffer }

func (w *tokenOutput) Write(p []byte) (int, error) {
	if w.buffer.Len()+len(p) > 4096 {
		return 0, fmt.Errorf("credential output exceeds limit")
	}
	return w.buffer.Write(p)
}

func (w *tokenOutput) String() string { return w.buffer.String() }

func dependencyGitHubToken(root string) func(context.Context) (string, error) {
	var once sync.Once
	var token string
	var tokenErr error
	return func(ctx context.Context) (string, error) {
		once.Do(func() {
			for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
				if value := os.Getenv(name); value != "" {
					token = value
					return
				}
			}
			path, err := exec.LookPath("gh")
			if err != nil {
				tokenErr = fmt.Errorf("GitHub credentials unavailable")
				return
			}
			path, err = filepath.EvalSymlinks(path)
			if err != nil {
				tokenErr = fmt.Errorf("GitHub executable unavailable")
				return
			}
			root, err = filepath.EvalSymlinks(root)
			if err != nil {
				tokenErr = fmt.Errorf("repository root unavailable")
				return
			}
			if rel, err := filepath.Rel(root, path); err != nil || filepath.IsLocal(rel) {
				tokenErr = fmt.Errorf("repository-local GitHub executable refused")
				return
			}
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, path, "auth", "token", "--hostname", "github.com")
			// gh auth token is noninteractive and does not need repository PATH entries.
			cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "PATH="+filepath.Dir(path)+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
			var out tokenOutput
			cmd.Stdout = &out
			cmd.Stderr = io.Discard
			if cmd.Run() != nil {
				tokenErr = fmt.Errorf("GitHub credentials unavailable")
				return
			}
			token = strings.TrimSpace(out.String())
		})
		if tokenErr != nil || token == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
			return "", fmt.Errorf("GitHub credentials unavailable")
		}
		return token, nil
	}
}
