package version

// populated by ldFlags
var Version string

func init() {
	if Version == "" {
		Version = "dev"
	}
}
