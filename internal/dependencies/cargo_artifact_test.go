package dependencies

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

const simpleCrateManifest = "[package]\nname='pkg'\nversion='1.0.0'\n[lib]\npath='src/lib.rs'\n"

func sealedCrate(node ProjectNode, model *cargoFeaturePackage, b []byte) CargoArtifact {
	h := sha256.Sum256(b)
	node.Integrity = "sha256-" + hex.EncodeToString(h[:])
	node.Resolved = "https://static.crates.io/crates/" + node.Name + "/" + node.Name + "-" + node.Version + ".crate"
	return CargoArtifact{Node: node, expected: &cargoArchiveExpectation{node: node, metadata: model}}
}

func crateFixture(t testing.TB, manifest string, extra []tar.Header, bodies []string, tail []byte) ([]byte, CargoArtifact) {
	t.Helper()
	prefix := "pkg-1.0.0/"
	headers := []tar.Header{{Name: prefix, Typeflag: tar.TypeDir}, {Name: prefix + "Cargo.toml"}, {Name: prefix + "src/lib.rs"}}
	data := []string{"", manifest, ""}
	headers = append(headers, extra...)
	data = append(data, bodies...)
	b := archiveFixture(t, headers, data, tail)
	return b, sealedCrate(ProjectNode{Name: "pkg", Version: "1.0.0", Path: cargoPackagePath("pkg", "1.0.0")}, &cargoFeaturePackage{complete: true, features: map[string][]string{}}, b)
}

func TestCrateArchiveEnvelope(t *testing.T) {
	b, a := crateFixture(t, simpleCrateManifest, nil, nil, nil)
	info, err := VerifyCargoArtifact(b, a)
	if err != nil || info.Files != 2 || info.ExpandedBytes <= 0 || info.BuildScript || info.ProcMacro || "sha256-"+info.SHA256 != a.Node.Integrity {
		t.Fatal(info, err)
	}
	for _, test := range []struct {
		name    string
		entries []tar.Header
		body    []string
		tail    []byte
	}{
		{"escape", []tar.Header{{Name: "pkg-1.0.0/../outside"}}, []string{"bad"}, nil},
		{"absolute", []tar.Header{{Name: "/outside"}}, []string{"bad"}, nil},
		{"other root", []tar.Header{{Name: "different-1.0.0/a"}}, []string{"bad"}, nil},
		{"backslash", []tar.Header{{Name: `pkg-1.0.0/a\b`}}, []string{"bad"}, nil},
		{"symlink", []tar.Header{{Name: "pkg-1.0.0/link", Typeflag: tar.TypeSymlink, Linkname: "/outside"}}, nil, nil},
		{"hardlink", []tar.Header{{Name: "pkg-1.0.0/link", Typeflag: tar.TypeLink, Linkname: "pkg-1.0.0/src/lib.rs"}}, nil, nil},
		{"fifo", []tar.Header{{Name: "pkg-1.0.0/fifo", Typeflag: tar.TypeFifo}}, nil, nil},
		{"setuid", []tar.Header{{Name: "pkg-1.0.0/a", Mode: 04755}}, []string{"bad"}, nil},
		{"duplicate", []tar.Header{{Name: "pkg-1.0.0/src/lib.rs"}}, []string{"bad"}, nil},
		{"case alias", []tar.Header{{Name: "pkg-1.0.0/SRC/other.rs"}}, []string{"bad"}, nil},
		{"case manifest", []tar.Header{{Name: "pkg-1.0.0/cargo.TOML"}}, []string{"bad"}, nil},
		{"implicit parent replaced", []tar.Header{{Name: "pkg-1.0.0/src"}}, []string{"bad"}, nil},
		{"file parent", []tar.Header{{Name: "pkg-1.0.0/file"}, {Name: "pkg-1.0.0/file/child"}}, []string{"a", "b"}, nil},
		{"case build discovery", []tar.Header{{Name: "pkg-1.0.0/Build.rs"}}, []string{"bad"}, nil},
		{"control config", []tar.Header{{Name: "pkg-1.0.0/.cargo/config.toml"}}, []string{"bad"}, nil},
		{"control directory", []tar.Header{{Name: "pkg-1.0.0/.CARGO", Typeflag: tar.TypeDir}}, nil, nil},
		{"vendor checksum", []tar.Header{{Name: "pkg-1.0.0/.cargo-checksum.json"}}, []string{"{}"}, nil},
		{"git config", []tar.Header{{Name: "pkg-1.0.0/.git/config"}}, []string{"bad"}, nil},
		{"reserved device", []tar.Header{{Name: "pkg-1.0.0/AUX.rs"}}, []string{"bad"}, nil},
		{"unicode filename", []tar.Header{{Name: "pkg-1.0.0/é.rs"}}, []string{"bad"}, nil},
		{"trailing dot", []tar.Header{{Name: "pkg-1.0.0/file."}}, []string{"bad"}, nil},
		{"trailing space", []tar.Header{{Name: "pkg-1.0.0/file "}}, []string{"bad"}, nil},
		{"xattrs", []tar.Header{{Name: "pkg-1.0.0/a", PAXRecords: map[string]string{"SCHILY.xattr.security.capability": "bad"}, Format: tar.FormatPAX}}, []string{"a"}, nil},
		{"hidden tail", nil, nil, []byte("hidden")},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, a := crateFixture(t, simpleCrateManifest, test.entries, test.body, test.tail)
			if _, err := VerifyCargoArtifact(b, a); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
	for _, raw := range [][]byte{nil, []byte("bad gzip"), make([]byte, MaxArtifactBytes+1)} {
		a := sealedCrate(a.Node, a.expected.metadata, raw)
		if _, err := VerifyCargoArtifact(raw, a); err == nil {
			t.Fatal("malformed/oversized input accepted")
		}
	}
	if _, err := VerifyCargoArtifact([]byte("different"), a); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	bad := a
	bad.Node.Resolved = "https://evil.test/pkg.crate"
	if _, err := VerifyCargoArtifact(b, bad); err == nil {
		t.Fatal("mutated binding accepted")
	}
	if _, err := VerifyCargoArtifact(b, CargoArtifact{Node: a.Node}); err == nil {
		t.Fatal("unsealed artifact accepted")
	}
	corrupt := append([]byte{}, b...)
	corrupt[len(corrupt)-1] ^= 1
	if _, err := VerifyCargoArtifact(corrupt, sealedCrate(a.Node, a.expected.metadata, corrupt)); err == nil {
		t.Fatal("gzip trailer corruption accepted")
	}
	var extra bytes.Buffer
	gz := gzip.NewWriter(&extra)
	_ = gz.Close()
	joined := append(append([]byte{}, b...), extra.Bytes()...)
	if _, err := VerifyCargoArtifact(joined, sealedCrate(a.Node, a.expected.metadata, joined)); err == nil {
		t.Fatal("second gzip member accepted")
	}
	missing := archiveFixture(t, []tar.Header{{Name: "pkg-1.0.0/src/lib.rs"}}, []string{""}, nil)
	if _, err := VerifyCargoArtifact(missing, sealedCrate(a.Node, a.expected.metadata, missing)); err == nil {
		t.Fatal("missing manifest accepted")
	}
}

func TestCrateDownloadBoundary(t *testing.T) {
	b, a := crateFixture(t, simpleCrateManifest, nil, nil, nil)
	calls := 0
	c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.String() != a.Node.Resolved || r.Header.Get("Authorization") != "" {
			t.Fatal("unsafe request", r.URL)
		}
		return response(string(b), 200), nil
	})}
	got, info, err := c.FetchCargoArtifact(context.Background(), a)
	if err != nil || !bytes.Equal(got, b) || info.Files != 2 {
		t.Fatal(info, err)
	}
	bad := a
	bad.Node.Version = "2.0.0"
	if _, _, err := c.FetchCargoArtifact(context.Background(), bad); err == nil || calls != 1 {
		t.Fatal("unapproved download attempted")
	}
	for _, endpoint := range []string{"http://static.crates.io/crates/pkg/pkg-1.0.0.crate", "https://evil.test/pkg.crate", a.Node.Resolved + "?token=x", a.Node.Resolved + "#fragment", "https://user@static.crates.io/crates/pkg/pkg-1.0.0.crate", "https://static.crates.io:443/crates/pkg/pkg-1.0.0.crate"} {
		bad := a
		bad.Node.Resolved = endpoint
		bad.expected = &cargoArchiveExpectation{node: bad.Node, metadata: a.expected.metadata}
		if _, _, err := c.FetchCargoArtifact(context.Background(), bad); err == nil || calls != 1 {
			t.Fatal("foreign origin bypassed binding", endpoint)
		}
	}
	for _, change := range []func(*ProjectNode){func(n *ProjectNode) { n.Name = "../outside" }, func(n *ProjectNode) { n.Version = "1.0" }, func(n *ProjectNode) { n.Integrity = "sha256-invalid" }} {
		bad := a
		change(&bad.Node)
		bad.expected = &cargoArchiveExpectation{node: bad.Node, metadata: a.expected.metadata}
		if _, _, err := c.FetchCargoArtifact(context.Background(), bad); err == nil || calls != 1 {
			t.Fatal("invalid identity fetched")
		}
	}
	c.HTTP = doFunc(func(*http.Request) (*http.Response, error) { return response("redirect", 302), nil })
	if _, _, err := c.FetchCargoArtifact(context.Background(), a); err == nil {
		t.Fatal("redirect accepted")
	}
	c.HTTP = doFunc(func(*http.Request) (*http.Response, error) { return response("wrong", 200), nil })
	if _, _, err := c.FetchCargoArtifact(context.Background(), a); err == nil {
		t.Fatal("unverified download returned")
	}
}

func approvedCrateArtifacts(t *testing.T) (string, Report, map[string][]byte) {
	t.Helper()
	m, l := cargoDocuments()
	blobs := map[string][]byte{}
	for _, name := range []string{"pkg", "child"} {
		manifest := "[package]\nname='" + name + "'\nversion='1.0.0'\n"
		if name == "pkg" {
			manifest += "[dependencies]\nchild='1.0.0'\n"
		}
		prefix := name + "-1.0.0/"
		blobs[name] = archiveFixture(t, []tar.Header{{Name: prefix + "Cargo.toml"}, {Name: prefix + "src/lib.rs"}}, []string{manifest, ""}, nil)
	}
	var lock struct {
		Version  int                  `toml:"version"`
		Packages []cargoLockedPackage `toml:"package"`
	}
	if err := toml.Unmarshal([]byte(l), &lock); err != nil {
		t.Fatal(err)
	}
	for i := range lock.Packages {
		p := &lock.Packages[i]
		if b := blobs[p.Name]; b != nil {
			sum := sha256.Sum256(b)
			p.Checksum = hex.EncodeToString(sum[:])
		}
	}
	lb, err := toml.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	dir := writeCargoDocuments(t, m, string(lb))
	c := cargoClient(t, dir, "normal")
	transport := c.HTTP
	c.HTTP = doFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := transport.Do(r)
		if err != nil || r.URL.Host != "crates.io" || strings.HasSuffix(r.URL.Path, "/dependencies") {
			return resp, err
		}
		defer resp.Body.Close()
		var data map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			t.Fatal(err)
		}
		v := data["version"].(map[string]any)
		sum := sha256.Sum256(blobs[v["crate"].(string)])
		v["checksum"] = hex.EncodeToString(sum[:])
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})
	r := c.AssessLocal(context.Background(), dir, "cargo", "pkg", nil, DefaultPolicy(14), checked)
	if r.Decision != Allow {
		t.Fatal("fixture not approved", r)
	}
	return dir, r, blobs
}

func TestPreparedCrateArtifacts(t *testing.T) {
	dir, r, blobs := approvedCrateArtifacts(t)
	artifacts, err := PrepareCargoArtifacts(dir, r)
	if err != nil || len(artifacts) != 2 {
		t.Fatal(artifacts, err)
	}
	for _, a := range artifacts {
		if _, err := VerifyCargoArtifact(blobs[a.Node.Name], a); err != nil {
			t.Fatal(err)
		}
	}
	r.Details.Project.cargo.featurePackages[artifacts[0].Node.Path].features["changed"] = []string{}
	if _, err := VerifyCargoArtifact(blobs[artifacts[0].Node.Name], artifacts[0]); err != nil {
		t.Fatal("assessment mutation changed frozen archive facts", err)
	}
	if !r.Details.Project.unchanged(dir) {
		t.Fatal("verification mutated project")
	}
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname='changed'"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareCargoArtifacts(dir, r); err == nil {
		t.Fatal("changed project approved")
	}
}

func TestCratePreparationRequiresApproval(t *testing.T) {
	for _, change := range []func(*Report){
		func(r *Report) { r.Decision = Unknown }, func(r *Report) { r.Scope = "package" }, func(r *Report) { r.Recommendation = nil }, func(r *Report) { r.Evidence.Identity = "unverified" }, func(r *Report) { r.Evidence.Maintainers = "single" }, func(r *Report) { r.Evidence.Missing = []string{"gap"} }, func(r *Report) { r.Details = nil }, func(r *Report) { r.Package = "npm:pkg" }, func(r *Report) { r.Recommendation.Version = "2.0.0" },
		func(r *Report) { r.Details.Project.Features.Status = "unknown" }, func(r *Report) { r.Details.Project.cargo = nil }, func(r *Report) { r.Details.Project.Findings[0].Finding.Decision = Deny }, func(r *Report) { r.Details.Project.Findings[0].Finding.Evidence.Identity = "unverified" }, func(r *Report) { r.Details.Project.Findings[0].Finding.Evidence = nil }, func(r *Report) { r.Details.Project.Findings[0].Finding.Version = "2.0.0" },
		func(r *Report) { r.Details.Project.Findings[0].Path = "wrong" }, func(r *Report) {
			r.Details.Project.Findings = append(r.Details.Project.Findings, r.Details.Project.Findings[0])
		},
		func(r *Report) { delete(r.Details.Project.cargo.featurePackages, r.Details.Project.Nodes[0].Path) }, func(r *Report) {
			r.Details.Project.cargo.featurePackages[r.Details.Project.Nodes[0].Path].features = nil
		},
	} {
		dir, r, _ := approvedCrateArtifacts(t)
		change(&r)
		if _, err := PrepareCargoArtifacts(dir, r); !errors.Is(err, ErrInstallUnsupported) {
			t.Fatal("unapproved artifact plan accepted", err)
		}
	}
}

func TestCrateArchiveLimits(t *testing.T) {
	headers := make([]tar.Header, 4097)
	bodies := make([]string, 4097)
	for i := range headers {
		headers[i] = tar.Header{Name: "pkg-1.0.0/f" + strconv.Itoa(i)}
	}
	b, a := crateFixture(t, simpleCrateManifest, headers, bodies, nil)
	if _, err := VerifyCargoArtifact(b, a); err == nil {
		t.Fatal("entry count exceeded")
	}
	b, a = crateFixture(t, strings.Repeat("x", (1<<20)+1), nil, nil, nil)
	if _, err := VerifyCargoArtifact(b, a); err == nil {
		t.Fatal("manifest bound exceeded")
	}
	b, a = crateFixture(t, simpleCrateManifest, []tar.Header{{Name: "pkg-1.0.0/large"}}, []string{strings.Repeat("x", maxUnpackedBytes+1)}, nil)
	if _, err := VerifyCargoArtifact(b, a); err == nil {
		t.Fatal("expansion bound exceeded")
	}
	if cargoPortablePath("pkg-1.0.0/" + strings.Repeat("a/", 65) + "file") {
		t.Fatal("path depth unbounded")
	}
	headers = make([]tar.Header, 2200)
	bodies = make([]string, 2200)
	for i := range headers {
		headers[i] = tar.Header{Name: "pkg-1.0.0/" + strings.Repeat("A", 3990) + strconv.Itoa(i)}
	}
	b, a = crateFixture(t, simpleCrateManifest, headers, bodies, nil)
	if _, err := VerifyCargoArtifact(b, a); err == nil {
		t.Fatal("path metadata bound exceeded")
	}
}

func FuzzCrateEnvelope(f *testing.F) {
	f.Add([]byte("invalid gzip"))
	b, _ := crateFixture(f, simpleCrateManifest, nil, nil, nil)
	f.Add(b)
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 8192 {
			t.Skip()
		}
		a := sealedCrate(ProjectNode{Name: "pkg", Version: "1.0.0"}, &cargoFeaturePackage{complete: true, features: map[string][]string{}}, b)
		_, _ = VerifyCargoArtifact(b, a)
	})
}
