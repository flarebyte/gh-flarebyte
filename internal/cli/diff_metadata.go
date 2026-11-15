// purpose: Enrich changed paths with existing Thoth documents without executing metadata tools.
package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func parseDiffMetadata(data []byte) (map[string]any, string, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var node yaml.Node
	if err := dec.Decode(&node); err != nil {
		return nil, "", err
	}
	// Reject aliases and custom tags: metadata is plain data, not an executable
	// format, and aliases can cause excessive expansion while normalizing to JSON.
	var validate func(*yaml.Node, int) error
	validate = func(n *yaml.Node, depth int) error {
		if depth > 100 {
			return fmt.Errorf("metadata nesting exceeds 100 levels")
		}
		if n.Kind == yaml.AliasNode || (n.Tag != "" && !strings.HasPrefix(n.Tag, "!!")) {
			return fmt.Errorf("aliases and custom YAML tags are unsupported")
		}
		if n.Kind == yaml.MappingNode {
			for i := 0; i < len(n.Content); i += 2 {
				if n.Content[i].Tag != "!!str" {
					return fmt.Errorf("metadata mapping keys must be strings")
				}
			}
		}
		for _, child := range n.Content {
			if err := validate(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := validate(&node, 0); err != nil {
		return nil, "", err
	}
	var doc map[string]any
	if err := node.Decode(&doc); err != nil {
		return nil, "", err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, "", fmt.Errorf("expected exactly one YAML document")
	}
	locator, ok := doc["locator"].(string)
	if !ok || locator == "" || path.IsAbs(locator) || strings.Contains(locator, "\\") || path.Clean(locator) == ".." || strings.HasPrefix(path.Clean(locator), "../") {
		return nil, "", fmt.Errorf("locator must be a repository-relative source path")
	}
	// Normalize YAML values to a JSON-compatible model once for both renderers.
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, "", err
	}
	if err = json.Unmarshal(encoded, &doc); err != nil {
		return nil, "", err
	}
	return doc, path.Clean(locator), nil
}

func (g diffGit) enrichMetadata(report *diffReport) error {
	working := map[string][]diffMetadata{}
	baseline := map[string][]diffMetadata{}
	add := func(data []byte, source, provenance string, index map[string][]diffMetadata) {
		doc, locator, err := parseDiffMetadata(data)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("metadata %q (%s): %s", source, provenance, err))
			return
		}
		index[locator] = append(index[locator], diffMetadata{Source: source, Provenance: provenance, Document: doc})
	}
	root := filepath.Join(g.root, "thoth-meta")
	info, err := os.Lstat(root)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && info.IsDir() {
		err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				report.Warnings = append(report.Warnings, fmt.Sprintf("metadata %q: %s", name, walkErr))
				return nil
			}
			if entry.IsDir() || !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".thoth.yaml") {
				return nil
			}
			relative, e := filepath.Rel(g.root, name)
			if e != nil {
				return e
			}
			data, e := os.ReadFile(name)
			if e != nil {
				report.Warnings = append(report.Warnings, fmt.Sprintf("metadata %q: %s", relative, e))
				return nil
			}
			add(data, filepath.ToSlash(relative), "working-tree", working)
			return nil
		})
		if err != nil {
			return err
		}
	}
	// Only consult baseline documents if at least one changed source needs them.
	needsBaseline := false
	for _, file := range report.Files {
		p := file.Path
		if file.PathEncoding == "base64" {
			b, _ := base64.StdEncoding.DecodeString(p)
			p = string(b)
		}
		if !strings.HasPrefix(p, "thoth-meta/") && len(working[p]) == 0 {
			needsBaseline = true
			break
		}
	}
	if needsBaseline {
		listing, e := g.run("ls-tree", "-r", "-z", report.Comparison.BaselineCommit, "--", "thoth-meta/")
		if e != nil {
			return e
		}
		for _, record := range bytes.Split(listing, []byte{0}) {
			header, name, ok := bytes.Cut(record, []byte{'\t'})
			if !ok || !bytes.HasSuffix(name, []byte(".thoth.yaml")) {
				continue
			}
			fields := strings.Fields(string(header))
			if len(fields) != 3 || fields[0] != "100644" && fields[0] != "100755" {
				continue
			}
			data, e := g.run("cat-file", "blob", fields[2])
			if e != nil {
				return e
			}
			add(data, string(name), "baseline", baseline)
		}
	}
	for i := range report.Details {
		detail := &report.Details[i]
		p := detail.Path
		if detail.PathEncoding == "base64" {
			b, _ := base64.StdEncoding.DecodeString(p)
			p = string(b)
		}
		if strings.HasPrefix(p, "thoth-meta/") {
			continue
		}
		matches := working[p]
		if len(matches) == 0 {
			matches = baseline[p]
		}
		if len(matches) > 0 {
			sort.Slice(matches, func(i, j int) bool { return matches[i].Source < matches[j].Source })
			detail.Metadata = matches
		}
	}
	sort.Strings(report.Warnings)
	return nil
}
