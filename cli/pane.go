// pane.go holds the pane command, the group of the commands that act on the active pane of a session.

package cli

var paneCmd = group("pane", "Type into and read the active pane of a session")

func init() { root.AddCommand(paneCmd) }
