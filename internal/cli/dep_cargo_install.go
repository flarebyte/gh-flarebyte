// purpose: Apply an approved existing Cargo dependency through isolated validation and a guarded exact pin.
// responsibilities: Preserve the lock graph, policy and concurrent edits without executing package code.
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

var dependencyCargoRunner = prepareDependencyCargo
var dependencyCargoArtifact = func(ctx context.Context, a dependencies.CargoArtifact) ([]byte, error) {
	b, _, err := dependencies.NewClient().FetchCargoArtifact(ctx, a)
	return b, err
}

func cargoInstallPolicy(root string, report *dependencies.Report) bool {
	p, _, err := dependencyPolicy(root)
	return err == nil && p.Digest() == report.PolicyDigest && !report.CheckedAt.IsZero() && !report.CheckedAt.After(time.Now()) && time.Since(report.CheckedAt) <= 24*time.Hour
}

func installCargoDependency(ctx context.Context, root string, report *dependencies.Report) error {
	plan, err := dependencies.PrepareCargoInstall(root, *report)
	if err != nil {
		return err
	}
	if !cargoInstallPolicy(root, report) {
		return fmt.Errorf("dependency assessment is stale or policy changed")
	}
	lockPath := filepath.Join(root, ".gh-flarebyte-dep.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("cannot acquire dependency installation lock")
	}
	lock.Close()
	defer os.Remove(lockPath)
	stage, err := os.MkdirTemp("", "gh-flarebyte-cargo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	staged, err := dependencies.PrepareCargoStage(root, stage, *report)
	if err != nil {
		return err
	}
	if err := staged.UseInstallPlan(plan); err != nil {
		return err
	}
	runner, err := dependencyCargoRunner(root, stage, staged.ValidateSDK)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for _, a := range staged.Artifacts() {
		b, err := dependencyCargoArtifact(ctx, a)
		if err != nil {
			return err
		}
		if err := staged.AddArtifact(b, a); err != nil {
			return err
		}
	}
	metadata, err := runner(ctx)
	if err != nil {
		return err
	}
	if err := staged.VerifyMetadata(metadata); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !plan.Unchanged(root) || !cargoInstallPolicy(root, report) {
		return fmt.Errorf("Cargo project or policy changed during installation")
	}
	// Keep commit backups on the destination filesystem for atomic rename/link.
	commit, err := os.MkdirTemp(root, ".gh-flarebyte-dep-")
	if err != nil {
		return err
	}
	retained := false
	defer func() {
		if !retained {
			_ = os.RemoveAll(commit)
		}
	}()
	if err := os.WriteFile(filepath.Join(commit, "Cargo.toml"), plan.Manifest, 0600); err != nil {
		return err
	}
	retained, err = commitCargoDependency(ctx, root, commit, plan, report)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(report)
	report.Installation = &dependencies.Installation{Version: report.Recommendation.Version, Files: []string{"Cargo.toml"}, AssessmentReference: dependencyDigest(encoded)}
	return nil
}

func commitCargoDependency(ctx context.Context, root, stage string, plan dependencies.CargoInstallPlan, report *dependencies.Report) (bool, error) {
	if !plan.Unchanged(root) || !cargoInstallPolicy(root, report) {
		return false, fmt.Errorf("Cargo project or policy changed before commit")
	}
	destination := filepath.Join(root, "Cargo.toml")
	info, err := os.Lstat(destination)
	if err != nil || !info.Mode().IsRegular() {
		return false, fmt.Errorf("unsafe Cargo manifest destination")
	}
	if !sameFileBytes(filepath.Join(stage, "Cargo.toml"), plan.Manifest) {
		return false, fmt.Errorf("staged Cargo pin changed")
	}
	if err := os.Chmod(filepath.Join(stage, "Cargo.toml"), info.Mode().Perm()); err != nil {
		return false, err
	}
	items := []dependencyCommitItem{{name: "Cargo.toml", data: plan.Manifest, backup: filepath.Join(stage, "backup-Cargo.toml")}}
	if err := dependencyRename(destination, items[0].backup); err != nil {
		return false, err
	}
	items[0].old = true
	fail := func(err error) (bool, error) { return dependencyCommitFailed(root, stage, items, "", err) }
	if !sameFileBytes(items[0].backup, plan.OriginalManifest()) || !plan.ContextUnchanged(root) || !cargoInstallPolicy(root, report) || ctx.Err() != nil {
		return fail(fmt.Errorf("Cargo inputs changed during commit"))
	}
	if err := os.Link(filepath.Join(stage, "Cargo.toml"), destination); err != nil {
		return fail(err)
	}
	items[0].placed = true
	if !sameFileBytes(destination, plan.Manifest) || !plan.ContextUnchanged(root) || !cargoInstallPolicy(root, report) || ctx.Err() != nil {
		return fail(fmt.Errorf("Cargo inputs changed during commit"))
	}
	return false, nil
}
