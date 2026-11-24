// server_kill.go holds the server kill command.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var killServerCmd = &cobra.Command{
	Use:   "kill",
	Short: "End the server and every session",
	Args:  cobra.NoArgs,
	RunE: func(*cobra.Command, []string) error {
		_, err := server.Ask(socket(), server.Request{Op: server.OpKillServer})
		return err
	},
}

func init() { serverCmd.AddCommand(killServerCmd) }
