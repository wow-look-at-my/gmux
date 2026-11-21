// attach.go holds the attach-session command.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var attachTarget string

var attachCmd = &cobra.Command{
	Use:     "attach-session",
	Aliases: []string{"attach", "a"},
	Short:   "Attach this terminal to a running session",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return attach(cmd, server.Request{Target: attachTarget})
	},
}

func init() {
	attachCmd.Flags().StringVarP(&attachTarget, "target", "t", "", "session to attach to (default: the one used last)")
	root.AddCommand(attachCmd)
}
