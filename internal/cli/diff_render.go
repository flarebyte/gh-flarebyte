// purpose: Render the canonical code-review report as Markdown, JSON, or YAML.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

func renderDiff(report diffReport, format string) ([]byte, error) {
	if format == "yaml" {
		return diffYAML(report)
	}
	var out bytes.Buffer
	if format == "json" {
		enc := json.NewEncoder(&out)
		enc.SetEscapeHTML(false)
		if report.HasChanges {
			enc.SetIndent("", "  ")
		}
		err := enc.Encode(report)
		return out.Bytes(), err
	}
	if !report.HasChanges {
		return []byte("No changes.\n"), nil
	}
	c := report.Comparison
	branch := "detached HEAD"
	if c.Branch != nil {
		branch = *c.Branch
	}
	fmt.Fprintln(&out, "# Code diff")
	fmt.Fprintf(&out, "\nComparison: %s; base %s at %s; branch %s; target working-tree.\n", c.Mode, markdownDiffPath(c.ResolvedRef), c.BaselineCommit, markdownDiffPath(branch))
	fmt.Fprintf(&out, "File ages as of: %s.\n", c.AgeAsOf.Format("2006-01-02T15:04:05Z"))
	fmt.Fprintln(&out, "\n## Files\n\n| Path | Change | Added | Removed |\n| --- | --- | ---: | ---: |")
	for _, f := range report.Files {
		fmt.Fprintf(&out, "| %s | %s | %s | %s |\n", markdownDiffPath(f.Path), f.Status, diffCount(f.Additions), diffCount(f.Deletions))
	}
	s := report.Summary
	fmt.Fprintf(&out, "\n%d files changed, %d insertions(+), %d deletions(-).", s.FilesChanged, s.Insertions, s.Deletions)
	if s.FilesWithUnknownLineCounts > 0 {
		fmt.Fprintf(&out, " Counts exclude %d files with unavailable line counts.", s.FilesWithUnknownLineCounts)
	}
	fmt.Fprintln(&out)
	for i, d := range report.Details {
		f := report.Files[i]
		fmt.Fprintf(&out, "\n## %s\n\nChange: %s; Git mode: %s → %s", markdownDiffPath(d.Path), f.Status, diffOptional(f.OldMode), diffOptional(f.NewMode))
		if d.PathEncoding != "" {
			fmt.Fprintf(&out, "; path encoding: %s", d.PathEncoding)
		}
		fmt.Fprintln(&out)
		h := d.FileHistory
		fmt.Fprint(&out, "\nLast committed update: ")
		if h.LastCommittedUpdate == nil {
			fmt.Fprint(&out, "none (new file)")
		} else {
			fmt.Fprintf(&out, "%s (%d days ago; history: %s)", h.LastCommittedUpdate.Format("2006-01-02T15:04:05Z"), *h.DaysSinceLastCommittedUpdate, *h.HistoryRef)
		}
		local := "no"
		if h.HasLocalChanges {
			local = "yes"
		}
		fmt.Fprintf(&out, "; local changes: %s.\n", local)
		for _, m := range d.Metadata {
			fmt.Fprintf(&out, "\n### Thoth metadata\n\nSource: %s (%s).\n\n", markdownDiffPath(m.Source), m.Provenance)
			data, err := diffYAML(m.Document)
			if err != nil {
				return nil, err
			}
			writeDiffFence(&out, "yaml", string(data))
		}
		switch d.Content.Kind {
		case "full-file":
			fmt.Fprint(&out, "\n### New file\n\n")
			writeDiffFence(&out, diffLanguage(d.Path), *d.Content.Text)
			if *d.Content.Text != "" && !strings.HasSuffix(*d.Content.Text, "\n") {
				fmt.Fprintln(&out, "\nNo newline at end of file.")
			}
		case "binary":
			fmt.Fprintf(&out, "\nBinary change; bytes: %s → %s.\n", diffByteCount(d.Content.OldBytes), diffByteCount(d.Content.NewBytes))
		case "submodule":
			fmt.Fprintf(&out, "\nSubmodule: %s → %s; dirty: %t.\n", diffOptional(d.Content.OldCommit), diffOptional(d.Content.NewCommit), d.Content.Dirty)
			if d.Content.Text != nil {
				fmt.Fprintln(&out)
				writeDiffFence(&out, diffLanguage(d.Path), *d.Content.Text)
			}
		default:
			fmt.Fprint(&out, "\n### Diff\n\n")
			writeDiffFence(&out, "diff", *d.Content.Text)
		}
	}
	if len(report.Warnings) > 0 {
		fmt.Fprint(&out, "\n## Warnings\n\n")
		for _, w := range report.Warnings {
			fmt.Fprintf(&out, "- %s\n", markdownDiffPath(w))
		}
	}
	return out.Bytes(), nil
}
func diffCount(n *int) string {
	if n == nil {
		return "—"
	}
	return strconv.Itoa(*n)
}
func diffByteCount(n *int64) string {
	if n == nil {
		return "absent"
	}
	return strconv.FormatInt(*n, 10)
}
func diffOptional(s *string) string {
	if s == nil {
		return "absent"
	}
	return *s
}
func markdownDiffPath(path string) string {
	var out strings.Builder
	for _, r := range path {
		switch {
		case unicode.IsControl(r):
			out.WriteString(strconv.QuoteRune(r))
		case strings.ContainsRune("\\`*_{}[]<>()#|&", r):
			fmt.Fprintf(&out, "&#%d;", r)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
func writeDiffFence(out *bytes.Buffer, language, text string) {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	fmt.Fprintln(out, fence+language)
	out.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, fence)
}
func diffLanguage(path string) string {
	switch filepath.Ext(path) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md":
		return "markdown"
	case ".py":
		return "python"
	case ".sh":
		return "sh"
	case ".rs":
		return "rust"
	case ".dart":
		return "dart"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	}
	return "text"
}
