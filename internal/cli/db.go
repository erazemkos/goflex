package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/erazemkos/goflex/pkg/db/migrate"
)

var (
	runDBCreate   = migrate.Create
	runDBMigrate  = migrate.UpWith
	runDBRollback = migrate.DownWith
	runDBStatus   = migrate.StatusWith
)

func dbCmd() *cobra.Command {
	root := &cobra.Command{Use: "db", Short: "database migrations"}
	migrateCfg := defaultDBConfig()
	rollbackCfg := defaultDBConfig()
	rollbackCfg.Step = 1
	createCfg := defaultDBConfig()
	statusCfg := defaultDBConfig()

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "apply database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			lastDB = migrateCfg
			if migrateCfg.Auto {
				return fmt.Errorf("auto migrate is configured in application code; CLI SQL migrations do not use --auto yet")
			}
			if err := runDBMigrate(migrate.Config{Driver: migrateCfg.Driver, DSN: migrateCfg.DSN, Dir: migrateCfg.Dir}); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "migrations applied")
			return nil
		},
	}
	rollbackCmd := &cobra.Command{
		Use:   "rollback",
		Short: "rollback database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			lastDB = rollbackCfg
			if err := runDBRollback(migrate.Config{Driver: rollbackCfg.Driver, DSN: rollbackCfg.DSN, Dir: rollbackCfg.Dir}, rollbackCfg.Step); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rolled back %d migration(s)\n", rollbackCfg.Step)
			return nil
		},
	}
	createCmd := &cobra.Command{
		Use:   "create <name>",
		Short: "create a database migration",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			createCfg.Name = args[0]
			lastDB = createCfg
			files, err := runDBCreate(createCfg.Dir, createCfg.Name)
			if err != nil {
				return err
			}
			for _, file := range files {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), file)
			}
			return nil
		},
	}
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "show database migration status",
		RunE: func(cmd *cobra.Command, args []string) error {
			lastDB = statusCfg
			info, err := runDBStatus(migrate.Config{Driver: statusCfg.Driver, DSN: statusCfg.DSN, Dir: statusCfg.Dir})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%d migrations, %d applied, %d pending\n", info.Total, info.Applied, info.Pending)
			return nil
		},
	}
	dbMigrateFlags(migrateCmd, &migrateCfg)
	dbRollbackFlags(rollbackCmd, &rollbackCfg)
	dbCommonFlags(createCmd, &createCfg)
	dbCommonFlags(statusCmd, &statusCfg)
	root.AddCommand(migrateCmd, rollbackCmd, createCmd, statusCmd)
	return root
}

func defaultDBConfig() DBConfig {
	return DBConfig{Dir: "db/migrations", DSN: "goflex.db", Driver: "sqlite"}
}

func dbMigrateFlags(cmd *cobra.Command, cfg *DBConfig) {
	dbCommonFlags(cmd, cfg)
	cmd.Flags().IntVar(&cfg.Step, "step", 0, "max steps")
	cmd.Flags().BoolVar(&cfg.Auto, "auto", false, "use application AutoMigrate hook in dev")
}

func dbRollbackFlags(cmd *cobra.Command, cfg *DBConfig) {
	dbCommonFlags(cmd, cfg)
	cmd.Flags().IntVar(&cfg.Step, "step", 1, "rollback steps")
}

func dbCommonFlags(cmd *cobra.Command, cfg *DBConfig) {
	cmd.Flags().StringVar(&cfg.Dir, "dir", "db/migrations", "migration directory")
	cmd.Flags().StringVar(&cfg.DSN, "dsn", "goflex.db", "database DSN")
	cmd.Flags().StringVar(&cfg.Driver, "driver", "sqlite", "sqlite|postgres|mysql")
}
