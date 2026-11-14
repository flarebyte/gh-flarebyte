package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func metaWrite(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

func metaFixture(t *testing.T, script string) {
	t.Helper()
	t.Chdir(t.TempDir())
	bin := t.TempDir()
	metaWrite(t, filepath.Join(bin, "maat"), "#!/bin/sh\nexit 0\n")
	metaWrite(t, filepath.Join(bin, "thoth"), "#!/bin/sh\n"+script)
	t.Setenv("PATH", bin)
}

func TestMetaMissingTools(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	var out, stderr bytes.Buffer
	result := Run([]string{"meta"}, &out, &stderr)
	if result.ExitCode != ExitFailure || out.Len() != 0 ||
		!strings.Contains(stderr.String(), "brew install flarebyte/tap/thoth-ostraca") ||
		!strings.Contains(stderr.String(), "npm install --global maat-ostraca") {
		t.Fatalf("%+v %s %s", result, &out, &stderr)
	}
}

func TestMetaNoPipelines(t *testing.T) {
	metaFixture(t, "exit 99\n")
	// Nested pipelines are outside the initial discovery scope.
	metaWrite(t, "nested/pipeline-go-maat.thoth.cue", "invalid")
	for _, configured := range []bool{false, true} {
		if configured {
			metaWrite(t, ".gh-flarebyte.cue", testConfigCue())
		}
		var out, stderr bytes.Buffer
		result := Run([]string{"meta"}, &out, &stderr)
		want := "pipeline-<scope>-maat.thoth.cue"
		if configured {
			want = "pipeline-go-maat.thoth.cue"
		}
		if result.ExitCode != ExitOK || !strings.Contains(out.String(), "no-op") || !strings.Contains(out.String(), want) || stderr.Len() != 0 {
			t.Fatalf("%+v %s %s", result, &out, &stderr)
		}
		if _, err := os.Stat(want); !os.IsNotExist(err) {
			t.Fatalf("suggestion created: %v", err)
		}
	}
}

func TestMetaRunPartialFailure(t *testing.T) {
	metaFixture(t, `
[ "$1" = run ] && [ "$2" = --config ] || exit 9
printf '%s\n' "$3" >> calls
case "$3" in
  ./pipeline-go-maat.thoth.cue)
    printf changed > thoth-meta/update.yaml
    printf created > thoth-meta/new.yaml
    /bin/rm thoth-meta/delete.yaml
    printf chatter
    ;;
  ./pipeline-rust-maat.thoth.cue)
    printf partial > thoth-meta/partial.yaml
    printf 'invalid CUE\n' >&2
    exit 4
    ;;
  ./pipeline-ts-maat.thoth.cue)
    printf changed > thoth-meta/update.yaml
    printf same > thoth-meta/same.yaml
    ;;
esac
`)
	for _, language := range []string{"ts", "rust", "go"} {
		metaWrite(t, "pipeline-"+language+"-maat.thoth.cue", "invalid CUE delegated to thoth")
	}
	for _, name := range []string{"update", "delete", "same"} {
		metaWrite(t, "thoth-meta/"+name+".yaml", "same")
	}
	metaWrite(t, ".gh-flarebyte.cue", testConfigCue())
	var out, stderr bytes.Buffer
	result := Run([]string{"meta"}, &out, &stderr)
	if result.ExitCode != ExitFailure || !strings.Contains(out.String(), "Metadata: 1 updated, 2 created, 1 deleted") || !strings.Contains(out.String(), "2 succeeded, 1 failed, 0 not run") || strings.Contains(out.String(), "chatter") || !strings.Contains(stderr.String(), "invalid CUE") {
		t.Fatalf("%+v %s %s", result, &out, &stderr)
	}
	calls, err := os.ReadFile("calls")
	if err != nil || string(calls) != "./pipeline-go-maat.thoth.cue\n./pipeline-rust-maat.thoth.cue\n./pipeline-ts-maat.thoth.cue\n" {
		t.Fatalf("calls=%s err=%v", calls, err)
	}
}

func TestMetaSuccessAndArguments(t *testing.T) {
	metaFixture(t, "printf hidden\n")
	metaWrite(t, "pipeline-go-maat.thoth.cue", "")
	var out, stderr bytes.Buffer
	result := Run([]string{"meta"}, &out, &stderr)
	if result.ExitCode != ExitOK || !strings.Contains(out.String(), "0 updated, 0 created, 0 deleted") || stderr.Len() != 0 {
		t.Fatalf("%+v %s %s", result, &out, &stderr)
	}
	if result := Run([]string{"meta", "extra"}, &out, &stderr); result.ExitCode != ExitUsage {
		t.Fatal(result)
	}
	if result := Run([]string{"meta", "--help"}, &out, &stderr); result.ExitCode != ExitOK {
		t.Fatal(result)
	}
}

func TestMetaUnsafePaths(t *testing.T) {
	metaFixture(t, "exit 0\n")
	outside := t.TempDir()
	metaWrite(t, filepath.Join(outside, "source"), "do not execute")
	if err := os.Symlink(filepath.Join(outside, "source"), "pipeline-escape-maat.thoth.cue"); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if result := Run([]string{"meta"}, &out, &stderr); result.ExitCode != ExitFailure || !strings.Contains(stderr.String(), "regular file") {
		t.Fatalf("%+v %s", result, &stderr)
	}
	if err := os.Symlink(outside, "thoth-meta"); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotMeta(); err == nil {
		t.Fatal("accepted metadata symlink")
	}
}

func TestMetaFailedSummaryAndCancellation(t *testing.T) {
	metaFixture(t, "/bin/ln -s / thoth-meta\n")
	metaWrite(t, "pipeline-a-maat.thoth.cue", "")
	var out, stderr bytes.Buffer
	if result := Run([]string{"meta"}, &out, &stderr); result.ExitCode != ExitFailure || !strings.Contains(stderr.String(), "summary unavailable") {
		t.Fatalf("%+v %s", result, &stderr)
	}
	if err := os.Remove("thoth-meta"); err != nil {
		t.Fatal(err)
	}
	bin := filepath.SplitList(os.Getenv("PATH"))[0]
	metaWrite(t, filepath.Join(bin, "thoth"), "#!/bin/sh\nexec /bin/sleep 10\n")
	metaWrite(t, "pipeline-b-maat.thoth.cue", "")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	out.Reset()
	stderr.Reset()
	result := runMeta(ctx, &out, &stderr)
	if result.ExitCode != ExitFailure || !strings.Contains(out.String(), "1 not run") || !strings.Contains(stderr.String(), "interrupted") {
		t.Fatalf("%+v %s %s", result, &out, &stderr)
	}
}

func TestMetaBoundedDiagnostics(t *testing.T) {
	metaFixture(t, "i=0; while [ $i -lt 1000 ]; do printf 'failure\\033[31m'; i=$((i+1)); done; exit 1\n")
	metaWrite(t, "pipeline-go-maat.thoth.cue", "")
	var out, stderr bytes.Buffer
	result := runMeta(context.Background(), &out, &stderr)
	if result.ExitCode != ExitFailure || !strings.Contains(stderr.String(), "diagnostics truncated") || stderr.Len() > 5000 || strings.ContainsRune(stderr.String(), '\x1b') {
		t.Fatalf("%+v output length=%d stderr=%s stdout=%s", result, stderr.Len(), &stderr, &out)
	}
}
