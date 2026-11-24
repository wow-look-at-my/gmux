// session_rename.go holds the session rename command.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var renameTarget string

var renameCmd = &cobra.Command{
	Use:   "rename [flags] name",
	Short: "Give a session a new name",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		_, err := server.Ask(socket(), server.Request{Op: server.OpRename, Target: renameTarget, Name: args[0]})
		return err
	},
}

func init() {
	renameCmd.Flags().StringVarP(&renameTarget, "target", "t", "", "session to rename (default: the one used last)")
	sessionCmd.AddCommand(renameCmd)
}
