package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/goflex/goflex/pkg/version"
)

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print version",
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), version.Version())
		},
	}
}
