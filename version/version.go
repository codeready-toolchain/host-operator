package version

// we can't have those variables filled by the `-ldflags="-X ..."` in the `cmd/manager` package because
// it's imported as `main`

var (
	// Version the operator version
	Version = "0.0.1"
	// Commit the commit hash corresponding to the code that was built. Can be suffixed with `-dirty`
	Commit = "unknown"
	// BuildTime the time of build of the binary
	BuildTime = "unknown"
)
