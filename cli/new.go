// new.go holds the new-session command.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var newFlags struct {
	name     string
	dir      string
	detached bool
	attach   bool
}

var newCmd = &cobra.Command{
	Use:     "new-session [flags] [-- command...]",
	Aliases: []string{"new"},
	Short:   "Start a session, and attach to it unless -d is given",
	RunE: func(cmd *cobra.Command, args []string) error {
		req := server.Request{Name: newFlags.name, Dir: newFlags.dir, Command: args}
		if newFlags.attach && newFlags.name != "" && sessionExists(newFlags.name) {
			return attach(cmd, server.Request{Target: newFlags.name})
		}
		if !newFlags.detached {
			return createAndAttach(cmd, req)
		}
		if err := start(socket()); err != nil {
			return err
		}
		req = describe(req)
		req.Op = server.OpNew
		_, err := server.Ask(socket(), req)
		return err
	},
}

// sessionExists reports whether a running server has a session with exactly this name.
func sessionExists(name string) bool {
	resp, err := server.Ask(socket(), server.Request{Op: server.OpList})
	if err != nil {
		return false
	}
	for _, s := range resp.Sessions {
		if s.Name == name {
			return true
		}
	}
	return false
}

func init() {
	newCmd.Flags().StringVarP(&newFlags.name, "name", "s", "", "name of the session")
	newCmd.Flags().StringVarP(&newFlags.dir, "dir", "c", "", "working directory of the session")
	newCmd.Flags().BoolVarP(&newFlags.detached, "detached", "d", false, "do not attach to the new session")
	newCmd.Flags().BoolVarP(&newFlags.attach, "attach", "A", false, "attach to the session named by -s when it exists")
	root.AddCommand(newCmd)
}
