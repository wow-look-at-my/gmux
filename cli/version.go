// version.go holds the version command.

package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the release and its commit. The linker of a release build fills it.
var Version string

// errNoVersion is what a binary reports when no release build filled in its version.
var errNoVersion = errors.New("this binary was not built for a release, so it has no version")

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the release this binary was built as",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if root.Version == "" {
			return errNoVersion
		}
		fmt.Fprintf(cmd.OutOrStdout(), "gmux version %s\n", root.Version)
		return nil
	},
}

func init() { root.AddCommand(versionCmd) }
