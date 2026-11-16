package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependencyCredentialEnvironment(t *testing.T) {
	t.Setenv("GH_TOKEN", "fixture-gh-token")
	t.Setenv("GITHUB_TOKEN", "fixture-github-token")
	lookup := dependencyGitHubToken(t.TempDir())
	token, err := lookup(context.Background())
	if err != nil || token != "fixture-gh-token" {
		t.Fatal("environment credential preference")
	}
	t.Setenv("GH_TOKEN", "changed")
	token, err = lookup(context.Background())
	if err != nil || token != "fixture-gh-token" {
		t.Fatal("credential cache")
	}
	t.Setenv("GH_TOKEN", "")
	lookup = dependencyGitHubToken(t.TempDir())
	token, err = lookup(context.Background())
	if err != nil || token != "fixture-github-token" {
		t.Fatal("fallback environment credential")
	}
	for _, value := range []string{"bad\ntoken", strings.Repeat("x", 4097)} {
		t.Setenv("GH_TOKEN", value)
		if token, err := dependencyGitHubToken(t.TempDir())(context.Background()); err == nil || token != "" {
			t.Fatal("invalid credential accepted")
		}
	}
}

func TestDependencyCredentialExecutableBoundary(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nprintf 'repository-token'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if token, err := dependencyGitHubToken(root)(context.Background()); err == nil || token != "" {
		t.Fatal("repository executable ran")
	}
	t.Setenv("PATH", t.TempDir())
	if token, err := dependencyGitHubToken(root)(context.Background()); err == nil || token != "" {
		t.Fatal("missing executable accepted")
	}
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "gh"), []byte("#!/bin/sh\n[ \"$*\" = 'auth token --hostname github.com' ] || exit 1\nprintf 'fixture-token\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", external)
	if token, err := dependencyGitHubToken(root)(context.Background()); err != nil || token != "fixture-token" {
		t.Fatalf("fixture credential lookup failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(external, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if token, err := dependencyGitHubToken(root)(context.Background()); err == nil || token != "" {
		t.Fatal("failed credential command accepted")
	}
	var output tokenOutput
	if _, err := output.Write(make([]byte, 4097)); err == nil {
		t.Fatal("unbounded credential output")
	}
}
