package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDepGoAdviceAndDelegation(t *testing.T) {
	testPackageAdviceAndDelegation(t, "go", "github.com/org/repo", "v1.2.3")
}

func TestDepGoNativeInputAndFailures(t *testing.T) {
	stubDependencyExecutable(t, "go", "IFS= read -r line\nprintf '%s:%s' \"$line\" \"$GOPROXY\"\n")
	t.Setenv("GOPROXY", "https://example.test/proxy")
	var out, stderr bytes.Buffer
	r := runDelegatedGo(context.Background(), t.TempDir(), "github.com/org/repo/v2", "v2.0.0", strings.NewReader("hello\n"), &out, &stderr)
	if r.ExitCode != 0 || out.String() != "hello:https://example.test/proxy" || stderr.Len() != 0 {
		t.Fatal(r, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	r = runDelegatedGo(context.Background(), t.TempDir(), "github.com/org/repo;bad", "v2.0.0", nil, &out, &stderr)
	if r.ExitCode != ExitUsage || out.Len() != 0 {
		t.Fatal(r)
	}
	t.Setenv("PATH", t.TempDir())
	r = runDelegatedGo(context.Background(), t.TempDir(), "github.com/org/repo", "v1.0.0", nil, &out, &stderr)
	if r.ExitCode != ExitDependencyFailure || out.Len() != 0 {
		t.Fatal(r)
	}
}
