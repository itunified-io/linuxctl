// Command linuxctl is the entry point for the linuxctl CLI.
package main

import (
	"fmt"
	"os"

	"github.com/itunified-io/linuxctl/internal/root"
)

// These are injected at build time via -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	if err := root.Execute(root.BuildInfo{Version: Version, Commit: Commit, Date: Date}); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
