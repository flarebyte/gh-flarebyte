// purpose: Verify bounded crates.io archives before any extraction or native tooling.
// responsibilities: Enforce SHA-256, portable archive paths and corroborated manifest identities.
// architecture notes: All entries are treated as data; no filesystem writes or package code execution occur.
package dependencies

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"strings"
)

type CargoArchiveInfo struct {
	SHA256        string `json:"sha256"`
	Files         int    `json:"files"`
	ExpandedBytes int64  `json:"expanded_bytes"`
	BuildScript   bool   `json:"build_script"`
	ProcMacro     bool   `json:"proc_macro"`
}

func (c *Client) FetchCargoArtifact(ctx context.Context, a CargoArtifact) ([]byte, CargoArchiveInfo, error) {
	if !a.valid() {
		return nil, CargoArchiveInfo{}, errors.New("crate artifact origin or assessment binding invalid")
	}
	b, err := c.fetchArtifactBytes(ctx, a.Node.Resolved)
	if err != nil {
		return nil, CargoArchiveInfo{}, err
	}
	info, err := VerifyCargoArtifact(b, a)
	if err != nil {
		return nil, CargoArchiveInfo{}, err
	}
	return b, info, nil
}

func VerifyCargoArtifact(b []byte, a CargoArtifact) (CargoArchiveInfo, error) {
	reject := func(message string) (CargoArchiveInfo, error) { return CargoArchiveInfo{}, errors.New(message) }
	if !a.valid() || len(b) > MaxArtifactBytes {
		return reject("crate artifact binding or compressed size invalid")
	}
	hash := sha256.Sum256(b)
	digest := hex.EncodeToString(hash[:])
	if a.Node.Integrity != "sha256-"+digest {
		return reject("crate artifact integrity mismatch")
	}
	raw := bytes.NewReader(b)
	gz, err := gzip.NewReader(raw)
	if err != nil {
		return reject("invalid compressed crate")
	}
	defer gz.Close()
	gz.Multistream(false)
	limited := &io.LimitedReader{R: gz, N: maxUnpackedBytes + 1}
	tr := tar.NewReader(limited)
	prefix := a.Node.Name + "-" + a.Node.Version
	seen := map[string]byte{}
	directories := map[string]bool{}
	spellings := map[string]string{}
	files := map[string]bool{}
	var manifest []byte
	var fileBytes int64
	pathBytes := 0
	info := CargoArchiveInfo{SHA256: digest}
	for count := 0; ; count++ {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return reject("invalid crate archive")
		}
		if count >= 4096 || limited.N <= 0 {
			return reject("crate archive expansion limit exceeded")
		}
		if (h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir) || h.Linkname != "" {
			return reject("crate links or special entries unsupported")
		}
		name := strings.TrimSuffix(h.Name, "/")
		if !cargoPortablePath(name) || (name != prefix && !strings.HasPrefix(name, prefix+"/")) || h.Typeflag == tar.TypeReg && strings.HasSuffix(h.Name, "/") || name == prefix && h.Typeflag != tar.TypeDir {
			return reject("unsafe or unsupported crate archive path")
		}
		if h.Size < 0 || h.Size > maxUnpackedBytes || h.Mode < 0 || h.Mode & ^int64(0777) != 0 || h.Typeflag == tar.TypeDir && h.Size != 0 {
			return reject("unsafe crate archive entry")
		}
		for key := range h.PAXRecords {
			switch key {
			case "path", "size", "mtime", "atime", "ctime", "uid", "gid", "uname", "gname":
			default:
				return reject("unsupported crate archive attributes")
			}
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists || h.Typeflag == tar.TypeReg && directories[key] {
			return reject("crate archive path collision")
		}
		for component := name; component != "."; component = path.Dir(component) {
			folded := strings.ToLower(component)
			if original, exists := spellings[folded]; exists && original != component {
				return reject("crate archive case collision")
			}
			if _, exists := spellings[folded]; !exists {
				pathBytes += len(component)
				if pathBytes > 8<<20 {
					return reject("crate path metadata limit exceeded")
				}
			}
			spellings[folded] = component
			if component != name {
				if seen[folded] == tar.TypeReg {
					return reject("crate archive file/directory collision")
				}
				directories[folded] = true
			}
		}
		seen[key] = h.Typeflag
		relative := strings.TrimPrefix(name, prefix+"/")
		for _, part := range strings.Split(relative, "/") {
			folded := strings.ToLower(part)
			if folded == ".cargo" || folded == ".cargo-checksum.json" || folded == ".git" || folded == ".hg" || folded == ".svn" {
				return reject("embedded crate control files unsupported")
			}
		}
		for _, canonical := range []string{"Cargo.toml", "Cargo.toml.orig", "Cargo.lock", "build.rs", "src/lib.rs", "src/main.rs"} {
			if strings.EqualFold(relative, canonical) && relative != canonical {
				return reject("noncanonical crate control path")
			}
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		fileBytes += h.Size
		if fileBytes > maxUnpackedBytes {
			return reject("crate logical expansion limit exceeded")
		}
		files[relative] = true
		info.Files++
		if relative == "Cargo.toml" {
			if h.Size > 1<<20 {
				return reject("crate manifest size invalid")
			}
			manifest, err = io.ReadAll(tr)
			if err != nil {
				return reject("crate manifest read failed")
			}
		} else if _, err := io.CopyN(io.Discard, tr, h.Size); err != nil {
			return reject("truncated crate entry")
		}
	}
	tail, err := io.ReadAll(limited)
	if err != nil || limited.N <= 0 || raw.Len() != 0 {
		return reject("invalid or oversized crate trailer")
	}
	for _, value := range tail {
		if value != 0 {
			return reject("unexpected crate trailer payload")
		}
	}
	if manifest == nil {
		return reject("crate manifest missing")
	}
	traits, err := verifyCargoManifest(manifest, a, files)
	if err != nil {
		return CargoArchiveInfo{}, err
	}
	info.BuildScript, info.ProcMacro = traits.BuildScript, traits.ProcMacro
	info.ExpandedBytes = maxUnpackedBytes + 1 - limited.N
	if fileBytes > info.ExpandedBytes {
		info.ExpandedBytes = fileBytes
	}
	return info, nil
}

// A deliberately portable subset avoids platform-dependent aliases before a
// future extraction step. Unsupported filenames are rejected, not normalised.
func cargoPortablePath(name string) bool {
	if name == "" || name == "." || len(name) > 4096 || name != path.Clean(name) || strings.HasPrefix(name, "/") {
		return false
	}
	for _, r := range name {
		if r < 32 || r > 126 || strings.ContainsRune(`\:*?<>"|`, r) {
			return false
		}
	}
	parts := strings.Split(name, "/")
	if len(parts) > 64 {
		return false
	}
	for _, part := range parts {
		if part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		stem, _, _ := strings.Cut(strings.ToLower(part), ".")
		if stem == "con" || stem == "prn" || stem == "aux" || stem == "nul" || len(stem) == 4 && (strings.HasPrefix(stem, "com") || strings.HasPrefix(stem, "lpt")) && stem[3] >= '1' && stem[3] <= '9' {
			return false
		}
	}
	return true
}
