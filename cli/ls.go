// ls.go holds the list-sessions command.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var lsCmd = &cobra.Command{
	Use:     "list-sessions",
	Aliases: []string{"ls"},
	Short:   "List the sessions of the server",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		resp, err := server.Ask(socket(), server.Request{Op: server.OpList})
		if err != nil {
			return err
		}
		for _, s := range resp.Sessions {
			line := fmt.Sprintf("%s: %d windows (created %s) [%dx%d]", s.Name, s.Windows,
				time.Unix(s.Created, 0).Format("Mon Jan _2 15:04:05 2006"), s.Cols, s.Rows)
			if s.Attached > 0 {
				line += " (attached)"
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
		return nil
	},
}

func init() { root.AddCommand(lsCmd) }
