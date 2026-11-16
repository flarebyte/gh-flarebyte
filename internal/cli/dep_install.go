// purpose: Apply a fully approved npm plan through isolated staging and guarded local commits.
// responsibilities: Verify artifacts, preserve assessment inputs and report partial recovery without overwriting concurrent edits.
// architecture notes: Policy rejection precedes all mutation; native hooks remain disabled and live resolution is forbidden.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

var dependencyArtifact = func(ctx context.Context, a dependencies.InstallArtifact) ([]byte, error) {
	return dependencies.NewClient().FetchArtifact(ctx, a)
}
var installDependency = installNPMDependency

func installNPMDependency(ctx context.Context, root string, report *dependencies.Report) error {
	plan, err := dependencies.PrepareInstall(root, *report)
	if err != nil {
		return err
	}
	if report.CheckedAt.IsZero() || report.CheckedAt.After(time.Now()) || time.Since(report.CheckedAt) > 24*time.Hour {
		return fmt.Errorf("assessment is stale")
	}
	policy, _, err := dependencyPolicy(root)
	if err != nil || policy.Digest() != report.PolicyDigest {
		return fmt.Errorf("dependency policy changed after assessment")
	}
	lockPath := filepath.Join(root, ".gh-flarebyte-dep.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("cannot acquire dependency installation lock")
	}
	lock.Close()
	defer os.Remove(lockPath)
	stage, err := os.MkdirTemp(root, ".gh-flarebyte-dep-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stage)
		}
	}()
	for name, data := range map[string][]byte{"package.json": plan.Manifest, "package-lock.json": plan.Lockfile} {
		if err := os.WriteFile(filepath.Join(stage, name), data, 0600); err != nil {
			return err
		}
	}
	runner, err := dependencyNPMRunner(root, stage)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	total := 0
	var expanded int64
	for index, a := range plan.Artifacts {
		b, err := dependencyArtifact(ctx, a)
		if err != nil {
			return err
		}
		// Recheck the boundary even when an injected transport supplied the bytes.
		if err := dependencies.VerifyArtifact(b, a); err != nil {
			return err
		}
		total += len(b)
		if total > 100<<20 {
			return fmt.Errorf("installation artifact budget exceeded")
		}
		size, err := dependencies.ArtifactExpandedSize(b)
		if err != nil {
			return err
		}
		expanded += size
		if expanded > 300<<20 {
			return fmt.Errorf("installation expansion budget exceeded")
		}
		path := filepath.Join(stage, fmt.Sprintf("artifact-%d.tgz", index))
		if err := os.WriteFile(path, b, 0600); err != nil {
			return err
		}
		if err := runner(ctx, []string{"cache", "add", path}); err != nil {
			return err
		}
	}
	if err := runner(ctx, []string{"ci"}); err != nil {
		return err
	}
	staged := dependencies.ReadProject(stage, "npm")
	if !staged.Complete || !sameFileBytes(filepath.Join(stage, "package.json"), plan.Manifest) || !sameFileBytes(filepath.Join(stage, "package-lock.json"), plan.Lockfile) {
		return fmt.Errorf("native tooling changed the approved resolution")
	}
	if err := validateInstalledTree(filepath.Join(stage, "node_modules")); err != nil {
		return err
	}
	if err := plan.VerifyInstalled(stage); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !plan.Unchanged(root) {
		return fmt.Errorf("project changed during installation")
	}
	if policyNow, _, err := dependencyPolicy(root); err != nil || policyNow.Digest() != report.PolicyDigest {
		return fmt.Errorf("dependency policy changed during installation")
	}
	if retained, err := commitDependencyStage(root, stage, plan); err != nil {
		cleanup = !retained
		return err
	}
	encoded, _ := json.Marshal(report)
	report.Installation = &dependencies.Installation{Version: report.Recommendation.Version, Files: []string{"package.json", "package-lock.json", "node_modules"}, AssessmentReference: dependencyDigest(encoded)}
	return nil
}

func validateInstalledTree(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("native installation tree missing or unsafe")
	}
	count := 0
	var size int64
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		if count > 20000 {
			return fmt.Errorf("installed file count exceeds limit")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("native installation contains unsupported links or special files")
		}
		size += info.Size()
		if size > 300<<20 {
			return fmt.Errorf("installed tree exceeds size limit")
		}
		return nil
	})
}
