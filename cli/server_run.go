// server_run.go holds the hidden command a client starts the server with.

package cli

import (
	"errors"
	"log"

	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var serverRunFlags struct {
	socket  string
	noStats bool
}

var serverRunCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run the server in the foreground",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger := log.New(cmd.ErrOrStderr(), "", log.LstdFlags)
		path := serverRunFlags.socket
		if path == "" {
			path = socket()
		}
		srv := server.New(server.Options{Socket: path, NoStats: serverRunFlags.noStats, Logf: logger.Printf})
		if err := srv.Listen(); err != nil {
			// A second server started in the same moment has nothing to do: the first one answers.
			if errors.Is(err, server.ErrRunning) {
				return nil
			}
			return err
		}
		logger.Printf("listening on %s", path)
		return srv.Serve()
	},
}

func init() {
	serverRunCmd.Flags().StringVar(&serverRunFlags.socket, "socket", "", "path to listen on")
	serverRunCmd.Flags().BoolVar(&serverRunFlags.noStats, "no-stats", false, "leave the host's load off the bars")
	serverCmd.AddCommand(serverRunCmd)
}
