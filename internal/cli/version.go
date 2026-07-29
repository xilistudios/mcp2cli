package cli

var (
	appVersion   = "dev"
	appCommit    = "unknown"
	appBuildTime = "unknown"
)

// SetVersionInfo sets build-time version metadata (called from main).
func SetVersionInfo(version, commit, buildTime string) {
	appVersion = version
	appCommit = commit
	appBuildTime = buildTime
}
