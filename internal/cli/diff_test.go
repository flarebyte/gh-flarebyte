package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func diffFixture(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_AUTHOR_DATE", "2026-11-10T14:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2026-11-10T14:00:00Z")
	diffTestGit(t, "init", "-b", "main")
	diffTestGit(t, "config", "user.name", "Diff Test")
	diffTestGit(t, "config", "user.email", "diff@example.invalid")
	diffTestGit(t, "config", "core.autocrlf", "false")
	diffTestWrite(t, "a.txt", "first\nsecond\nthird\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "initial")
}
func diffTestGit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func diffTestWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
func diffCollect(t *testing.T, against string, contextLines int) diffReport {
	t.Helper()
	now := time.Date(2026, 11, 14, 14, 0, 0, 0, time.UTC)
	report, err := collectDiff(context.Background(), diffOptions{Against: against, Context: contextLines}, now)
	if err != nil {
		t.Fatal(err)
	}
	return report
}
func diffFind(t *testing.T, report diffReport, path string) (diffFile, diffDetail) {
	t.Helper()
	for i, f := range report.Files {
		if f.Path == path {
			return f, report.Details[i]
		}
	}
	t.Fatalf("missing %q in %+v", path, report.Files)
	return diffFile{}, diffDetail{}
}

func TestDiffEmptyAndValidation(t *testing.T) {
	diffFixture(t)
	for _, format := range []string{"markdown", "json", "yaml"} {
		var out, stderr bytes.Buffer
		result := Run([]string{"diff", "--format", format}, &out, &stderr)
		if result.ExitCode != 0 || stderr.Len() != 0 {
			t.Fatalf("%+v %s", result, &stderr)
		}
		if format == "markdown" && out.String() != "No changes.\n" {
			t.Fatal(out.String())
		}
		if format == "json" && out.String() != "{\"schemaVersion\":1,\"hasChanges\":false,\"files\":[]}\n" {
			t.Fatal(out.String())
		}
		if format == "yaml" {
			var doc map[string]any
			if err := yaml.Unmarshal(out.Bytes(), &doc); err != nil || len(doc) != 3 {
				t.Fatalf("%v %s", err, &out)
			}
		}
	}
	for _, args := range [][]string{{"--against", "upstream"}, {"--base", "origin/main"}, {"--context", "-1"}, {"--context", "oops"}, {"--format", "xml"}, {"extra"}} {
		var out, stderr bytes.Buffer
		result := Run(append([]string{"diff"}, args...), &out, &stderr)
		if result.ExitCode != ExitUsage || out.Len() != 0 {
			t.Fatalf("%v: %+v %s %s", args, result, &out, &stderr)
		}
	}
}

func TestDiffBaselinesAndContext(t *testing.T) {
	diffFixture(t)
	diffTestGit(t, "checkout", "-b", "feature")
	diffTestWrite(t, "branch.txt", "branch commit\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "branch")
	diffTestGit(t, "checkout", "main")
	diffTestWrite(t, "only-main.txt", "main commit\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "main moved")
	diffTestGit(t, "checkout", "feature")
	if report := diffCollect(t, "head", 10); report.HasChanges {
		t.Fatalf("head includes committed changes: %+v", report.Files)
	}
	report := diffCollect(t, "main", 10)
	if len(report.Files) != 1 || report.Files[0].Path != "branch.txt" {
		t.Fatalf("bad main comparison: %+v", report.Files)
	}
	diffTestWrite(t, "a.txt", "first\nchanged\nthird\n")
	zero := diffCollect(t, "head", 0)
	_, d := diffFind(t, zero, "a.txt")
	if strings.Contains(*d.Content.Text, "\n first\n") || !strings.Contains(*d.Content.Text, "@@ -2 +2 @@") {
		t.Fatal(*d.Content.Text)
	}
	full := diffCollect(t, "head", 10)
	_, d = diffFind(t, full, "a.txt")
	if !strings.Contains(*d.Content.Text, " first") || !strings.Contains(*d.Content.Text, " third") {
		t.Fatal(*d.Content.Text)
	}
	if d.FileHistory.DaysSinceLastCommittedUpdate == nil || *d.FileHistory.DaysSinceLastCommittedUpdate != 4 || !d.FileHistory.HasLocalChanges {
		t.Fatalf("%+v", d.FileHistory)
	}
}

func TestDiffMixedChangesAndNoMutation(t *testing.T) {
	diffFixture(t)
	diffTestWrite(t, "delete.txt", "remove me\n")
	diffTestWrite(t, "staged.txt", "old\n")
	diffTestWrite(t, ".gitignore", "ignored.txt\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "more files")
	diffTestWrite(t, "a.txt", "first\nlocal\nthird\n")
	diffTestGit(t, "add", "a.txt")
	diffTestWrite(t, "a.txt", "first\nfinal\nthird\n")
	diffTestWrite(t, "staged.txt", "staged\n")
	diffTestGit(t, "add", "staged.txt")
	if err := os.Remove("delete.txt"); err != nil {
		t.Fatal(err)
	}
	diffTestWrite(t, "new file.txt", "new content without newline")
	diffTestWrite(t, "ignored.txt", "excluded")
	before, err := os.ReadFile(".git/index")
	if err != nil {
		t.Fatal(err)
	}
	report := diffCollect(t, "head", 10)
	paths := []string{}
	for _, f := range report.Files {
		paths = append(paths, f.Path)
	}
	if !reflect.DeepEqual(paths, []string{"a.txt", "delete.txt", "new file.txt", "staged.txt"}) {
		t.Fatal(paths)
	}
	_, a := diffFind(t, report, "a.txt")
	if !strings.Contains(*a.Content.Text, "+final") || strings.Contains(*a.Content.Text, "+local") {
		t.Fatal(*a.Content.Text)
	}
	f, d := diffFind(t, report, "delete.txt")
	if f.Status != "deleted" || !strings.Contains(*d.Content.Text, "-remove me") {
		t.Fatalf("%+v %+v", f, d)
	}
	f, d = diffFind(t, report, "new file.txt")
	if f.Status != "created" || d.Content.Kind != "full-file" || *d.Content.Text != "new content without newline" || d.FileHistory.LastCommittedUpdate != nil {
		t.Fatalf("%+v %+v", f, d)
	}
	after, err := os.ReadFile(".git/index")
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("index changed")
	}
	diffTestGit(t, "add", "new file.txt")
	if err := os.Remove("new file.txt"); err != nil {
		t.Fatal(err)
	}
	report = diffCollect(t, "head", 10)
	for _, f := range report.Files {
		if f.Path == "new file.txt" {
			t.Fatal("net-zero staged creation included")
		}
	}
	if err = os.Mkdir("nested", 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir("nested")
	nested := diffCollect(t, "head", 10)
	if !reflect.DeepEqual(report.Files, nested.Files) {
		t.Fatal("subdirectory changed paths")
	}
}

func TestDiffThothEnrichmentAndFormats(t *testing.T) {
	diffFixture(t)
	diffTestWrite(t, "thoth-meta/go/a.txt.thoth.yaml", "locator: a.txt\nmeta:\n  purpose: Original purpose\n  responsibilities: |\n    One\n    Two\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "metadata")
	// Removed metadata remains available from the baseline.
	if err := os.Remove("thoth-meta/go/a.txt.thoth.yaml"); err != nil {
		t.Fatal(err)
	}
	diffTestWrite(t, "a.txt", "changed\n")
	report := diffCollect(t, "head", 10)
	_, d := diffFind(t, report, "a.txt")
	if len(d.Metadata) != 1 || d.Metadata[0].Provenance != "baseline" || d.Metadata[0].Document["locator"] != "a.txt" {
		t.Fatalf("%+v", d.Metadata)
	}
	diffTestWrite(t, "thoth-meta/custom/second.thoth.yaml", "locator: ./a.txt\nmeta:\n  purpose: Current purpose\n")
	diffTestWrite(t, "thoth-meta/custom/first.thoth.yaml", "locator: a.txt\nmeta:\n  language: text\n")
	diffTestWrite(t, "thoth-meta/broken.thoth.yaml", "locator: [bad\n")
	diffTestWrite(t, "thoth-meta/escape.thoth.yaml", "locator: ../outside\n")
	report = diffCollect(t, "head", 10)
	_, d = diffFind(t, report, "a.txt")
	if len(d.Metadata) != 2 || d.Metadata[0].Source != "thoth-meta/custom/first.thoth.yaml" || len(report.Warnings) != 2 {
		t.Fatalf("%+v %v", d.Metadata, report.Warnings)
	}
	var jsonDoc, yamlDoc any
	data, err := renderDiff(report, "json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &jsonDoc); err != nil {
		t.Fatal(err)
	}
	data, err = renderDiff(report, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = yaml.Unmarshal(data, &yamlDoc); err != nil {
		t.Fatal(err)
	}
	// Normalize YAML numbers to JSON numbers before comparing.
	normalized, err := json.Marshal(yamlDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(normalized, &yamlDoc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jsonDoc, yamlDoc) {
		t.Fatalf("format models differ\n%s", data)
	}
	markdown, err := renderDiff(report, "markdown")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(markdown, []byte("4 days ago")) || !bytes.Contains(markdown, []byte("Current purpose")) {
		t.Fatal(string(markdown))
	}
}

func TestDiffBinaryModesSymlinksAndFences(t *testing.T) {
	diffFixture(t)
	diffTestWrite(t, "binary.bin", "old\x00data")
	diffTestWrite(t, "execute.sh", "echo hi\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "special files")
	diffTestWrite(t, "binary.bin", "new\x00data")
	if err := os.Chmod("execute.sh", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/outside-do-not-read", "link"); err != nil {
		t.Fatal(err)
	}
	diffTestWrite(t, "new.md", "`````\nembedded fence\n`````\n")
	diffTestWrite(t, "new-invalid.bin", "\xffbad")
	diffTestWrite(t, "pipe|line\nname.txt", "safe\n")
	report := diffCollect(t, "head", 10)
	f, d := diffFind(t, report, "binary.bin")
	if !f.Binary || f.Additions != nil || d.Content.Kind != "binary" || d.Content.OldBytes == nil || *d.Content.NewBytes != 8 {
		t.Fatalf("%+v %+v", f, d.Content)
	}
	f, d = diffFind(t, report, "execute.sh")
	if d.Content.Kind != "mode-change" || f.Additions == nil || *f.Additions != 0 {
		t.Fatalf("%+v %+v", f, d.Content)
	}
	f, d = diffFind(t, report, "link")
	if *f.NewMode != "120000" || *d.Content.Text != "/outside-do-not-read" {
		t.Fatalf("%+v %+v", f, d.Content)
	}
	f, _ = diffFind(t, report, "new-invalid.bin")
	if !f.Binary {
		t.Fatal("invalid UTF8 must be binary")
	}
	markdown, err := renderDiff(report, "markdown")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(markdown, []byte("``````markdown")) || bytes.Contains(markdown, []byte("pipe|line\nname.txt")) {
		t.Fatal(string(markdown))
	}
}

func TestDiffErrorsAndDetachedHEAD(t *testing.T) {
	diffFixture(t)
	diffTestGit(t, "checkout", "--detach")
	if report := diffCollect(t, "main", 10); report.HasChanges {
		t.Fatal("detached clean comparison")
	}
	diffTestGit(t, "branch", "-D", "main")
	var out, stderr bytes.Buffer
	result := Run([]string{"diff"}, &out, &stderr)
	if result.ExitCode != ExitFailure || out.Len() != 0 || !strings.Contains(stderr.String(), "local main") {
		t.Fatalf("%+v %s %s", result, &out, &stderr)
	}
	if report := diffCollect(t, "head", 10); report.HasChanges {
		t.Fatal("head requires no main")
	}
	t.Chdir(t.TempDir())
	diffTestGit(t, "init", "-b", "main")
	out.Reset()
	stderr.Reset()
	result = Run([]string{"diff", "--against", "head"}, &out, &stderr)
	if result.ExitCode != ExitFailure || !strings.Contains(stderr.String(), "committed HEAD") {
		t.Fatalf("%+v %s", result, &stderr)
	}
}

func TestDiffConflicts(t *testing.T) {
	diffFixture(t)
	diffTestGit(t, "checkout", "-b", "feature")
	diffTestWrite(t, "a.txt", "feature\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "feature")
	diffTestGit(t, "checkout", "main")
	diffTestWrite(t, "a.txt", "main\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "main")
	if err := exec.Command("git", "merge", "feature").Run(); err == nil {
		t.Fatal("expected conflict")
	}
	var out, stderr bytes.Buffer
	result := Run([]string{"diff", "--against", "head"}, &out, &stderr)
	if result.ExitCode != ExitFailure || out.Len() != 0 || !strings.Contains(stderr.String(), "unmerged") || !strings.Contains(stderr.String(), "a.txt") {
		t.Fatalf("%+v %s %s", result, &out, &stderr)
	}
}

func TestDiffRecreatedIndexDeletion(t *testing.T) {
	diffFixture(t)
	diffTestGit(t, "rm", "a.txt")
	diffTestWrite(t, "a.txt", "first\nsecond\nthird\n")
	if r := diffCollect(t, "head", 10); r.HasChanges {
		t.Fatalf("net-zero recreation: %+v", r.Files)
	}
	diffTestWrite(t, "a.txt", "replacement\n")
	r := diffCollect(t, "head", 10)
	f, d := diffFind(t, r, "a.txt")
	if len(r.Files) != 1 || f.Status != "updated" || !strings.Contains(*d.Content.Text, "+replacement") {
		t.Fatalf("%+v %+v", r.Files, d.Content)
	}
}

func TestDiffMetadataRejectsUnsafeYAML(t *testing.T) {
	for _, data := range []string{"locator: a\nmeta: !custom tag\n", "locator: &path a\nmeta: *path\n", "locator: a\n---\nlocator: b\n", "locator: /outside\n", "locator: a\nmeta:\n  2: value\n"} {
		if _, _, err := parseDiffMetadata([]byte(data)); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
}

func TestDiffSparseCheckout(t *testing.T) {
	diffFixture(t)
	diffTestWrite(t, "included/file.txt", "keep\n")
	diffTestWrite(t, "omitted/file.txt", "old\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "directories")
	diffTestGit(t, "checkout", "-b", "feature")
	diffTestWrite(t, "omitted/file.txt", "new\n")
	diffTestWrite(t, "omitted/created.txt", "created\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "omitted changes")
	diffTestGit(t, "sparse-checkout", "init", "--cone")
	diffTestGit(t, "sparse-checkout", "set", "included")
	report := diffCollect(t, "main", 10)
	if len(report.Files) != 2 {
		t.Fatalf("sparse omissions mistaken for deletions: %+v", report.Files)
	}
	f, d := diffFind(t, report, "omitted/created.txt")
	if f.Status != "created" || *d.Content.Text != "created\n" || d.FileHistory.HasLocalChanges {
		t.Fatalf("%+v %+v", f, d)
	}
	_, d = diffFind(t, report, "omitted/file.txt")
	if !strings.Contains(*d.Content.Text, "+new") {
		t.Fatal(*d.Content.Text)
	}
}

func TestDiffDoesNotExecuteGitDrivers(t *testing.T) {
	diffFixture(t)
	diffTestWrite(t, ".gitattributes", "a.txt filter=example diff=example\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "attributes")
	diffTestGit(t, "config", "filter.example.clean", "touch filter-executed; cat")
	diffTestGit(t, "config", "filter.example.process", "touch process-executed; exit 1")
	diffTestGit(t, "config", "filter.example.required", "true")
	diffTestGit(t, "config", "diff.example.command", "touch diff-executed")
	diffTestGit(t, "config", "diff.example.textconv", "touch textconv-executed")
	diffTestGit(t, "config", "core.fsmonitor", "touch fsmonitor-executed")
	diffTestWrite(t, "a.txt", "changed\n")
	if r := diffCollect(t, "head", 10); !r.HasChanges {
		t.Fatal("missing changes")
	}
	for _, name := range []string{"filter-executed", "process-executed", "diff-executed", "textconv-executed", "fsmonitor-executed"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("driver executed: %s (%v)", name, err)
		}
	}
}

func TestDiffInvalidUTF8Path(t *testing.T) {
	diffFixture(t)
	name := "invalid-\xff.txt"
	if err := os.WriteFile(name, []byte("content\n"), 0o644); err != nil {
		t.Skipf("filesystem does not support non-UTF-8 paths: %v", err)
	}
	report := diffCollect(t, "head", 10)
	if len(report.Files) != 1 || report.Files[0].PathEncoding != "base64" || report.Details[0].PathEncoding != "base64" {
		t.Fatalf("%+v", report.Files)
	}
}

func TestDiffSubmoduleAndUninitializedCheckout(t *testing.T) {
	diffFixture(t)
	source := t.TempDir()
	diffTestGit(t, "-C", source, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diffTestGit(t, "-C", source, "add", ".")
	diffTestGit(t, "-C", source, "-c", "user.name=Diff Test", "-c", "user.email=diff@example.invalid", "commit", "-m", "submodule initial")
	first := diffTestGit(t, "-C", source, "rev-parse", "HEAD")
	diffTestGit(t, "-c", "protocol.file.allow=always", "submodule", "add", source, "module")
	diffTestGit(t, "commit", "-am", "add module")
	diffTestWrite(t, "module/.gitattributes", "file.txt filter=submodule-driver\n")
	diffTestGit(t, "-C", "module", "config", "filter.submodule-driver.clean", "touch submodule-filter-executed; cat")
	diffTestWrite(t, "module/file.txt", "dirty\n")
	report := diffCollect(t, "head", 10)
	f, d := diffFind(t, report, "module")
	if f.Additions != nil || d.Content.Kind != "submodule" || !d.Content.Dirty || *d.Content.NewCommit != first {
		t.Fatalf("%+v %+v", f, d.Content)
	}
	if _, err := os.Stat("module/submodule-filter-executed"); !os.IsNotExist(err) {
		t.Fatalf("submodule filter executed: %v", err)
	}
	diffTestGit(t, "-C", "module", "config", "--unset", "filter.submodule-driver.clean")
	if err := os.Remove("module/.gitattributes"); err != nil {
		t.Fatal(err)
	}
	diffTestGit(t, "-C", "module", "checkout", "--", "file.txt")
	// Move the gitlink while leaving an intentionally uninitialized checkout.
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diffTestGit(t, "-C", source, "-c", "user.name=Diff Test", "-c", "user.email=diff@example.invalid", "commit", "-am", "second")
	second := diffTestGit(t, "-C", source, "rev-parse", "HEAD")
	diffTestGit(t, "submodule", "deinit", "-f", "module")
	diffTestGit(t, "update-index", "--cacheinfo", "160000,"+second+",module")
	report = diffCollect(t, "head", 10)
	_, d = diffFind(t, report, "module")
	if *d.Content.OldCommit != first || *d.Content.NewCommit != second || d.Content.Dirty {
		t.Fatalf("%+v", d.Content)
	}
}

func TestDiffFutureHistoryAndDeletedInvalidText(t *testing.T) {
	diffFixture(t)
	diffTestWrite(t, "invalid.txt", "\xffold\n")
	diffTestGit(t, "add", ".")
	diffTestGit(t, "commit", "-m", "invalid text")
	if err := os.Remove("invalid.txt"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	report, err := collectDiff(context.Background(), diffOptions{Against: "head", Context: 10}, now)
	if err != nil {
		t.Fatal(err)
	}
	f, d := diffFind(t, report, "invalid.txt")
	if !f.Binary || d.Content.OldBytes == nil || *d.Content.OldBytes != 5 || *d.FileHistory.DaysSinceLastCommittedUpdate != 0 || len(report.Warnings) == 0 {
		t.Fatalf("%+v %+v %v", f, d, report.Warnings)
	}
}
