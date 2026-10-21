// Package version exposes build metadata that is injected at link time.
package version

import (
	"fmt"
	"runtime"
)

// These values are overwritten with -ldflags -X at build time.
// See the Makefile and the CI workflow.
var (
	Version   = "dev"
	BuildTime = "unknown"
	Commit    = "unknown"
)

// String returns a single-line human readable description of the build.
func String() string {
	return fmt.Sprintf("lite-upf %s (commit %s, built %s, %s %s/%s)",
		Version, Commit, BuildTime, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
