// purpose: Detect duplicate source and test code using an already installed jscpd.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const ccInstall = "Install jscpd on macOS/Linux with Homebrew: brew install jscpd\nOther installation methods: https://jscpd.dev/start/installation\nOfficial project: https://github.com/kucherenko/jscpd"
const ccFormats = "go,javascript,typescript,jsx,tsx,python,rust,dart,swift,kotlin,java,c,cpp,csharp,ruby,php"

// Scan source formats anywhere, including root files and monorepo packages.
// Explicit ignores supplement gitignore in both supported jscpd generations.
func ccIgnores() []string {
	var patterns []string
	for _, dir := range []string{".git", "node_modules", "vendor", "cache", ".cache", ".gocache", ".gomodcache", ".e2e-bin", ".dart_tool", ".pub-cache", ".build", ".swiftpm", ".gradle", ".venv", "venv", "__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache", "build", "dist", "target", "coverage", "report", "reports", "generated", "doc", "docs"} {
		patterns = append(patterns, "**/"+dir+"/**")
	}
	return append(patterns, "**/*.min.js", "**/*.generated.*", "**/*.g.dart", "**/*.freezed.dart", "**/*.pb.go", "**/*_generated.go")
}

type ccLocation struct {
	Name  string `json:"name"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type ccDuplicate struct {
	Lines  int        `json:"lines"`
	First  ccLocation `json:"firstFile"`
	Second ccLocation `json:"secondFile"`
}

func newCCCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{Use: "cc", Short: "Find carbon-copy code using installed jscpd (at most 10 pairs)", Args: noExtraArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return resultToError(runCC(stdout, stderr)) }}
}

func runCC(stdout, stderr io.Writer) Result {
	fail := func(err error) Result {
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitFailure, Err: err}
	}
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	env := os.Environ()
	version, diagnostics, err := runCommandCapture("jscpd", []string{"--version"}, env)
	if err != nil {
		return fail(fmt.Errorf("cannot run installed jscpd: %w\n%s\n%s", err, ccDiagnostic(diagnostics), ccInstall))
	}
	match := regexp.MustCompile(`(?m)^(?:jscpd\s+)?v?([45])\.\d+\.\d+\s*$`).FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return fail(fmt.Errorf("cc requires jscpd v4 or v5; run jscpd --version\n%s", ccInstall))
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-cc-")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	// An explicit JSON config prevents auto-discovery of repository jscpd options.
	policy := map[string]any{"minLines": cfg.CC.MinLines, "minTokens": 50, "mode": "mild", "reporters": []string{"json"}, "output": dir, "ignore": ccIgnores(), "format": strings.Split(ccFormats, ","), "gitignore": true}
	data, err := json.Marshal(policy)
	if err != nil {
		return fail(err)
	}
	configPath := filepath.Join(dir, "config.json")
	if err = os.WriteFile(configPath, data, 0o600); err != nil {
		return fail(err)
	}
	_, diagnostics, err = runCommandCapture("jscpd", []string{"--config", configPath, "."}, env)
	if err != nil {
		return fail(fmt.Errorf("jscpd scan failed: %w\n%s", err, ccDiagnostic(diagnostics)))
	}
	data, err = os.ReadFile(filepath.Join(dir, "jscpd-report.json"))
	if err != nil {
		return fail(fmt.Errorf("jscpd report missing: %w", err))
	}
	pairs, err := parseCCReport(data)
	if err != nil {
		return fail(err)
	}
	printCCReport(stdout, pairs)
	if len(pairs) > 0 {
		return Result{ExitCode: ExitFailure, Err: fmt.Errorf("duplicate code found")}
	}
	return Result{ExitCode: ExitOK}
}

func ccDiagnostic(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 2000 {
		return s[:2000] + "\n[diagnostics truncated]"
	}
	return s
}

func ccLocationKey(loc ccLocation) string {
	return fmt.Sprintf("%s:%09d:%09d", loc.Name, loc.Start, loc.End)
}

func parseCCReport(data []byte) ([]ccDuplicate, error) {
	var report struct {
		Duplicates *[]ccDuplicate `json:"duplicates"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("invalid jscpd report: %w", err)
	}
	if report.Duplicates == nil {
		return nil, fmt.Errorf("invalid jscpd report: missing duplicates array")
	}
	pairs := *report.Duplicates
	for i := range pairs {
		p := &pairs[i]
		for _, loc := range []*ccLocation{&p.First, &p.Second} {
			if loc.Name == "" || loc.Start < 1 || loc.End < loc.Start || p.Lines < 1 {
				return nil, fmt.Errorf("invalid jscpd duplicate location")
			}
			if filepath.IsAbs(loc.Name) {
				root, err := os.Getwd()
				if err != nil {
					return nil, err
				}
				loc.Name, err = filepath.Rel(root, loc.Name)
				if err != nil {
					return nil, err
				}
			}
			loc.Name = filepath.ToSlash(filepath.Clean(loc.Name))
		}
		if ccLocationKey(p.First) > ccLocationKey(p.Second) {
			p.First, p.Second = p.Second, p.First
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		a, b := pairs[i], pairs[j]
		if a.Lines != b.Lines {
			return a.Lines > b.Lines
		}
		if ccLocationKey(a.First) != ccLocationKey(b.First) {
			return ccLocationKey(a.First) < ccLocationKey(b.First)
		}
		return ccLocationKey(a.Second) < ccLocationKey(b.Second)
	})
	return pairs, nil
}

func printCCReport(w io.Writer, pairs []ccDuplicate) {
	shown := min(len(pairs), 10)
	for _, p := range pairs[:shown] {
		// Quote paths to keep each result on one line even with unusual filenames.
		_, _ = fmt.Fprintf(w, "%s:%d-%d ~ %s:%d-%d (%d lines)\n", strconv.Quote(p.First.Name), p.First.Start, p.First.End, strconv.Quote(p.Second.Name), p.Second.Start, p.Second.End, p.Lines)
	}
	_, _ = fmt.Fprintf(w, "cc: %d duplicate pairs; showing %d, %d more.\n", len(pairs), shown, len(pairs)-shown)
	if len(pairs) > 0 {
		_, _ = fmt.Fprintln(w, "Fix these and rerun gh flarebyte cc.")
	}
}
