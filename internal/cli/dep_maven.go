// purpose: Print a recommended Maven dependency declaration without editing or executing a build.
// responsibilities: Detect only regular local build files; label manual/ambiguous instructions honestly.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

func printMavenDependency(dir, name, version string, stdout, stderr io.Writer) Result {
	canonical, exact, err := dependencies.Parse("maven", name+"@"+version)
	if err != nil || canonical != name || exact == nil || *exact != version {
		_, _ = fmt.Fprintln(stderr, "invalid exact Maven coordinate")
		return Result{ExitCode: ExitUsage}
	}
	formats := []string{}
	for _, file := range []string{"build.gradle.kts", "build.gradle", "pom.xml"} {
		if info, err := os.Lstat(filepath.Join(dir, file)); err == nil && info.Mode().IsRegular() {
			formats = append(formats, file)
		}
	}
	var text strings.Builder
	text.WriteString("PRINT ONLY — nothing installed.\n")
	kotlin := fmt.Sprintf("implementation(\"%s:%s\")", name, version)
	groovy := fmt.Sprintf("implementation '%s:%s'", name, version)
	group, artifact, _ := strings.Cut(name, ":")
	xml := fmt.Sprintf("<dependency>\n  <groupId>%s</groupId>\n  <artifactId>%s</artifactId>\n  <version>%s</version>\n</dependency>", group, artifact, version)
	if len(formats) == 1 {
		fmt.Fprintf(&text, "Add to the dependencies section of %s:\n", formats[0])
		switch formats[0] {
		case "build.gradle.kts":
			text.WriteString(kotlin)
		case "build.gradle":
			text.WriteString(groovy)
		case "pom.xml":
			text.WriteString(xml)
		}
	} else {
		text.WriteString("Build format is missing or ambiguous; choose the declaration manually.\n")
		fmt.Fprintf(&text, "Gradle Kotlin DSL: %s\nGradle Groovy DSL: %s\nMaven:\n%s", kotlin, groovy, xml)
	}
	text.WriteString("\nUse your normal Maven/Gradle workflow to resolve dependencies and build.\n")
	if _, err := io.WriteString(stdout, text.String()); err != nil {
		return Result{ExitCode: ExitDependencyFailure, Err: err}
	}
	return Result{ExitCode: ExitOK}
}
