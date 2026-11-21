// kill_session.go holds the kill-session command.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var killSessionTarget string

var killSessionCmd = &cobra.Command{
	Use:   "kill-session",
	Short: "End a session and every program in it",
	Args:  cobra.NoArgs,
	RunE: func(*cobra.Command, []string) error {
		_, err := server.Ask(socket(), server.Request{Op: server.OpKillSession, Target: killSessionTarget})
		return err
	},
}

func init() {
	killSessionCmd.Flags().StringVarP(&killSessionTarget, "target", "t", "", "session to end (default: the one used last)")
	root.AddCommand(killSessionCmd)
}
