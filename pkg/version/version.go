package version

var buildVersion string

// Version returns the build-time version, or "dev" when unset.
func Version() string {
	if buildVersion == "" {
		return "dev"
	}
	return buildVersion
}

func setForTest(v string) func() {
	old := buildVersion
	buildVersion = v
	return func() { buildVersion = old }
}
