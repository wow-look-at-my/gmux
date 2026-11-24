// Command gmux is a terminal multiplexer.
package main

import (
	"os"

	"github.com/wow-look-at-my/go-tmux/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
