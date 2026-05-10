package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	frontendbuild "github.com/erazemkos/goflex/internal/build"
)

var runFrontendBuild = frontendbuild.Build
var runCSSBuild = frontendbuild.BuildCSS
var runAssetCopy = frontendbuild.CopyAssets
var runProductionBuild = frontendbuild.Production

func buildCmd() *cobra.Command {
	cfg := BuildConfig{Out: "bin/app", Minify: true}
	cmd := &cobra.Command{
		Use:   "build",
		Short: "build a production binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			lastBuild = cfg
			if err := runProductionBuild(cmd.Context(), frontendbuild.ProductionOptions{Dir: ".", Out: cfg.Out, Minify: cfg.Minify, Target: cfg.Target}); err != nil {
				return err
			}
			if cfg.Target == "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "built %s\n", cfg.Out)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "built %s for %s\n", cfg.Out, cfg.Target)
			}
			return nil
		},
	}
	buildFlags(cmd, &cfg)
	return cmd
}

func buildFlags(cmd *cobra.Command, cfg *BuildConfig) {
	cmd.Flags().StringVar(&cfg.Out, "out", "bin/app", "output binary path")
	cmd.Flags().BoolVar(&cfg.Minify, "minify", true, "minify assets")
	cmd.Flags().StringVar(&cfg.Target, "target", "", "optional GOOS/GOARCH or comma-separated targets")
}
