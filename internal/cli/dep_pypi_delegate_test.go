package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDepPyPIAdviceAndDelegation(t *testing.T) {
	testPackageAdviceAndDelegation(t, "pypi", "my-pkg", "1.2.3")
}

func TestDepPyPINormalizesNativeName(t *testing.T) {
	localDependencyRepo(t, "")
	stubDependencyExecutable(t, "uv", "printf '%s\\n' \"$@\"\n")
	stubPackageAdviceAPI(t, false)
	var out, stderr bytes.Buffer
	r := Run([]string{"dep", "install", "pypi", "My_Pkg@1.2.3"}, &out, &stderr)
	if r.ExitCode != 0 || out.String() != "add\nmy-pkg==1.2.3\n" || stderr.Len() != 0 {
		t.Fatal(r, out.String(), stderr.String())
	}
}

func TestDepPyPINativeInputAndFailures(t *testing.T) {
	stubDependencyExecutable(t, "uv", "IFS= read -r line\nprintf '%s:%s:%s' \"$line\" \"$UV_DEFAULT_INDEX\" \"$2\"\n")
	t.Setenv("UV_DEFAULT_INDEX", "https://example.test/simple")
	var out, stderr bytes.Buffer
	r := runDelegatedPyPI(context.Background(), t.TempDir(), "my-pkg", "1!2.3.post1", strings.NewReader("hello\n"), &out, &stderr)
	if r.ExitCode != 0 || out.String() != "hello:https://example.test/simple:my-pkg==1!2.3.post1" {
		t.Fatal(r, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	r = runDelegatedPyPI(context.Background(), t.TempDir(), "my-pkg", `1.2.3;evil`, nil, &out, &stderr)
	if r.ExitCode != ExitUsage || out.Len() != 0 {
		t.Fatal(r)
	}
	bin := t.TempDir()
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "pip"), []byte("#!/bin/sh\nprintf called > pip-called\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	r = runDelegatedPyPI(context.Background(), work, "my-pkg", "1.2.3", nil, &out, &stderr)
	if r.ExitCode != ExitDependencyFailure || out.Len() != 0 {
		t.Fatal(r)
	}
	if _, err := os.Stat(filepath.Join(work, "pip-called")); !os.IsNotExist(err) {
		t.Fatal("unexpected pip fallback")
	}
}
