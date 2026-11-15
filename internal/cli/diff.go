// purpose: Expose read-only, AI-friendly Git comparisons independently of project configuration.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type diffOptions struct {
	Against, Format string
	Context         int
}
type diffReport struct {
	SchemaVersion int             `json:"schemaVersion"`
	HasChanges    bool            `json:"hasChanges"`
	Comparison    *diffComparison `json:"comparison,omitempty"`
	Summary       *diffSummary    `json:"summary,omitempty"`
	Files         []diffFile      `json:"files"`
	Details       []diffDetail    `json:"details,omitempty"`
	Warnings      []string        `json:"warnings"`
}

// Empty reports intentionally have only three fields in every format.
func (r diffReport) MarshalJSON() ([]byte, error) {
	if !r.HasChanges {
		return json.Marshal(struct {
			SchemaVersion int        `json:"schemaVersion"`
			HasChanges    bool       `json:"hasChanges"`
			Files         []diffFile `json:"files"`
		}{r.SchemaVersion, false, []diffFile{}})
	}
	type reportAlias diffReport
	if r.Warnings == nil {
		r.Warnings = []string{}
	}
	return json.Marshal(reportAlias(r))
}

type diffComparison struct {
	Mode           string    `json:"mode"`
	RequestedBase  string    `json:"requestedBase"`
	ResolvedRef    string    `json:"resolvedRef"`
	BaselineCommit string    `json:"baselineCommit"`
	HeadCommit     string    `json:"headCommit"`
	Branch         *string   `json:"branch"`
	Target         string    `json:"target"`
	AgeAsOf        time.Time `json:"ageAsOf"`
}
type diffSummary struct {
	FilesChanged               int `json:"filesChanged"`
	Created                    int `json:"created"`
	Updated                    int `json:"updated"`
	Deleted                    int `json:"deleted"`
	Insertions                 int `json:"insertions"`
	Deletions                  int `json:"deletions"`
	FilesWithUnknownLineCounts int `json:"filesWithUnknownLineCounts"`
}
type diffFile struct {
	Path         string  `json:"path"`
	PathEncoding string  `json:"pathEncoding,omitempty"`
	Status       string  `json:"status"`
	Binary       bool    `json:"binary"`
	OldMode      *string `json:"oldMode"`
	NewMode      *string `json:"newMode"`
	Additions    *int    `json:"additions"`
	Deletions    *int    `json:"deletions"`
}
type diffDetail struct {
	Path         string         `json:"path"`
	PathEncoding string         `json:"pathEncoding,omitempty"`
	FileHistory  diffHistory    `json:"fileHistory"`
	Metadata     []diffMetadata `json:"metadata"`
	Content      diffContent    `json:"content"`
}
type diffHistory struct {
	LastCommittedUpdate          *time.Time `json:"lastCommittedUpdate"`
	DaysSinceLastCommittedUpdate *int       `json:"daysSinceLastCommittedUpdate"`
	HistoryRef                   *string    `json:"historyRef"`
	HasLocalChanges              bool       `json:"hasLocalChanges"`
}
type diffMetadata struct {
	Source     string         `json:"source"`
	Provenance string         `json:"provenance"`
	Document   map[string]any `json:"document"`
}
type diffContent struct {
	Kind      string  `json:"kind"`
	Text      *string `json:"text,omitempty"`
	OldBytes  *int64  `json:"oldBytes,omitempty"`
	NewBytes  *int64  `json:"newBytes,omitempty"`
	OldMode   *string `json:"oldMode,omitempty"`
	NewMode   *string `json:"newMode,omitempty"`
	OldCommit *string `json:"oldCommit,omitempty"`
	NewCommit *string `json:"newCommit,omitempty"`
	Dirty     bool    `json:"dirty,omitempty"`
}

func newDiffCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := diffOptions{}
	cmd := &cobra.Command{
		Use: "diff", Short: "Show contextual code changes with Thoth metadata",
		Long: "Compare the working tree (including nonignored untracked files) with the merge base of local main, or with HEAD for uncommitted changes. Read-only; no configuration or network access required.",
		Args: noExtraArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (opts.Against != "main" && opts.Against != "head") || (opts.Format != "markdown" && opts.Format != "json" && opts.Format != "yaml") || opts.Context < 0 {
				err := fmt.Errorf("invalid invocation: expected --against main|head, --format markdown|json|yaml, and nonnegative --context")
				fmt.Fprintln(stderr, err)
				return exitCodeError{code: ExitUsage, err: err}
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return resultToError(runDiff(ctx, opts, stdout, stderr))
		},
	}
	cmd.Flags().StringVar(&opts.Against, "against", "main", "Baseline: main (merge base with local main) or head (uncommitted changes)")
	cmd.Flags().StringVar(&opts.Format, "format", "markdown", "Output format: markdown, json, or yaml")
	cmd.Flags().IntVar(&opts.Context, "context", 10, "Unchanged lines around each diff hunk")
	return cmd
}

func runDiff(ctx context.Context, opts diffOptions, stdout, stderr io.Writer) Result {
	report, err := collectDiff(ctx, opts, time.Now().UTC())
	var data []byte
	if err == nil {
		data, err = renderDiff(report, opts.Format)
	}
	if err == nil {
		for _, warning := range report.Warnings {
			fmt.Fprintf(stderr, "Warning: %q\n", warning)
		}
		_, err = stdout.Write(data)
	}
	if err != nil {
		fmt.Fprintf(stderr, "diff: %v\n", err)
		return Result{ExitCode: ExitFailure, Err: err}
	}
	return Result{ExitCode: ExitOK}
}

// JSON is the canonical schema. Decode into a YAML node to preserve field order
// while sharing one set of field names and omission rules across both serializers.
func diffYAML(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err = yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	var block func(*yaml.Node)
	block = func(n *yaml.Node) {
		n.Style = 0
		if n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
			// JSON-compatible string quoting also preserves timestamps as strings.
			n.Style = yaml.DoubleQuotedStyle
			if bytes.ContainsRune([]byte(n.Value), '\n') {
				n.Style = yaml.LiteralStyle
			}
		}
		for i, child := range n.Content {
			block(child)
			if n.Kind == yaml.MappingNode && i%2 == 0 {
				child.Style = 0
			}
		}
	}
	block(&node)
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err = enc.Encode(&node); err != nil {
		return nil, err
	}
	if err = enc.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
