package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDepCargoAdviceAndDelegation(t *testing.T) {
	testPackageAdviceAndDelegation(t, "cargo", "my_crate", "1.2.3")
}

func TestDepCargoNativeInputAndFailures(t *testing.T) {
	stubDependencyExecutable(t, "cargo", "IFS= read -r line\nprintf '%s:%s:%s' \"$line\" \"$CARGO_HOME\" \"$2\"\n")
	t.Setenv("CARGO_HOME", "/example/cargo-home")
	var out, stderr bytes.Buffer
	r := runDelegatedCargo(context.Background(), t.TempDir(), "my_crate", "1.2.3+build.1", strings.NewReader("hello\n"), &out, &stderr)
	if r.ExitCode != 0 || out.String() != "hello:/example/cargo-home:my_crate@=1.2.3+build.1" {
		t.Fatal(r, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	r = runDelegatedCargo(context.Background(), t.TempDir(), "my_crate;bad", "1.2.3", nil, &out, &stderr)
	if r.ExitCode != ExitUsage || out.Len() != 0 {
		t.Fatal(r)
	}
	t.Setenv("PATH", t.TempDir())
	r = runDelegatedCargo(context.Background(), t.TempDir(), "my_crate", "1.2.3", nil, &out, &stderr)
	if r.ExitCode != ExitDependencyFailure || out.Len() != 0 {
		t.Fatal(r)
	}
}
