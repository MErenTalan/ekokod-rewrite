// Package buildinfo exposes the values stamped into the binary at link time.
package buildinfo

import "runtime"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Info describes the running build.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
}

// Get returns the build information stamped into this binary.
func Get() Info {
	return Info{Version: version, Commit: commit, Date: date, Go: runtime.Version()}
}
