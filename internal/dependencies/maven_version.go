// purpose: Recognize clearly labelled Maven prereleases without implementing ComparableVersion.
// responsibilities: Leave unrecognized qualifiers unresolved; numeric release ordering uses the bounded shared parser.
package dependencies

import "regexp"

var mavenPrerelease = regexp.MustCompile(`(?i)^[0-9]+(?:\.[0-9]+){0,3}[-.](?:snapshot|alpha|beta|milestone|m|rc|cr)(?:[-.]?[0-9]+)?$`)
