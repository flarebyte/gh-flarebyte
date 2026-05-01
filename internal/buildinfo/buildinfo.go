package buildinfo

// These values are overridden at build time via -ldflags.
var (
	Version   = "dev"
	CommitID  = "unknown"
	Date      = "unknown"
	GoVersion = "unknown"
)
