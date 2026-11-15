// purpose: Collect baseline-to-working-tree changes using read-only Git plumbing.
package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type diffGit struct {
	ctx    context.Context
	root   string
	config []string
}

func (g diffGit) run(args ...string) ([]byte, error) {
	name := args[0]
	args = append(append([]string{"--no-pager", "--literal-pathspecs", "-c", "core.quotePath=true", "-c", "core.fsmonitor=false", "-c", "diff.suppressBlankEmpty=false"}, g.config...), args...)
	cmd := exec.CommandContext(g.ctx, "git", args...)
	cmd.Dir = g.root
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	var out, diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &diagnostic
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w (%s)", name, err, strconv.Quote(strings.TrimSpace(diagnostic.String())))
	}
	return out.Bytes(), nil
}
func (g diffGit) text(args ...string) (string, error) {
	b, err := g.run(args...)
	return strings.TrimSpace(string(b)), err
}
func diffArgs() []string {
	return []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--ignore-submodules=none", "--submodule=short", "--full-index", "--output-indicator-new=+", "--output-indicator-old=-", "--output-indicator-context= ", "--src-prefix=a/", "--dst-prefix=b/", "--diff-algorithm=myers", "--no-indent-heuristic", "--inter-hunk-context=0"}
}
func (g diffGit) state() ([]byte, error) {
	return g.run("status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
}

type diffChange struct {
	path, status, oldMode, newMode, oldID, newID string
	untracked                                    bool
	recreated                                    bool
}

func parseDiffRaw(data []byte) ([]diffChange, error) {
	parts := bytes.Split(data, []byte{0})
	changes := []diffChange{}
	for i := 0; i < len(parts)-1; i += 2 {
		if i+1 >= len(parts)-1 {
			return nil, fmt.Errorf("invalid Git raw diff")
		}
		fields := strings.Fields(string(parts[i]))
		if len(fields) != 5 || !strings.HasPrefix(fields[0], ":") {
			return nil, fmt.Errorf("invalid Git raw diff record")
		}
		status := "updated"
		if fields[4] == "A" {
			status = "created"
		}
		if fields[4] == "D" {
			status = "deleted"
		}
		changes = append(changes, diffChange{path: string(parts[i+1]), status: status, oldMode: fields[0][1:], newMode: fields[1], oldID: fields[2], newID: fields[3]})
	}
	return changes, nil
}
func diffPath(path string) (string, string) {
	if utf8.ValidString(path) {
		return path, ""
	}
	return base64.StdEncoding.EncodeToString([]byte(path)), "base64"
}
func diffMode(mode string) *string {
	if mode == "000000" || mode == "" {
		return nil
	}
	return &mode
}
func diffInt(n int) *int          { return &n }
func diffString(s string) *string { return &s }

func collectDiff(ctx context.Context, opts diffOptions, now time.Time) (diffReport, error) {
	report := diffReport{SchemaVersion: 1, Files: []diffFile{}}
	g := diffGit{ctx: ctx, root: "."}
	rootBytes, err := g.run("rev-parse", "--show-toplevel")
	root := strings.TrimSuffix(string(rootBytes), "\n")
	if err != nil {
		return report, fmt.Errorf("run inside a Git working tree: %w", err)
	}
	g.root = root
	if err := g.disableFilters(); err != nil {
		return report, err
	}
	if err := g.disableSubmoduleFilters(); err != nil {
		return report, err
	}
	head, err := g.text("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return report, fmt.Errorf("a committed HEAD is required: %w", err)
	}
	baseline, ref := head, "HEAD"
	if opts.Against == "main" {
		if _, err = g.text("rev-parse", "--verify", "refs/heads/main^{commit}"); err != nil {
			return report, fmt.Errorf("local main is required: %w", err)
		}
		bases, baseErr := g.text("merge-base", "--all", "HEAD", "refs/heads/main")
		if baseErr != nil || len(strings.Fields(bases)) != 1 {
			return report, fmt.Errorf("expected one merge base with local main; check history (deepen a shallow clone if necessary)")
		}
		baseline, ref = bases, "refs/heads/main"
	}
	conflicts, err := g.run("ls-files", "--unmerged", "-z")
	if err != nil {
		return report, err
	}
	if len(conflicts) > 0 {
		paths := map[string]bool{}
		for _, entry := range bytes.Split(conflicts, []byte{0}) {
			if _, p, ok := bytes.Cut(entry, []byte{'\t'}); ok {
				paths[strconv.Quote(string(p))] = true
			}
		}
		names := []string{}
		for p := range paths {
			names = append(names, p)
		}
		sort.Strings(names)
		return report, fmt.Errorf("resolve unmerged paths before comparing: %s", strings.Join(names, ", "))
	}
	before, err := g.state()
	if err != nil {
		return report, err
	}
	args := append(diffArgs(), "--raw", "-z", "--no-abbrev", baseline, "--")
	raw, err := g.run(args...)
	if err != nil {
		return report, err
	}
	changes, err := parseDiffRaw(raw)
	if err != nil {
		return report, err
	}
	untracked, err := g.run("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return report, err
	}
	for _, p := range bytes.Split(untracked, []byte{0}) {
		if len(p) > 0 {
			changes = append(changes, diffChange{path: string(p), status: "created", untracked: true})
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].path < changes[j].path })
	// A path deleted from the index and recreated untracked is one net change.
	unique := changes[:0]
	for _, c := range changes {
		if len(unique) > 0 && unique[len(unique)-1].path == c.path {
			old := unique[len(unique)-1]
			old.untracked = false
			old.recreated = true
			old.status = "updated"
			old.newMode = ""
			unique[len(unique)-1] = old
		} else {
			unique = append(unique, c)
		}
	}
	changes = unique
	local, err := g.run(append(diffArgs(), "--name-only", "-z", head, "--")...)
	if err != nil {
		return report, err
	}
	localPaths := map[string]bool{}
	for _, p := range bytes.Split(append(append(local, 0), untracked...), []byte{0}) {
		if len(p) > 0 {
			localPaths[string(p)] = true
		}
	}
	comparison := &diffComparison{Mode: opts.Against, RequestedBase: opts.Against, ResolvedRef: ref, BaselineCommit: baseline, HeadCommit: head, Target: "working-tree", AgeAsOf: now}
	if branch, e := g.text("symbolic-ref", "--quiet", "--short", "HEAD"); e == nil {
		comparison.Branch = &branch
	}
	report.Comparison = comparison
	report.Summary = &diffSummary{}
	fingerprints := map[string][32]byte{}
	for _, change := range changes {
		file, detail, hash, e := g.collectFile(change, baseline, head, opts.Context, now, localPaths[change.path], &report.Warnings)
		if e != nil {
			return report, e
		}
		// Index deletions with an identical recreated file have no effective change.
		if file.Status == "updated" && detail.Content.Kind == "unified-diff" && detail.Content.Text != nil && *detail.Content.Text == "" {
			continue
		}
		report.Files = append(report.Files, file)
		report.Details = append(report.Details, detail)
		if hash != nil {
			fingerprints[change.path] = *hash
		}
		switch file.Status {
		case "created":
			report.Summary.Created++
		case "deleted":
			report.Summary.Deleted++
		default:
			report.Summary.Updated++
		}
		if file.Additions == nil || file.Deletions == nil {
			report.Summary.FilesWithUnknownLineCounts++
		} else {
			report.Summary.Insertions += *file.Additions
			report.Summary.Deletions += *file.Deletions
		}
	}
	report.HasChanges = len(report.Files) > 0
	report.Summary.FilesChanged = len(report.Files)
	if report.HasChanges {
		if err = g.enrichMetadata(&report); err != nil {
			return report, err
		}
	}
	after, err := g.state()
	if err != nil {
		return report, err
	}
	currentHead, err := g.text("rev-parse", "--verify", "HEAD")
	if err != nil {
		return report, err
	}
	if !bytes.Equal(before, after) || currentHead != head {
		return report, fmt.Errorf("repository changed during collection; rerun diff")
	}
	for path, want := range fingerprints {
		b, _, e := g.readTarget(path)
		if e != nil || sha256.Sum256(b) != want {
			return report, fmt.Errorf("file %q changed during collection; rerun diff", path)
		}
	}
	if !report.HasChanges {
		report.Comparison = nil
		report.Summary = nil
		report.Details = nil
		report.Warnings = nil
	}
	return report, nil
}

// Read symlink targets without following them. Reject symlink ancestors and
// special files. Intentionally absent sparse paths use their index blob.
func (g diffGit) readTarget(path string) ([]byte, string, error) {
	parts := strings.Split(filepath.FromSlash(path), string(filepath.Separator))
	prefix := g.root
	for _, part := range parts[:len(parts)-1] {
		prefix = filepath.Join(prefix, part)
		if info, e := os.Lstat(prefix); e == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, "", fmt.Errorf("refusing symlink ancestor for %q", path)
		}
	}
	full := filepath.Join(g.root, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		entry, e := g.run("ls-files", "-t", "--", path)
		if e == nil && bytes.HasPrefix(entry, []byte("S ")) {
			b, e := g.run("cat-file", "blob", ":"+path)
			return b, "100644", e
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf("read %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, e := os.Readlink(full)
		return []byte(target), "120000", e
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("unsupported nonregular file %q", path)
	}
	mode := "100644"
	if info.Mode()&0o111 != 0 {
		mode = "100755"
	}
	b, err := os.ReadFile(full)
	return b, mode, err
}
func (g diffGit) fileHistory(path, head, baseline string, now time.Time, local bool, warnings *[]string) (diffHistory, error) {
	history := diffHistory{HasLocalChanges: local}
	for _, ref := range []string{head, baseline} {
		stamp, err := g.text("log", "--no-show-signature", "--no-notes", "-1", "--format=%cI", ref, "--", path)
		if err != nil {
			return history, err
		}
		if stamp == "" {
			continue
		}
		updated, err := time.Parse(time.RFC3339, stamp)
		if err != nil {
			return history, fmt.Errorf("invalid Git commit timestamp for %q: %w", path, err)
		}
		updated = updated.UTC()
		days := int(now.Sub(updated).Hours() / 24)
		if updated.After(now) {
			days = 0
			*warnings = append(*warnings, fmt.Sprintf("future commit timestamp for %q; file age clamped to zero", path))
		}
		label := "HEAD"
		if ref != head {
			label = "baseline"
		}
		history.LastCommittedUpdate = &updated
		history.DaysSinceLastCommittedUpdate = &days
		history.HistoryRef = &label
		break
	}
	return history, nil
}

func (g diffGit) collectFile(c diffChange, baseline, head string, context int, now time.Time, local bool, warnings *[]string) (diffFile, diffDetail, *[32]byte, error) {
	path, encoding := diffPath(c.path)
	file := diffFile{Path: path, PathEncoding: encoding, Status: c.status, OldMode: diffMode(c.oldMode), NewMode: diffMode(c.newMode)}
	detail := diffDetail{Path: path, PathEncoding: encoding, Metadata: []diffMetadata{}}
	var data []byte
	var hash *[32]byte
	var err error
	if c.newMode != "160000" && c.status != "deleted" {
		var mode string
		data, mode, err = g.readTarget(c.path)
		if err != nil {
			return file, detail, nil, err
		}
		if c.newMode == "" {
			file.NewMode = &mode
		}
		h := sha256.Sum256(data)
		hash = &h
	}
	detail.FileHistory, err = g.fileHistory(c.path, head, baseline, now, local, warnings)
	if err != nil {
		return file, detail, nil, err
	}
	if c.newMode == "160000" || c.oldMode == "160000" {
		detail.Content = diffContent{Kind: "submodule", OldMode: file.OldMode, NewMode: file.NewMode}
		if c.oldMode == "160000" {
			detail.Content.OldCommit = diffString(c.oldID)
		}
		if c.newMode == "160000" {
			detail.Content.NewCommit = diffString(c.newID)
			entry, e := g.run("ls-files", "--stage", "-z", "--", c.path)
			if e != nil {
				return file, detail, nil, e
			}
			fields := strings.Fields(string(bytes.SplitN(entry, []byte{'\t'}, 2)[0]))
			if len(fields) >= 2 && fields[0] == "160000" {
				detail.Content.NewCommit = &fields[1]
			}
			sub := diffGit{ctx: g.ctx, root: filepath.Join(g.root, c.path), config: g.config}
			// An uninitialized submodule can otherwise resolve its parent's HEAD.
			if info, e := os.Lstat(sub.root); e == nil && info.IsDir() {
				top, e := sub.text("rev-parse", "--show-toplevel")
				if e == nil && filepath.Clean(top) == filepath.Clean(sub.root) {
					if e = sub.disableFilters(); e != nil {
						return file, detail, nil, e
					}
					if commit, e := sub.text("rev-parse", "--verify", "HEAD"); e == nil {
						detail.Content.NewCommit = &commit
					}
					state, e := sub.state()
					if e != nil {
						return file, detail, nil, e
					}
					detail.Content.Dirty = len(state) > 0
				}
			}
		} else if c.status != "deleted" {
			if utf8.Valid(data) && !bytes.ContainsRune(data, 0) {
				detail.Content.Text = diffString(string(data))
			} else {
				file.Binary = true
				size := int64(len(data))
				detail.Content.NewBytes = &size
			}
		}
		return file, detail, hash, nil
	}
	if c.recreated {
		old, e := g.run("cat-file", "blob", baseline+":"+c.path)
		if e != nil {
			return file, detail, nil, e
		}
		return g.collectRecreated(file, detail, c.path, old, data, context, hash)
	}

	if c.untracked {
		attrs, e := g.run("check-attr", "-z", "diff", "--", c.path)
		if e != nil {
			return file, detail, nil, e
		}
		fields := bytes.Split(attrs, []byte{0})
		file.Binary = !utf8.Valid(data) || bytes.ContainsRune(data, 0) || (len(fields) > 2 && string(fields[2]) == "unset")
		file.OldMode = nil
		if !file.Binary {
			file.Additions = diffInt(diffLineCount(data))
			file.Deletions = diffInt(0)
		}
	} else {
		stats, e := g.run(append(diffArgs(), "--numstat", "-z", baseline, "--", c.path)...)
		if e != nil {
			return file, detail, nil, e
		}
		fields := bytes.SplitN(stats, []byte{'\t'}, 3)
		if len(fields) == 3 && string(fields[0]) != "-" {
			a, e1 := strconv.Atoi(string(fields[0]))
			d, e2 := strconv.Atoi(string(fields[1]))
			if e1 != nil || e2 != nil {
				return file, detail, nil, fmt.Errorf("invalid Git line counts for %q", c.path)
			}
			file.Additions = &a
			file.Deletions = &d
		} else {
			file.Binary = len(stats) > 0
		}
		if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
			file.Binary = true
			file.Additions = nil
			file.Deletions = nil
		}
	}
	if file.Binary {
		detail.Content = diffContent{Kind: "binary"}
	} else if c.status == "created" {
		detail.Content = diffContent{Kind: "full-file", Text: diffString(string(data))}
	} else {
		patch, e := g.run(append(diffArgs(), "--unified="+strconv.Itoa(context), baseline, "--", c.path)...)
		if e != nil {
			return file, detail, nil, e
		}
		if !utf8.Valid(patch) {
			file.Binary = true
			file.Additions = nil
			file.Deletions = nil
			detail.Content = diffContent{Kind: "binary"}
		} else {
			kind := "unified-diff"
			if !bytes.Contains(patch, []byte("@@")) && file.OldMode != nil && file.NewMode != nil && *file.OldMode != *file.NewMode {
				kind = "mode-change"
				file.Additions = diffInt(0)
				file.Deletions = diffInt(0)
			}
			detail.Content = diffContent{Kind: kind, Text: diffString(string(patch)), OldMode: file.OldMode, NewMode: file.NewMode}
		}
	}
	if file.Binary {
		if file.NewMode != nil {
			size := int64(len(data))
			detail.Content.NewBytes = &size
		}
		if file.OldMode != nil {
			size, e := g.text("cat-file", "-s", baseline+":"+c.path)
			if e != nil {
				return file, detail, nil, e
			}
			n, e := strconv.ParseInt(size, 10, 64)
			if e != nil {
				return file, detail, nil, e
			}
			detail.Content.OldBytes = &n
		}
	}

	return file, detail, hash, nil
}
func diffLineCount(data []byte) int {
	n := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// Disable attribute-selected clean/process filters as well as diff/textconv
// drivers. The report reads repository data without running repository hooks.
func (g *diffGit) disableFilters() error {
	names, err := g.run("config", "--null", "--name-only", "--get-regexp", `^filter\..*\.(clean|smudge|process|required)$`)
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil
		}
		return err
	}
	for _, name := range bytes.Split(names, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		value := ""
		if bytes.HasSuffix(name, []byte(".required")) {
			value = "false"
		}
		g.config = append(g.config, "-c", string(name)+"="+value)
	}
	return nil
}

// Git omits untracked content for an index-deleted path. Compare its two byte
// snapshots outside the repository instead; never stage it or write Git objects.
func (g diffGit) collectRecreated(file diffFile, detail diffDetail, path string, old, data []byte, context int, hash *[32]byte) (diffFile, diffDetail, *[32]byte, error) {
	attrs, err := g.run("check-attr", "-z", "diff", "--", path)
	if err != nil {
		return file, detail, nil, err
	}
	fields := bytes.Split(attrs, []byte{0})
	file.Binary = !utf8.Valid(old) || !utf8.Valid(data) || bytes.ContainsRune(old, 0) || bytes.ContainsRune(data, 0) || (len(fields) > 2 && string(fields[2]) == "unset")
	if bytes.Equal(old, data) && file.OldMode != nil && file.NewMode != nil && *file.OldMode == *file.NewMode {
		detail.Content = diffContent{Kind: "unified-diff", Text: diffString("")}
		return file, detail, hash, nil
	}
	if file.Binary {
		a, b := int64(len(old)), int64(len(data))
		detail.Content = diffContent{Kind: "binary", OldBytes: &a, NewBytes: &b}
		return file, detail, hash, nil
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-diff-")
	if err != nil {
		return file, detail, nil, err
	}
	defer os.RemoveAll(dir)
	oldPath, newPath := filepath.Join(dir, "old"), filepath.Join(dir, "new")
	if err = os.WriteFile(oldPath, old, 0o600); err != nil {
		return file, detail, nil, err
	}
	if err = os.WriteFile(newPath, data, 0o600); err != nil {
		return file, detail, nil, err
	}
	// --no-index returns 1 when differences exist. Capture that successful result.
	cmd := exec.CommandContext(g.ctx, "git", "--no-pager", "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--diff-algorithm=myers", "--no-indent-heuristic", "--inter-hunk-context=0", "--unified="+strconv.Itoa(context), "--", oldPath, newPath)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	patch, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return file, detail, nil, fmt.Errorf("compare recreated %q: %w", path, err)
		}
	}
	start := bytes.Index(patch, []byte("@@"))
	a, d := 0, 0
	body := []byte{}
	if start >= 0 {
		body = patch[start:]
		for _, line := range bytes.Split(body, []byte{'\n'}) {
			if len(line) > 0 {
				if line[0] == '+' {
					a++
				}
				if line[0] == '-' {
					d++
				}
			}
		}
	}
	file.Additions = &a
	file.Deletions = &d
	var header strings.Builder
	fmt.Fprintf(&header, "diff --git %s %s\n", strconv.Quote("a/"+path), strconv.Quote("b/"+path))
	kind := "unified-diff"
	if file.OldMode != nil && file.NewMode != nil && *file.OldMode != *file.NewMode {
		fmt.Fprintf(&header, "old mode %s\nnew mode %s\n", *file.OldMode, *file.NewMode)
		if start < 0 {
			kind = "mode-change"
		}
	}
	if start >= 0 {
		fmt.Fprintf(&header, "--- %s\n+++ %s\n", strconv.Quote("a/"+path), strconv.Quote("b/"+path))
	}
	detail.Content = diffContent{Kind: kind, Text: diffString(header.String() + string(body)), OldMode: file.OldMode, NewMode: file.NewMode}
	return file, detail, hash, nil
}

// Parent Git status/diff can inspect initialized submodules. Disable their
// configured filters before that happens, including filters in nested modules.
func (g *diffGit) disableSubmoduleFilters() error {
	entries, err := g.run("ls-files", "--stage", "-z")
	if err != nil {
		return err
	}
	for _, entry := range bytes.Split(entries, []byte{0}) {
		header, name, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok || !bytes.HasPrefix(header, []byte("160000 ")) {
			continue
		}
		full := g.root
		safe := true
		for _, part := range strings.Split(string(name), "/") {
			full = filepath.Join(full, part)
			info, e := os.Lstat(full)
			if e != nil || !info.IsDir() {
				safe = false
				break
			}
		}
		if !safe {
			continue
		}
		sub := diffGit{ctx: g.ctx, root: full}
		top, e := sub.text("rev-parse", "--show-toplevel")
		if e != nil || filepath.Clean(top) != filepath.Clean(full) {
			continue
		}
		if e = sub.disableFilters(); e != nil {
			return e
		}
		if e = sub.disableSubmoduleFilters(); e != nil {
			return e
		}
		g.config = append(g.config, sub.config...)
	}
	return nil
}
