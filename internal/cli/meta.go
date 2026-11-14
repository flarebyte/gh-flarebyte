// purpose: Run repository metadata pipelines with concise results and content-based change counts.
package cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
	"github.com/spf13/cobra"
)

func newMetaCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{Use: "meta", Short: "Run root-level Thoth/Maat metadata pipelines", Args: noExtraArgs,
		Long: "Run trusted root-level pipeline-*-maat.thoth.cue files in filename order. Requires installed thoth and maat; configuration is optional. Pipelines may execute code.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return resultToError(runMeta(ctx, stdout, stderr))
		}}
}

// Bound capture while the process runs, rather than truncating an unbounded buffer afterward.
type metaCapture struct {
	data      []byte
	truncated bool
}

func (b *metaCapture) Write(p []byte) (int, error) {
	n := len(p)
	keep := min(n, 2000-len(b.data))
	b.data = append(b.data, p[:keep]...)
	b.truncated = b.truncated || keep < n
	return n, nil
}

func (b *metaCapture) diagnostic() string {
	// Quote controls so tool output cannot inject terminal escapes or extra result lines.
	s := strconv.Quote(strings.TrimSpace(string(b.data)))
	if b.truncated {
		s += " [diagnostics truncated]"
	}
	return s
}

func runMeta(ctx context.Context, stdout, stderr io.Writer) Result {
	fail := func(err error) Result {
		fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitFailure, Err: err}
	}
	var missing []string
	for _, tool := range []struct{ name, install string }{
		{"thoth", "brew install flarebyte/tap/thoth-ostraca (https://github.com/flarebyte/thoth-ostraca)"},
		{"maat", "npm install --global maat-ostraca; requires supported Node.js (https://github.com/flarebyte/maat-ostraca)"},
	} {
		if _, err := exec.LookPath(tool.name); err != nil {
			missing = append(missing, "Missing "+tool.name+": "+tool.install)
		}
	}
	if len(missing) > 0 {
		return fail(fmt.Errorf("%s", strings.Join(missing, "\n")))
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		return fail(err)
	}
	var files []string
	for _, entry := range entries {
		if match, _ := filepath.Match("pipeline-*-maat.thoth.cue", entry.Name()); match {
			files = append(files, entry.Name())
		}
	}
	if len(files) == 0 {
		name := "pipeline-<scope>-maat.thoth.cue"
		if cfg, err := config.Load(""); err == nil && cfg.Build.Language != "" {
			name = "pipeline-" + cfg.Build.Language + "-maat.thoth.cue"
		}
		fmt.Fprintf(stdout, "No metadata pipelines found (no-op). Consider creating %s.\n", strconv.Quote(name))
		return Result{ExitCode: ExitOK}
	}
	before, err := snapshotMeta()
	if err != nil {
		return fail(fmt.Errorf("metadata summary unavailable before execution: %w", err))
	}
	succeeded, failed := 0, 0
	for _, file := range files {
		if ctx.Err() != nil {
			break
		}
		info, runErr := os.Lstat(file)
		var output metaCapture
		if runErr == nil && !info.Mode().IsRegular() {
			runErr = fmt.Errorf("pipeline must be a regular file, not a symlink or directory")
		}
		if runErr == nil {
			cmd := exec.CommandContext(ctx, "thoth", "run", "--config", "./"+file)
			cmd.Stdout, cmd.Stderr = &output, &output
			cmd.WaitDelay = time.Second
			runErr = cmd.Run()
		}
		if runErr != nil {
			failed++
			fmt.Fprintf(stderr, "ERROR %s: %s", strconv.Quote(file), strconv.Quote(runErr.Error()))
			if len(output.data) > 0 {
				fmt.Fprintf(stderr, " %s", output.diagnostic())
			}
			fmt.Fprintln(stderr)
		} else {
			succeeded++
			fmt.Fprintf(stdout, "OK %s\n", strconv.Quote(file))
		}
	}
	after, scanErr := snapshotMeta()
	if scanErr == nil {
		created, updated, deleted := 0, 0, 0
		for name, hash := range after {
			if old, exists := before[name]; !exists {
				created++
			} else if old != hash {
				updated++
			}
		}
		for name := range before {
			if _, exists := after[name]; !exists {
				deleted++
			}
		}
		fmt.Fprintf(stdout, "Metadata: %d updated, %d created, %d deleted\n", updated, created, deleted)
	} else {
		fmt.Fprintf(stderr, "Metadata summary unavailable: %v\n", scanErr)
	}
	fmt.Fprintf(stdout, "Pipelines: %d succeeded, %d failed, %d not run\n", succeeded, failed, len(files)-succeeded-failed)
	if ctx.Err() != nil {
		return fail(fmt.Errorf("metadata run interrupted: %w", ctx.Err()))
	}
	if scanErr != nil || failed > 0 {
		return Result{ExitCode: ExitFailure, Err: fmt.Errorf("metadata refresh failed")}
	}
	return Result{ExitCode: ExitOK}
}

func snapshotMeta() (map[string][sha256.Size]byte, error) {
	result := make(map[string][sha256.Size]byte)
	// OpenRoot confines reads even if a path is replaced with an escaping symlink.
	root, err := os.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	defer root.Close()
	err = fs.WalkDir(root.FS(), "thoth-meta", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == "thoth-meta" && os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in metadata tree: %s", strconv.Quote(path))
		}
		if path == "thoth-meta" && !entry.IsDir() {
			return fmt.Errorf("thoth-meta must be a directory")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular metadata file: %s", strconv.Quote(path))
		}
		file, err := root.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, readErr := io.Copy(hash, file)
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		var sum [sha256.Size]byte
		copy(sum[:], hash.Sum(nil))
		result[path] = sum
		return nil
	})
	return result, err
}
