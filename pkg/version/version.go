// Package version exposes the linuxctl build metadata.
package version

// Info describes a build.
type Info struct {
	Version string
	Commit  string
	Date    string
}
