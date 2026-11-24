// capture.go holds the capture-pane command.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var captureTarget string

var captureCmd = &cobra.Command{
	Use:   "capture-pane",
	Short: "Print the text on the active pane of a session",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		resp, err := server.Ask(socket(), server.Request{Op: server.OpCapture, Target: captureTarget})
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), resp.Output)
		return nil
	},
}

func init() {
	captureCmd.Flags().StringVarP(&captureTarget, "target", "t", "", "session to read (default: the one used last)")
	// tmux prints with -p and fills a buffer without it. This command always prints.
	captureCmd.Flags().BoolP("print", "p", true, "print the text (always on)")
	root.AddCommand(captureCmd)
}
