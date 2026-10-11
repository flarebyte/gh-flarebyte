// purpose: Stage only fresh Python build artifacts into the configured release directory.
package cli

import (
	"os"
	"path/filepath"
)

func stagePythonRelease(built []string, dir string, includeChecksums bool) ([]string, error) {
	if err := ensureUniqueArtifactBasenames(built); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var artifacts []string
	for i, src := range built {
		// buildPythonPackage always returns its checksum manifest last.
		if i == len(built)-1 && !includeChecksums {
			continue
		}
		dst := filepath.Join(dir, filepath.Base(src))
		srcAbs, err := filepath.Abs(src)
		if err != nil {
			return nil, err
		}
		dstAbs, err := filepath.Abs(dst)
		if err != nil {
			return nil, err
		}
		if srcAbs != dstAbs {
			if err := copyFile(src, dst, 0o644); err != nil {
				return nil, err
			}
		}
		artifacts = append(artifacts, dst)
	}
	return artifacts, nil
}
