package cli

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/loggers"
	"github.com/algorand/conduit/conduit/pipeline"
	"github.com/algorand/conduit/pkg/cli/internal/initialize"
	"github.com/algorand/conduit/pkg/cli/internal/list"
	"github.com/algorand/conduit/version"
)

var (
	logger *log.Logger

	// ConduitCmd is the root command for conduit
	ConduitCmd = MakeConduitCmd()

	//Banner is the banner for conduit's pipeline
	//go:embed banner.txt
	Banner string
)

const (
	conduitEnvVar = "CONDUIT_DATA_DIR"
)

// runConduitCmdWithConfig run the main logic with a supplied conduit config
func runConduitCmdWithConfig(args *data.Args) error {
	defer pipeline.HandlePanic(logger)

	if args.ConduitDataDir == "" {
		args.ConduitDataDir = os.Getenv(conduitEnvVar)
	}

	if args.ConduitDataDir == "" {
		return fmt.Errorf("the data directory is required and must be provided with a command line option or the '%s' environment variable", conduitEnvVar)
	}

	pCfg, err := data.MakePipelineConfig(args)
	if err != nil {
		return err
	}

	// Initialize logger
	level, err := log.ParseLevel(pCfg.LogLevel)
	if err != nil {
		var levels []string
		for _, l := range log.AllLevels {
			levels = append(levels, l.String())
		}
		return fmt.Errorf("invalid configuration: '%s' is not a valid log level, valid levels: %s", pCfg.LogLevel, strings.Join(levels, ", "))
	}

	logger, err = loggers.MakeThreadSafeLogger(level, pCfg.LogFile)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	logger.Infof("Starting Conduit %s", version.LongVersion())
	logger.Infof("Using data directory: %s", args.ConduitDataDir)
	logger.Info("Conduit configuration is valid")

	if !pCfg.HideBanner {
		fmt.Print(Banner)
	}

	if pCfg.LogFile != "" {
		fmt.Printf("Writing logs to file: %s\n", pCfg.LogFile)
	} else {
		fmt.Println("Writing logs to console.")
	}

	ctx := context.Background()
	pipeline, err := pipeline.MakePipeline(ctx, pCfg, logger)
	if err != nil {
		err = fmt.Errorf("pipeline creation error: %w", err)

		// Suppress log, it is about to be printed to stderr.
		if pCfg.LogFile != "" {
			logger.Error(err)
		}
		return err
	}

	err = pipeline.Init()
	if err != nil {
		// Suppress log, it is about to be printed to stderr.
		if pCfg.LogFile != "" {
			logger.Error(err)
		}
		return fmt.Errorf("pipeline init error: %w", err)
	}
	pipeline.Start()
	defer pipeline.Stop()
	pipeline.Wait()
	return pipeline.Error()
}

// MakeConduitCmdWithUtilities creates the main cobra command with all utilities
func MakeConduitCmdWithUtilities() *cobra.Command {
	cmd := MakeConduitCmd()
	cmd.AddCommand(initialize.InitCommand)
	cmd.AddCommand(list.Command)
	return cmd
}

// MakeConduitCmd creates the main cobra command, initializes flags
func MakeConduitCmd() *cobra.Command {
	cfg := &data.Args{}
	var vFlag bool
	cmd := &cobra.Command{
		Use:   "conduit",
		Short: "Run the Conduit framework.",
		Long: `Conduit is a framework for ingesting blocks from the Algorand blockchain
into external applications. It is designed as a modular plugin system that
allows users to configure their own data pipelines.

You must provide a data directory containing a file named conduit.yml. The
file configures pipeline and all enabled plugins.

See other subcommands for further built in utilities and information.

Detailed documentation is online: https://github.com/algorand/conduit`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			err := runConduitCmdWithConfig(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nExiting with error:\n%s.\n", err)
				os.Exit(1)
			}
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if vFlag {
				fmt.Printf("%s\n", version.LongVersion())
				os.Exit(0)
			}
		},
	}
	cmd.Flags().StringVarP(&cfg.ConduitDataDir, "data-dir", "d", "", "Set the Conduit data directory. If not set the CONDUIT_DATA_DIR environment variable is used.")
	cmd.Flags().Uint64VarP(&cfg.NextRoundOverride, "next-round-override", "r", 0, "Set the starting round. Overrides next-round in metadata.json. Some exporters do not support overriding the starting round.")
	cmd.Flags().BoolVarP(&vFlag, "version", "v", false, "Print the Conduit version.")
	// No need for shell completions.
	cmd.CompletionOptions.DisableDefaultCmd = true

	return cmd
}
