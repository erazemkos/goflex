package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	glog "github.com/erazemkos/goflex/pkg/log"
)

type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// Execute runs the CLI and maps framework errors to stable process exit codes.
func Execute(args []string, stdout, stderr io.Writer) int {
	cmd := NewRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		var ue usageError
		if errors.As(err, &ue) || strings.Contains(err.Error(), "unknown command") {
			return 2
		}
		return 1
	}
	return 0
}

// NewRootCommand builds the top-level goflex command tree.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "goflex",
		Short:         "GoFlex full-stack Go framework",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		glog.Debugf(cmd.ErrOrStderr(), "running %s", cmd.CommandPath())
	}
	root.AddCommand(newCmd(), devCmd(), buildCmd(), generateCmd(), dbCmd(), versionCmd())
	return root
}

func requireArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usageError{fmt.Errorf("requires exactly %d argument(s)", n)}
		}
		return nil
	}
}

type NewConfig struct{ Name, Template, Module string }
type DevConfig struct {
	Addr   string
	NoOpen bool
}
type BuildConfig struct {
	Out    string
	Minify bool
	Target string
}
type GenerateConfig struct{ Only string }
type DBConfig struct {
	Step   int
	Name   string
	Dir    string
	DSN    string
	Driver string
	Auto   bool
}

var lastNew NewConfig
var lastDev DevConfig
var lastBuild BuildConfig
var lastGenerate GenerateConfig
var lastDB DBConfig

// ParsedNewConfig returns the most recently parsed `new` config for tests.
func ParsedNewConfig() NewConfig { return lastNew }

// ParsedDevConfig returns the most recently parsed `dev` config for tests.
func ParsedDevConfig() DevConfig { return lastDev }

// ParsedBuildConfig returns the most recently parsed `build` config for tests.
func ParsedBuildConfig() BuildConfig { return lastBuild }

// ParsedGenerateConfig returns the most recently parsed `generate` config for tests.
func ParsedGenerateConfig() GenerateConfig { return lastGenerate }

// ParsedDBConfig returns the most recently parsed `db` config for tests.
func ParsedDBConfig() DBConfig { return lastDB }
