// Package cli is the gmux command line: one file per command, each registering itself.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/client"
	"github.com/wow-look-at-my/go-tmux/server"
	"github.com/wow-look-at-my/tml/term"
)

var (
	socketPath string
	socketName string
)

var root = &cobra.Command{
	Use:   "gmux",
	Short: "A terminal multiplexer with status bars that take the mouse",
	Long: "gmux keeps terminal sessions running on a server and attaches terminals to them.\n" +
		"With no command it starts a new session and attaches to it.",
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return createAndAttach(cmd, server.Request{})
	},
}

func init() {
	root.PersistentFlags().StringVarP(&socketPath, "socket-path", "S", "", "path of the server socket")
	root.PersistentFlags().StringVarP(&socketName, "socket-name", "L", "", "name of the server socket in the default directory")
}

// The seams a test replaces: where the terminal comes from, and how a server is started.
var (
	terminal = func() client.Terminal { return client.Terminal{In: os.Stdin, Out: os.Stdout} }
	start    = func(socket string) error {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return client.Start(socket, exe)
	}
)

// Execute runs the command line and reports the process's exit code.
func Execute(version string, args []string, stdout, stderr io.Writer) int {
	root.Version = version
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(stderr, "gmux:", err)
		return 1
	}
	return 0
}

// socket is the path of the server this invocation talks to.
func socket() string { return server.SocketPath(socketPath, socketName) }

// errNested is what an attach from inside a session reports.
var errNested = errors.New("this terminal is already inside a gmux session; unset $GMUX to nest one anyway")

// attach hands the terminal to the server and prints why it came back.
func attach(cmd *cobra.Command, req server.Request) error {
	if os.Getenv(client.NestEnv) != "" {
		return errNested
	}
	req.Light = os.Getenv("GMUX_THEME") == "light"
	resp, err := client.Attach(socket(), req, terminal())
	if err != nil {
		return err
	}
	switch resp.Exit {
	case server.ExitDetached:
		fmt.Fprintf(cmd.OutOrStdout(), "[detached (from session %s)]\n", resp.Session)
	case server.ExitClosed:
		fmt.Fprintln(cmd.OutOrStdout(), "[server exited]")
	default:
		fmt.Fprintln(cmd.OutOrStdout(), "[exited]")
	}
	return nil
}

// describe fills in what a new session takes from the place the command runs: the environment. The directory, and
// the size of the terminal when there is one.
func describe(req server.Request) server.Request {
	req.Env = os.Environ()
	if req.Dir == "" {
		req.Dir, _ = os.Getwd()
	}
	if cols, rows, err := term.Size(terminal().Out); err == nil {
		req.Cols, req.Rows = cols, max(1, rows-2)
	}
	return req
}

// createAndAttach starts a session and attaches to it, starting the server when none runs.
func createAndAttach(cmd *cobra.Command, req server.Request) error {
	if os.Getenv(client.NestEnv) != "" {
		return errNested
	}
	if err := start(socket()); err != nil {
		return err
	}
	req = describe(req)
	req.Create = true
	return attach(cmd, req)
}
