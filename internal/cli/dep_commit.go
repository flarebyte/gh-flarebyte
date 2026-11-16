// purpose: Commit staged dependency files with recoverable local filesystem changes.
// responsibilities: Detect changed inputs, avoid overwriting edits during rollback and preserve recovery files on conflict.
// architecture notes: The installer holds an exclusive command lock; other package managers must not run concurrently.
package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

func dependencyDigest(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}
func sameFileBytes(path string, expected []byte) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(expected)) {
		return false
	}
	b, err := os.ReadFile(path)
	return err == nil && bytes.Equal(b, expected)
}

func dependencyTreeDigest(dir string) (string, error) {
	if err := validateInstalledTree(dir); err != nil {
		return "", err
	}
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%q %v %d\n", relative, info.Mode(), info.Size())
		if entry.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, readErr := io.Copy(h, io.LimitReader(f, 300<<20))
		closeErr := f.Close()
		if readErr != nil {
			return readErr
		}
		return closeErr
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type dependencyCommitItem struct {
	name   string
	data   []byte
	tree   bool
	old    bool
	placed bool
	backup string
}

var dependencyRename = os.Rename

func commitDependencyStage(root, stage string, plan dependencies.InstallPlan) (bool, error) {
	if !plan.Unchanged(root) {
		return false, fmt.Errorf("project changed before commit")
	}
	items := []dependencyCommitItem{{name: "package.json", data: plan.Manifest}, {name: "package-lock.json", data: plan.Lockfile}, {name: "node_modules", tree: true}}
	for i := range items {
		item := &items[i]
		item.backup = filepath.Join(stage, "backup-"+item.name)
		info, err := os.Lstat(filepath.Join(root, item.name))
		if os.IsNotExist(err) && item.tree {
			continue
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || item.tree && !info.IsDir() || !item.tree && !info.Mode().IsRegular() {
			return false, fmt.Errorf("unsafe dependency destination")
		}
		if !item.tree {
			if err := os.Chmod(filepath.Join(stage, item.name), info.Mode().Perm()); err != nil {
				return false, err
			}
		}
	}
	treeHash, err := dependencyTreeDigest(filepath.Join(stage, "node_modules"))
	if err != nil {
		return false, err
	}
	for i := range items {
		item := &items[i]
		destination := filepath.Join(root, item.name)
		if _, err := os.Lstat(destination); err == nil {
			if err := dependencyRename(destination, item.backup); err != nil {
				return dependencyCommitFailed(root, stage, items, treeHash, err)
			}
			item.old = true
		} else if !os.IsNotExist(err) {
			return dependencyCommitFailed(root, stage, items, treeHash, err)
		}
		if item.tree {
			err = dependencyRename(filepath.Join(stage, item.name), destination)
		} else {
			err = os.Link(filepath.Join(stage, item.name), destination)
		}
		if err != nil {
			return dependencyCommitFailed(root, stage, items, treeHash, err)
		}
		item.placed = true
	}
	for _, item := range items {
		if !item.tree && !sameFileBytes(filepath.Join(root, item.name), item.data) {
			return dependencyCommitFailed(root, stage, items, treeHash, fmt.Errorf("project edited during commit"))
		}
	}
	if hash, err := dependencyTreeDigest(filepath.Join(root, "node_modules")); err != nil || hash != treeHash {
		return dependencyCommitFailed(root, stage, items, treeHash, fmt.Errorf("installed tree changed during commit"))
	}
	return false, nil
}

func dependencyCommitFailed(root, stage string, items []dependencyCommitItem, treeHash string, cause error) (bool, error) {
	retained := false
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		destination := filepath.Join(root, item.name)
		if item.placed {
			if item.tree {
				hash, err := dependencyTreeDigest(destination)
				if err != nil || hash != treeHash {
					retained = true
					continue
				}
				moved := filepath.Join(stage, "failed-node_modules")
				if err := os.Rename(destination, moved); err != nil {
					retained = true
					continue
				}
				if hash, err := dependencyTreeDigest(moved); err != nil || hash != treeHash {
					retained = true
				}
			} else {
				if !sameFileBytes(destination, item.data) {
					retained = true
					continue
				}
				if err := os.Remove(destination); err != nil {
					retained = true
					continue
				}
				if !sameFileBytes(filepath.Join(stage, item.name), item.data) {
					retained = true
				}
			}
		}
		if item.old {
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				retained = true
				continue
			}
			var err error
			if item.tree {
				err = os.Rename(item.backup, destination)
			} else {
				err = os.Link(item.backup, destination)
			}
			if err != nil {
				retained = true
			}
		}
	}
	if retained {
		return true, fmt.Errorf("dependency commit failed; recovery files retained in %s", filepath.Base(stage))
	}
	return false, fmt.Errorf("dependency commit failed and was rolled back: %w", cause)
}
