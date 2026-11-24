// kill_server.go holds the kill-server command.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var killServerCmd = &cobra.Command{
	Use:   "kill-server",
	Short: "End the server and every session",
	Args:  cobra.NoArgs,
	RunE: func(*cobra.Command, []string) error {
		_, err := server.Ask(socket(), server.Request{Op: server.OpKillServer})
		return err
	},
}

func init() { root.AddCommand(killServerCmd) }
