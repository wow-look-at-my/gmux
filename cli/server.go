// server.go holds the hidden command a client starts the server with.

package cli

import (
	"errors"
	"log"

	"github.com/spf13/cobra"
	"github.com/wow-look-at-my/go-tmux/server"
)

var serverFlags struct {
	socket  string
	noStats bool
}

var serverCmd = &cobra.Command{
	Use:    "server",
	Short:  "Run the server in the foreground",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger := log.New(cmd.ErrOrStderr(), "", log.LstdFlags)
		path := serverFlags.socket
		if path == "" {
			path = socket()
		}
		srv := server.New(server.Options{Socket: path, NoStats: serverFlags.noStats, Logf: logger.Printf})
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
	serverCmd.Flags().StringVar(&serverFlags.socket, "socket", "", "path to listen on")
	serverCmd.Flags().BoolVar(&serverFlags.noStats, "no-stats", false, "leave the host's load off the bars")
	root.AddCommand(serverCmd)
}
