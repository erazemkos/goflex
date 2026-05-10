package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	cfg := NewConfig{Template: "default"}
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "scaffold a new app",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Name = args[0]
			if cfg.Module == "" {
				cfg.Module = filepath.Base(filepath.Clean(cfg.Name))
			}
			lastNew = cfg
			return printStub(cmd, "new", 2)
		},
	}
	newFlags(cmd, &cfg)
	return cmd
}

func newFlags(cmd *cobra.Command, cfg *NewConfig) {
	cmd.Flags().StringVar(&cfg.Template, "template", "default", "template name")
	cmd.Flags().StringVar(&cfg.Module, "module", "", "module path")
}
