// purpose: Invoke the committed Gradle wrapper with isolated Kotlin reports and strict dependency locks.
package cli

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed assets/kotlin.init.gradle
var kotlinInitScript string

type kotlinProject struct{ Dir, Wrapper string }

func prepareKotlin() (kotlinProject, error) {
	var project kotlinProject
	project.Wrapper = "./gradlew"
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return project, fmt.Errorf("Kotlin command execution initially supports macOS and Linux")
	}
	if _, err := os.Stat("build.gradle.kts"); err != nil {
		if _, err := os.Stat("build.gradle"); err != nil {
			return project, fmt.Errorf("Kotlin requires a root build.gradle.kts or build.gradle")
		}
	}
	for _, path := range []string{"gradlew", "gradle/wrapper/gradle-wrapper.jar", "gradle/wrapper/gradle-wrapper.properties", "gradle.lockfile"} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return project, fmt.Errorf("Kotlin requires committed %s; generate the Gradle wrapper and dependency locks first", path)
		}
		if path == "gradlew" && info.Mode().Perm()&0o111 == 0 {
			return project, fmt.Errorf("Kotlin requires executable gradlew; run chmod +x gradlew")
		}
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-kotlin-")
	if err != nil {
		return project, err
	}
	project.Dir = dir
	if err := os.WriteFile(filepath.Join(dir, "init.gradle"), []byte(kotlinInitScript), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return project, err
	}
	return project, nil
}

func (p kotlinProject) run(env []string, operation string, tasks ...string) error {
	args := []string{"--no-daemon", "--console=plain", "--no-configuration-cache", "--no-build-cache", "--rerun-tasks", "--init-script", filepath.Join(p.Dir, "init.gradle"), "-Dgh.flarebyte.output=" + p.Dir, "-Dgh.flarebyte.operation=" + operation}
	args = append(args, tasks...)
	out, diagnostics, err := runCommandCapture(p.Wrapper, args, env)
	if err != nil {
		return commandError(err, strings.TrimSpace(diagnostics+"\n"+out))
	}
	return nil
}

func (p kotlinProject) tests() (goTestReport, error) {
	var report goTestReport
	paths, err := filepath.Glob(filepath.Join(p.Dir, "tests", "test", "TEST-*.xml"))
	if err != nil {
		return report, err
	}
	if len(paths) == 0 {
		return report, fmt.Errorf("Kotlin JUnit reports missing; expected fresh results from the root test task")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return report, err
		}
		part, err := parseKotlinTests(data)
		report.Tests += part.Tests
		report.Failed += part.Failed
		report.Skipped += part.Skipped
		report.Events = append(report.Events, part.Events...)
		if err != nil {
			return report, err
		}
	}
	if report.Tests == 0 || report.Failed > 0 {
		return report, fmt.Errorf("Kotlin test report: tests=%d failed=%d; expected passing tests", report.Tests, report.Failed)
	}
	return report, nil
}
