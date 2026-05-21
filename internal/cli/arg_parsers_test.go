// purpose: Validate strict CLI argument parsing so malformed or ambiguous invocations fail with predictable errors.
// responsibilities: Assert parse outcomes and error branches for repo, release, build, and config parser helpers.
// architecture notes: Tests target pure parser functions directly to provide fast coverage of contract-level CLI behavior.
package cli

import (
	"path/filepath"
	"testing"
)

func TestParseRepoArgsAndHelpers(t *testing.T) {
	if repo, overwrite, err := parseRepoInitArgs([]string{"--repo", "o/r", "--overwrite"}); err != nil || repo != "o/r" || !overwrite {
		t.Fatalf("unexpected init parse: repo=%q overwrite=%v err=%v", repo, overwrite, err)
	}
	if _, _, err := parseRepoInitArgs([]string{"--repo"}); err == nil {
		t.Fatalf("expected init parse error")
	}
	if _, _, err := parseRepoInitArgs([]string{"--x"}); err == nil {
		t.Fatalf("expected init unknown arg error")
	}

	if repo, asJSON, err := parseRepoAuditArgs([]string{"--repo", "o/r", "--json"}); err != nil || repo != "o/r" || !asJSON {
		t.Fatalf("unexpected audit parse: repo=%q json=%v err=%v", repo, asJSON, err)
	}
	if _, _, err := parseRepoAuditArgs([]string{"--repo"}); err == nil {
		t.Fatalf("expected audit parse error")
	}

	if repo, confirm, accept, err := parseRepoUpdateArgs([]string{"--repo", "o/r", "--confirm-deletions", "--accept-visibility-change-consequences"}); err != nil || repo != "o/r" || !confirm || !accept {
		t.Fatalf("unexpected update parse: %q %v %v err=%v", repo, confirm, accept, err)
	}
	if _, _, _, err := parseRepoUpdateArgs([]string{"--repo"}); err == nil {
		t.Fatalf("expected update parse error")
	}

	if org, asJSON, err := parseReposMineArgs([]string{"--org", "flarebyte", "--json"}); err != nil || org != "flarebyte" || !asJSON {
		t.Fatalf("unexpected repos mine parse: org=%q json=%v err=%v", org, asJSON, err)
	}
	if _, _, err := parseReposMineArgs([]string{"--org"}); err == nil {
		t.Fatalf("expected repos mine parse error")
	}

	if owner, name, err := splitRepo("a/b"); err != nil || owner != "a" || name != "b" {
		t.Fatalf("unexpected splitRepo: owner=%q name=%q err=%v", owner, name, err)
	}
	if _, _, err := splitRepo("a"); err == nil {
		t.Fatalf("expected splitRepo error")
	}

	if abs := mustAbs("."); abs == "." {
		t.Fatalf("expected absolute path from mustAbs")
	}
}

func TestParseReleaseBuildAndConfigArgs(t *testing.T) {
	if draft, notesFile, err := parseReleaseArgs([]string{"--draft", "--notes-file", "notes.md"}); err != nil || !draft || notesFile != "notes.md" {
		t.Fatalf("unexpected parseReleaseArgs: draft=%v notes=%q err=%v", draft, notesFile, err)
	}
	if _, _, err := parseReleaseArgs([]string{"--notes-file"}); err == nil {
		t.Fatalf("expected release missing notes path error")
	}
	if _, _, err := parseReleaseArgs([]string{"--bad"}); err == nil {
		t.Fatalf("expected release unknown arg error")
	}

	if target, outDir, err := parseBuildArgs([]string{"--target", "linux-amd64", "--output-dir", "dist"}); err != nil || target != "linux-amd64" || outDir != "dist" {
		t.Fatalf("unexpected parseBuildArgs: target=%q outDir=%q err=%v", target, outDir, err)
	}
	if _, _, err := parseBuildArgs([]string{"--target"}); err == nil {
		t.Fatalf("expected build missing target error")
	}
	if _, _, err := parseBuildArgs([]string{"--output-dir"}); err == nil {
		t.Fatalf("expected build missing output-dir value error")
	}
	if _, _, err := parseBuildArgs([]string{"--bad"}); err == nil {
		t.Fatalf("expected build unknown arg error")
	}

	if path, err := parseConfigPath([]string{"--config", filepath.Join("a", "b.cue")}); err != nil || path == "" {
		t.Fatalf("unexpected parseConfigPath: path=%q err=%v", path, err)
	}
	if _, err := parseConfigPath([]string{"--config"}); err == nil {
		t.Fatalf("expected config missing path error")
	}
	if _, err := parseConfigPath([]string{"--bad"}); err == nil {
		t.Fatalf("expected config unknown arg error")
	}
}
