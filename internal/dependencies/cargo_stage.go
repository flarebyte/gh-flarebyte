// purpose: Stage an approved Cargo graph as verified directory sources without touching the repository.
// responsibilities: Extract preflighted archives, generate checksums and preserve immutable resolution inputs.
// architecture notes: The synthetic root validates dependency resolution only; it does not authorize a project commit.
package dependencies

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type CargoStage struct {
	dir        string
	original   *ProjectSnapshot
	artifacts  []CargoArtifact
	files      map[string]string
	written    map[string]bool
	features   map[string][]string
	compressed int
	expanded   int64
}

// PrepareCargoStage requires an empty private directory outside the repository.
// Callers own its cleanup, including on error. It never reads project source code.
func PrepareCargoStage(dir, stage string, report Report) (*CargoStage, error) {
	artifacts, err := PrepareCargoArtifacts(dir, report)
	if err != nil {
		return nil, err
	}
	stage, err = filepath.EvalSymlinks(stage)
	if err != nil {
		return nil, err
	}
	stage, err = filepath.Abs(stage)
	if err != nil {
		return nil, err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if rel, err := filepath.Rel(dir, stage); err != nil || filepath.IsLocal(rel) || rel == "." {
		return nil, fmt.Errorf("Cargo stage must be outside the repository")
	}
	info, err := os.Stat(stage)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("Cargo stage must be private")
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) != 0 {
		return nil, fmt.Errorf("Cargo stage must be empty")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	m, err := projectFile(root, "Cargo.toml", 1<<20)
	if err != nil {
		return nil, err
	}
	l, err := projectFile(root, "Cargo.lock", maxBody)
	if err != nil {
		return nil, err
	}
	approved := report.Details.Project
	if digestBytes(m) != approved.ManifestDigest || digestBytes(l) != approved.LockDigest {
		return nil, fmt.Errorf("Cargo project changed before staging")
	}
	var manifest map[string]any
	if err := toml.Unmarshal(m, &manifest); err != nil {
		return nil, err
	}
	pkg := manifest["package"].(map[string]any)
	// Copy resolution semantics, excluding arbitrary package metadata and source
	// paths. An explicit workspace prevents discovery of an ancestor workspace.
	synthetic := map[string]any{"package": map[string]any{}, "workspace": map[string]any{}}
	for _, key := range []string{"name", "version", "edition", "rust-version", "resolver"} {
		if v, exists := pkg[key]; exists {
			synthetic["package"].(map[string]any)[key] = v
		}
	}
	for _, key := range []string{"dependencies", "build-dependencies", "dev-dependencies", "features"} {
		if v, exists := manifest[key]; exists {
			synthetic[key] = v
		}
	}
	b, err := toml.Marshal(synthetic)
	if err != nil {
		return nil, err
	}
	// Freeze our own graph instead of retaining the caller's mutable report.
	original := ReadProject(dir, "cargo")
	if !original.Complete || original.ManifestDigest != approved.ManifestDigest || original.LockDigest != approved.LockDigest {
		return nil, fmt.Errorf("Cargo project changed before staging")
	}
	s := &CargoStage{dir: stage, original: original, artifacts: artifacts, files: map[string]string{}, written: map[string]bool{}, features: map[string][]string{}}
	for _, a := range artifacts {
		original.cargo.featurePackages[a.Node.Path] = a.expected.metadata
	}
	proof := resolveCargoFeatures(original)
	if proof.Status != "resolved" {
		return nil, fmt.Errorf("Cargo feature proof changed before staging")
	}
	for _, f := range proof.Enabled {
		s.features[f.Path] = append([]string{}, f.Features...)
	}
	config, err := toml.Marshal(map[string]any{"source": map[string]any{"crates-io": map[string]string{"replace-with": "verified"}, "verified": map[string]string{"directory": filepath.Join(stage, "vendor")}}, "net": map[string]bool{"offline": true}})
	if err != nil {
		return nil, err
	}
	for name, data := range map[string][]byte{"project/Cargo.toml": b, "project/Cargo.lock": l, "project/src/lib.rs": {}, "home/config.toml": config} {
		if err := s.write(name, data); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *CargoStage) Artifacts() []CargoArtifact { return append([]CargoArtifact{}, s.artifacts...) }

func (s *CargoStage) write(name string, b []byte) error {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	parent := ""
	for _, part := range strings.Split(filepath.Dir(name), string(os.PathSeparator)) {
		parent = filepath.Join(parent, part)
		if err := root.Mkdir(parent, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		info, err := root.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe Cargo stage directory")
		}
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	s.files[name] = digestBytes(b)
	return nil
}

// AddArtifact preflights the entire archive before writing any of its entries.
// A failed stage must be discarded; it must never be used as a shared cache.
func (s *CargoStage) AddArtifact(b []byte, a CargoArtifact) error {
	bound := false
	for _, expected := range s.artifacts {
		if a.Node == expected.Node && a.expected == expected.expected {
			bound = true
		}
	}
	if !bound || s.written[a.Node.Path] {
		return fmt.Errorf("unexpected or duplicate staged crate")
	}
	info, err := VerifyCargoArtifact(b, a)
	if err != nil {
		return err
	}
	if s.compressed+len(b) > 100<<20 || s.expanded+info.ExpandedBytes > 300<<20 {
		return fmt.Errorf("Cargo staging artifact budget exceeded")
	}
	// Reserve budgets before extraction, so a failed attempt cannot reset them.
	s.compressed += len(b)
	s.expanded += info.ExpandedBytes
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	checksums := map[string]string{}
	prefix := a.Node.Name + "-" + a.Node.Version + "/"
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(h.Name, prefix)
		if err := s.write(filepath.Join("vendor", strings.TrimSuffix(prefix, "/"), name), data); err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		checksums[name] = hex.EncodeToString(sum[:])
	}
	data, err := json.Marshal(map[string]any{"package": info.SHA256, "files": checksums})
	if err != nil {
		return err
	}
	if err := s.write(filepath.Join("vendor", strings.TrimSuffix(prefix, "/"), ".cargo-checksum.json"), data); err != nil {
		return err
	}
	s.written[a.Node.Path] = true
	return nil
}

func (s *CargoStage) Unchanged(dir string) bool { return s.original.unchanged(dir) }

// ValidateSDK compares actual stable SDK versions with the declared baseline.
// This does not prove that the application compiles on that toolchain.
func (s *CargoStage) ValidateSDK(cargo, rust string) error {
	c, cOK := rustVersion(cargo)
	r, rOK := rustVersion(rust)
	minimum, _ := rustVersion("1.85") // Supports lock v4 and edition 2024.
	baseline, bOK := rustVersion(s.original.RuntimeBaseline)
	if !cOK || !rOK || !bOK || compareNumbers(c, minimum) < 0 || compareNumbers(r, baseline) < 0 {
		return fmt.Errorf("%w: stable Cargo 1.85+ and compatible Rust required", ErrInstallUnsupported)
	}
	for _, a := range s.artifacts {
		m, ok := rustVersion(a.expected.metadata.rustVersion)
		floor, exists := rustEditionFloors[defaultCargoEdition(a.expected.metadata.edition)]
		f, _ := rustVersion(floor)
		if !ok || !exists || compareNumbers(r, m) < 0 || compareNumbers(r, f) < 0 {
			return fmt.Errorf("%w: incompatible crate compiler requirement", ErrInstallUnsupported)
		}
	}
	return nil
}
