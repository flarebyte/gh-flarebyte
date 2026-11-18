// purpose: Orchestrate local dependency assessments without unchecked native installs.
// responsibilities: Discover policy, render reports and fail closed on incomplete evidence.
// architecture notes: Dependency-specific exit codes avoid changing existing repository command meanings.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

const (
	ExitDependencyUnknown     = 8
	ExitDependencyUnsupported = 9
	ExitDependencyFailure     = 10
)

var assessDependency = func(ctx context.Context, root, system, name string, version *string, policy dependencies.Policy, now time.Time) dependencies.Report {
	client := dependencies.NewClient()
	client.GitHubToken = dependencyGitHubToken(root)
	if dependencies.SupportsPackageAdvice(system) {
		return client.AssessPackage(ctx, system, name, version, policy, now)
	}
	return client.AssessLocal(ctx, root, system, name, version, policy, now)
}

func dependencyRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		info, err := os.Stat(filepath.Join(dir, ".git"))
		if err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("dep requires a local Git repository")
		}
		dir = parent
	}
}

func dependencyPolicy(root string) (dependencies.Policy, string, error) {
	path := filepath.Join(root, config.DefaultPath)
	cfg, err := config.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return dependencies.DefaultPolicy(14), "", nil
	}
	if err != nil {
		return dependencies.Policy{}, path, err
	}
	return dependencies.DefaultPolicy(cfg.Dependencies.MinAgeDays), path, nil
}

func runDependency(ctx context.Context, action, system, arg string, jsonOutput, explain bool, stdout, stderr io.Writer) Result {
	name, version, err := dependencies.Parse(system, arg)
	if err != nil {
		return dependencyError(system+":"+arg, "INVALID_INPUT", err, ExitUsage, jsonOutput, stdout, stderr)
	}
	if action == "install" && dependencies.SupportsPackageAdvice(system) && (jsonOutput || explain) {
		return dependencyError(system+":"+name, "INVALID_INPUT", fmt.Errorf("%s install uses plain output; --json and --explain are check-only", system), ExitUsage, false, stdout, stderr)
	}
	if action == "install" && version != nil && !dependencies.SupportsPackageAdvice(system) {
		return dependencyError(system+":"+name, "INVALID_INPUT", fmt.Errorf("install selects a recommendation; use check for exact versions"), ExitUsage, jsonOutput, stdout, stderr)
	}
	root, err := dependencyRoot()
	if err != nil {
		return dependencyError(system+":"+name, "LOCAL_REPOSITORY_REQUIRED", err, ExitUsage, jsonOutput, stdout, stderr)
	}
	policy, path, err := dependencyPolicy(root)
	if err != nil {
		return dependencyError(system+":"+name, "INVALID_CONFIG", err, ExitUsage, jsonOutput, stdout, stderr)
	}
	if dependencies.SupportsPackageAdvice(system) {
		policy = dependencies.PackageAdvicePolicy(policy.MinAgeDays)
	}
	assessmentCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	report := assessDependency(assessmentCtx, root, system, name, version, policy, time.Now().UTC())
	cancel()
	if !dependencies.SupportsPackageAdvice(system) && report.Decision == dependencies.Allow && report.Scope != "project-resolution" {
		report.Decision = dependencies.Unknown
		report.Recommendation = nil
		report.Reasons = append(report.Reasons, "INSUFFICIENT_EVIDENCE")
		report.Evidence.Missing = append(report.Evidence.Missing, "PROJECT_RESOLUTION")
	}
	if report.Details != nil {
		report.Details.ConfigPath = path
	}
	code := ExitDependencyUnknown
	if report.Error != nil && report.Error.Code == "VERSION_NOT_FOUND" {
		code = ExitUsage
	} else if report.Decision == dependencies.Deny {
		code = ExitFailure
	} else if report.Decision == dependencies.Allow {
		code = ExitOK
	} else if (system == "pub" || system == "swift") && !dependencies.SupportsPackageAdvice(system) {
		code = ExitDependencyUnsupported
	}
	if action == "install" && dependencies.SupportsPackageAdvice(system) {
		if code != ExitOK || report.Recommendation == nil || report.Scope != "package" || report.Policy != dependencies.PackageAdvicePolicyID || len(report.Evidence.Missing) > 0 {
			if err := writeDependencyReport(stderr, report, false); err != nil {
				return Result{ExitCode: ExitDependencyFailure, Err: err}
			}
			if code == ExitOK {
				code = ExitDependencyUnknown
				_, _ = fmt.Fprintf(stderr, "No complete package recommendation; %s was not started.\n", system)
			}
			return Result{ExitCode: code}
		}
		selected := report.Recommendation.Version
		if version != nil {
			selected = *version
		}
		dir, err := os.Getwd()
		if err != nil {
			return dependencyError(report.Package, "INSTALL_FAILED", err, ExitDependencyFailure, false, stdout, stderr)
		}
		switch system {
		case "npm":
			return delegateNPM(ctx, dir, name, selected, os.Stdin, stdout, stderr)
		case "go":
			return delegateGo(ctx, dir, name, selected, os.Stdin, stdout, stderr)
		case "pypi":
			return delegatePyPI(ctx, dir, name, selected, os.Stdin, stdout, stderr)
		case "cargo":
			return delegateCargo(ctx, dir, name, selected, os.Stdin, stdout, stderr)
		case "swift":
			return printSwiftDependency(name, selected, stdout, stderr)
		case "maven":
			return printMavenDependency(dir, name, selected, stdout, stderr)
		default:
			return Result{ExitCode: ExitDependencyUnsupported}
		}
	}
	if action == "install" && code == ExitOK {
		if err := installDependency(ctx, root, &report); err != nil {
			report.Decision = dependencies.Unknown
			report.Recommendation = nil
			code = ExitDependencyFailure
			errorCode := "INSTALL_FAILED"
			if errors.Is(err, dependencies.ErrInstallUnsupported) {
				code = ExitDependencyUnsupported
				errorCode = "INSTALL_UNSUPPORTED"
			}
			report.Error = &dependencies.ReportError{Code: errorCode, Message: err.Error()}
		}
	} else if action == "install" && code != ExitFailure && code != ExitUsage {
		report.Decision = dependencies.Unknown
		report.Recommendation = nil
		report.Error = &dependencies.ReportError{Code: "INSTALL_UNSUPPORTED", Message: "Guarded installation is unavailable until identity, maintainer authority and safe project resolution are established"}
		report.Reasons = append(report.Reasons, "UNSUPPORTED_COVERAGE")
		code = ExitDependencyUnsupported
	}
	if !explain {
		report.Details = nil
	}
	if err := writeDependencyReport(stdout, report, jsonOutput); err != nil {
		return Result{ExitCode: ExitDependencyFailure, Err: err}
	}
	return Result{ExitCode: code}
}

func dependencyError(pkg, code string, err error, exit int, jsonOutput bool, stdout, stderr io.Writer) Result {
	message := "dependency command failed"
	if safe := dependencies.Description(err.Error()); safe != nil {
		message = *safe
	}
	if len(pkg) > 512 {
		pkg = ""
	}
	if jsonOutput {
		r := dependencies.NewReport("", "", nil, dependencies.DefaultPolicy(14), time.Now().UTC())
		r.Package = pkg
		r.Details = nil
		r.Reasons = []string{code}
		r.Error = &dependencies.ReportError{Code: code, Message: message}
		if writeErr := writeDependencyReport(stdout, r, true); writeErr != nil {
			return Result{ExitCode: ExitDependencyFailure, Err: writeErr}
		}
	} else {
		_, _ = fmt.Fprintln(stderr, message)
	}
	return Result{ExitCode: exit, Err: err}
}

func dependencyJSONRequested(args []string) bool {
	requested := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch arg {
		case "--json", "--json=true":
			requested = true
		case "--json=false":
			requested = false
		}
	}
	return requested
}

func writeDependencyReport(w io.Writer, r dependencies.Report, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(w).Encode(r)
	}
	if _, err := fmt.Fprintf(w, "%s %s", strings.ToUpper(r.Decision), r.Package); err != nil {
		return err
	}
	if r.Requested != nil {
		if _, err := fmt.Fprintf(w, "@%s", *r.Requested); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if r.Description != nil {
		if _, err := fmt.Fprintln(w, *r.Description); err != nil {
			return err
		}
	}
	if r.Installation != nil {
		if _, err := fmt.Fprintf(w, "Installed: %s\n", r.Installation.Version); err != nil {
			return err
		}
	} else if r.Recommendation != nil {
		if _, err := fmt.Fprintf(w, "Recommended: %s\n", r.Recommendation.Version); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Reasons: %s\nMissing: %s\n", strings.Join(r.Reasons, ", "), strings.Join(r.Evidence.Missing, ", ")); err != nil {
		return err
	}
	if r.Error != nil {
		if _, err := fmt.Fprintln(w, r.Error.Message); err != nil {
			return err
		}
	}
	if r.Coverage != "" {
		if _, err := fmt.Fprintf(w, "Coverage: %s\nWarnings: %s\n", r.Coverage, strings.Join(r.Warnings, ", ")); err != nil {
			return err
		}
	}
	if r.Details != nil {
		return json.NewEncoder(w).Encode(r.Details)
	}
	return nil
}
