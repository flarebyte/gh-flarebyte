// purpose: Stage fresh build artifacts into the configured release directory.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func stagePythonRelease(built []string, dir string, includeChecksums bool) ([]string, error) {
	return stageFreshRelease(built, "", dir, includeChecksums)
}

func stageFreshRelease(built []string, outputDir, dir string, includeChecksums bool) ([]string, error) {
	if err := ensureUniqueArtifactBasenames(built); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var artifacts []string
	for i, src := range built {
		// Artifact-producing builders always return their checksum manifest last.
		if i == len(built)-1 && !includeChecksums {
			continue
		}
		name := filepath.Base(src)
		if outputDir != "" && i != len(built)-1 {
			rel, err := filepath.Rel(outputDir, src)
			if err != nil {
				return nil, err
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("build artifact lies outside build.outputDir: %s", src)
			}
			name = rel
		}
		dst := filepath.Join(dir, name)
		srcAbs, err := filepath.Abs(src)
		if err != nil {
			return nil, err
		}
		dstAbs, err := filepath.Abs(dst)
		if err != nil {
			return nil, err
		}
		if srcAbs != dstAbs {
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return nil, err
			}
			info, err := os.Stat(src)
			if err != nil {
				return nil, err
			}
			if err := copyFile(src, dst, info.Mode().Perm()); err != nil {
				return nil, err
			}
		}
		artifacts = append(artifacts, dst)
	}
	return artifacts, nil
}
