// Command gmux is a terminal multiplexer.
package main

import (
	"os"

	"github.com/wow-look-at-my/go-tmux/cli"
)

// Version is filled in by a release build.
var Version string

func main() {
	os.Exit(cli.Execute(Version, os.Args[1:], os.Stdout, os.Stderr))
}
