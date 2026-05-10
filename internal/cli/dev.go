package cli

import (
	"context"
	"os/signal"
	"syscall"

	frontendbuild "github.com/goflex/goflex/internal/build"
	"github.com/goflex/goflex/internal/devserver"
	"github.com/spf13/cobra"
)

var runDevServer = devserver.Run

func devCmd() *cobra.Command {
	cfg := DevConfig{Addr: ":3000"}
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "run dev server",
		RunE: func(cmd *cobra.Command, args []string) error {
			lastDev = cfg
			if err := runCSSBuild(frontendbuild.CSSOptions{Dir: "."}); err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return runDevServer(ctx, devserver.Options{Addr: cfg.Addr, Dir: ".", Out: cmd.OutOrStdout()})
		},
	}
	devFlags(cmd, &cfg)
	return cmd
}

func devFlags(cmd *cobra.Command, cfg *DevConfig) {
	cmd.Flags().StringVar(&cfg.Addr, "addr", ":3000", "listen address")
	cmd.Flags().BoolVar(&cfg.NoOpen, "no-open", false, "do not open browser")
}
