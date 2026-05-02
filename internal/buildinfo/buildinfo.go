// purpose: Define mutable build metadata fields that binaries expose through the version command.
// responsibilities: Hold default version/build values; provide ldflags override targets for build pipelines.
// architecture notes: Defaults are intentionally non-empty ("dev"/"unknown") so callers can detect missing injection without nil handling.
package buildinfo

// These values are overridden at build time via -ldflags.
var (
	Version   = "dev"
	CommitID  = "unknown"
	Date      = "unknown"
	GoVersion = "unknown"
)
