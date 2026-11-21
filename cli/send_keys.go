// send_keys.go holds the send-keys command.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var sendKeysFlags struct {
	target  string
	literal bool
}

var sendKeysCmd = &cobra.Command{
	Use:   "send-keys [flags] key...",
	Short: "Type keys into the active pane of a session",
	Long: "Each argument is a key name (enter, tab, esc, up, f5, ctrl+c, or tmux's Enter and C-c) or text to type.\n" +
		"With -l every argument is text.",
	Args: cobra.MinimumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		_, err := server.Ask(socket(), server.Request{
			Op: server.OpSendKeys, Target: sendKeysFlags.target, Keys: args, Literal: sendKeysFlags.literal,
		})
		return err
	},
}

func init() {
	sendKeysCmd.Flags().StringVarP(&sendKeysFlags.target, "target", "t", "", "session to type into (default: the one used last)")
	sendKeysCmd.Flags().BoolVarP(&sendKeysFlags.literal, "literal", "l", false, "send every argument as text")
	root.AddCommand(sendKeysCmd)
}
