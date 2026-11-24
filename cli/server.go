// server.go holds the server command, the group of the commands that act on the server.

package cli

var serverCmd = group("server", "End the server")

func init() { root.AddCommand(serverCmd) }
