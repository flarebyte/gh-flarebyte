// purpose: Verify bounded npm artifacts before handing them to native package tooling.
// responsibilities: Enforce SHA-512 integrity and reject archive escapes, links, bundles and metadata disagreement.
// architecture notes: No archive is extracted by this code and no dependency code executes.
package dependencies

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"
)

const MaxArtifactBytes = 20 << 20
const maxUnpackedBytes = 50 << 20

func ArtifactExpandedSize(b []byte) (int64, error) {
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return 0, fmt.Errorf("invalid compressed artifact")
	}
	defer gz.Close()
	n, err := io.CopyN(io.Discard, gz, maxUnpackedBytes+1)
	if (err != nil && err != io.EOF) || n > maxUnpackedBytes {
		return 0, fmt.Errorf("artifact expansion limit exceeded")
	}
	return n, nil
}

func (c *Client) FetchArtifact(ctx context.Context, a InstallArtifact) ([]byte, error) {
	if !npmArtifact(a.Node.Resolved, a.Node.Integrity) {
		return nil, fmt.Errorf("artifact origin or integrity unsupported")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Node.Resolved, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gh-flarebyte-dep/1")
	resp, _, err := c.request(req)
	if err != nil {
		return nil, fmt.Errorf("artifact unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("artifact download failed (HTTP %d)", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxArtifactBytes+1))
	if err != nil {
		return nil, fmt.Errorf("artifact read failed")
	}
	if len(b) > MaxArtifactBytes {
		return nil, fmt.Errorf("artifact exceeds compressed size limit")
	}
	if err := VerifyArtifact(b, a); err != nil {
		return nil, err
	}
	return b, nil
}

func VerifyArtifact(b []byte, a InstallArtifact) error {
	if len(b) > MaxArtifactBytes {
		return fmt.Errorf("artifact exceeds compressed size limit")
	}
	hash := sha512.Sum512(b)
	if a.Node.Integrity != "sha512-"+base64.StdEncoding.EncodeToString(hash[:]) {
		return fmt.Errorf("artifact integrity mismatch")
	}
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("invalid compressed artifact")
	}
	defer gz.Close()
	limited := &io.LimitedReader{R: gz, N: maxUnpackedBytes + 1}
	reader := tar.NewReader(limited)
	seen := map[string]bool{}
	manifestFound := false
	for count := 0; ; count++ {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid artifact archive")
		}
		if count >= 4096 || limited.N <= 0 {
			return fmt.Errorf("artifact expansion limit exceeded")
		}
		name := strings.TrimSuffix(h.Name, "/")
		if !utf8.ValidString(name) || strings.ContainsAny(name, "\\\x00") || name != path.Clean(name) || (name != "package" && !strings.HasPrefix(name, "package/")) {
			return fmt.Errorf("unsafe artifact path")
		}
		if seen[name] || h.Size < 0 || h.Size > maxUnpackedBytes || h.Mode&06000 != 0 {
			return fmt.Errorf("unsafe artifact entry")
		}
		seen[name] = true
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
			return fmt.Errorf("artifact links or special files are unsupported")
		}
		if name == "package/npm-shrinkwrap.json" || strings.HasPrefix(name, "package/node_modules/") || name == "package/node_modules" {
			return fmt.Errorf("embedded resolution is unsupported")
		}
		if name == "package/package.json" {
			if h.Typeflag != tar.TypeReg || h.Size > 1<<20 {
				return fmt.Errorf("invalid artifact manifest")
			}
			data, err := io.ReadAll(reader)
			if err != nil {
				return fmt.Errorf("artifact manifest read failed")
			}
			var manifest npmPackage
			if uniqueJSON(data, &manifest) != nil || manifest.Name != a.Node.Name || manifest.Version != a.Node.Version {
				return fmt.Errorf("artifact package identity mismatch")
			}
			if raw := bytes.TrimSpace(manifest.Bin); len(raw) > 0 && !bytes.Equal(raw, []byte("null")) && !bytes.Equal(raw, []byte("{}")) {
				return fmt.Errorf("artifact executable bin entries are unsupported")
			}
			manifest.DevDependencies = nil
			expected := a.Manifest
			expected.DevDependencies = nil
			if !sameDeps(manifest, expected) {
				return fmt.Errorf("artifact dependency metadata mismatch")
			}
			manifestFound = true
		}
	}
	// Validate the gzip trailer as well as any trailing tar padding. A second
	// hidden archive or nonzero payload after the tar terminator is refused.
	tail, err := io.ReadAll(limited)
	if err != nil || limited.N <= 0 {
		return fmt.Errorf("invalid or oversized artifact trailer")
	}
	for _, v := range tail {
		if v != 0 {
			return fmt.Errorf("unexpected artifact trailer")
		}
	}
	if !manifestFound {
		return fmt.Errorf("artifact manifest missing")
	}
	return nil
}
