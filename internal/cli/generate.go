package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/erazemkos/goflex/internal/gen"
)

var runGenerate = gen.Generate

func generateCmd() *cobra.Command {
	cfg := GenerateConfig{Only: "all"}
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "run code generators",
		RunE: func(cmd *cobra.Command, args []string) error {
			lastGenerate = cfg
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			changed, err := runGenerate(root, cfg.Only)
			if err != nil {
				return err
			}
			if changed {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "generated api files")
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no changes")
			}
			return nil
		},
	}
	generateFlags(cmd, &cfg)
	return cmd
}

func generateFlags(cmd *cobra.Command, cfg *GenerateConfig) {
	cmd.Flags().StringVar(&cfg.Only, "only", "all", "api|routes|all")
}
