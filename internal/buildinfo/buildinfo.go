// Package buildinfo exposes release metadata injected by the build pipeline.
package buildinfo

import "fmt"

var (
	Version = "1.0.0-dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func String() string {
	if Commit == "" || Commit == "unknown" {
		return "Lumeleaf " + Version
	}
	return fmt.Sprintf("Lumeleaf %s (%s, %s)", Version, Commit, Date)
}
