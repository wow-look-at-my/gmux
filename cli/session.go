// session.go holds the session command, the group of the commands that act on a session.

package cli

var sessionCmd = group("session", "Start, attach to, rename, end and list sessions")

func init() { root.AddCommand(sessionCmd) }
