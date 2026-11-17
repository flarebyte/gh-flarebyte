package dependencies

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func fixtureArtifact(t *testing.T) ([]byte, InstallArtifact) {
	t.Helper()
	b, err := os.ReadFile("testdata/fixture-lib.tgz")
	if err != nil {
		t.Fatal(err)
	}
	h := sha512.Sum512(b)
	return b, InstallArtifact{Node: ProjectNode{Name: "fixture-lib", Version: "1.0.0", Path: "node_modules/fixture-lib", Resolved: "https://registry.npmjs.org/fixture-lib/-/fixture-lib-1.0.0.tgz", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(h[:])}}
}

func archiveFixture(t testing.TB, entries []tar.Header, bodies []string, extra []byte) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for i, h := range entries {
		if h.Typeflag == 0 {
			h.Typeflag = tar.TypeReg
		}
		if h.Mode == 0 {
			h.Mode = 0644
		}
		if i < len(bodies) {
			h.Size = int64(len(bodies[i]))
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if i < len(bodies) {
			if _, err := io.WriteString(tw, bodies[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = tw.Close()
	raw.Write(extra)
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func withArtifactHash(a InstallArtifact, b []byte) InstallArtifact {
	h := sha512.Sum512(b)
	a.Node.Integrity = "sha512-" + base64.StdEncoding.EncodeToString(h[:])
	return a
}

func TestArtifactPreflight(t *testing.T) {
	b, a := fixtureArtifact(t)
	if size, err := ArtifactExpandedSize(b); err != nil || size <= 0 {
		t.Fatal(size, err)
	}
	if _, err := ArtifactExpandedSize([]byte("invalid")); err == nil {
		t.Fatal("invalid expansion accepted")
	}
	if err := VerifyArtifact(b, a); err != nil {
		t.Fatal(err)
	}
	if err := VerifyArtifact([]byte("wrong"), a); err == nil {
		t.Fatal("integrity mismatch accepted")
	}
	if err := VerifyArtifact([]byte("bad gzip"), withArtifactHash(a, []byte("bad gzip"))); err == nil {
		t.Fatal("invalid compression accepted")
	}
	for _, tt := range []struct {
		name    string
		headers []tar.Header
		bodies  []string
		extra   []byte
	}{
		{"escape", []tar.Header{{Name: "package/../outside"}}, []string{"data"}, nil},
		{"absolute", []tar.Header{{Name: "/outside"}}, []string{"data"}, nil},
		{"backslash", []tar.Header{{Name: "package/a\\b"}}, []string{"data"}, nil},
		{"link", []tar.Header{{Name: "package/link", Typeflag: tar.TypeSymlink, Linkname: "/outside"}}, nil, nil},
		{"setuid", []tar.Header{{Name: "package/a", Mode: 04755}}, []string{"data"}, nil},
		{"duplicate", []tar.Header{{Name: "package/a"}, {Name: "package/a"}}, []string{"a", "b"}, nil},
		{"bundle", []tar.Header{{Name: "package/node_modules/hidden"}}, []string{"data"}, nil},
		{"shrinkwrap", []tar.Header{{Name: "package/npm-shrinkwrap.json"}}, []string{`{}`}, nil},
		{"missing manifest", []tar.Header{{Name: "package/index.js"}}, []string{"data"}, nil},
		{"wrong name", []tar.Header{{Name: "package/package.json"}}, []string{`{"name":"other","version":"1.0.0"}`}, nil},
		{"wrong deps", []tar.Header{{Name: "package/package.json"}}, []string{`{"name":"fixture-lib","version":"1.0.0","dependencies":{"hidden":"1.0.0"}}`}, nil},
		{"bin", []tar.Header{{Name: "package/package.json"}}, []string{`{"name":"fixture-lib","version":"1.0.0","bin":"run.js"}`}, nil},
		{"trailing payload", []tar.Header{{Name: "package/package.json"}}, []string{`{"name":"fixture-lib","version":"1.0.0"}`}, []byte("hidden")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := archiveFixture(t, tt.headers, tt.bodies, tt.extra)
			if VerifyArtifact(b, withArtifactHash(a, b)) == nil {
				t.Fatal("unsafe artifact accepted")
			}
		})
	}
	large := make([]byte, MaxArtifactBytes+1)
	if VerifyArtifact(large, withArtifactHash(a, large)) == nil {
		t.Fatal("oversize accepted")
	}
	valid := archiveFixture(t, []tar.Header{{Name: "package", Typeflag: tar.TypeDir}, {Name: "package/package.json"}}, []string{"", `{"name":"fixture-lib","version":"1.0.0"}`}, nil)
	if err := VerifyArtifact(valid, withArtifactHash(a, valid)); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactDownload(t *testing.T) {
	b, a := fixtureArtifact(t)
	c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "registry.npmjs.org" || r.Header.Get("Authorization") != "" {
			t.Fatal("unsafe artifact request")
		}
		return response(string(b), 200), nil
	})}
	got, err := c.FetchArtifact(context.Background(), a)
	if err != nil || !bytes.Equal(got, b) {
		t.Fatal(err)
	}
	bad := a
	bad.Node.Resolved = "https://evil.test/file"
	if _, err := c.FetchArtifact(context.Background(), bad); err == nil {
		t.Fatal("foreign download")
	}
	c.HTTP = doFunc(func(*http.Request) (*http.Response, error) { return response("missing", 404), nil })
	if _, err := c.FetchArtifact(context.Background(), a); err == nil {
		t.Fatal("missing artifact")
	}
	c.HTTP = doFunc(func(*http.Request) (*http.Response, error) {
		return response(strings.Repeat("x", MaxArtifactBytes+1), 200), nil
	})
	if _, err := c.FetchArtifact(context.Background(), a); err == nil {
		t.Fatal("oversize download")
	}
}
