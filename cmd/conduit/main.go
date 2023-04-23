package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/algorand/conduit/cmd/conduit/internal/initialize"
	"github.com/algorand/conduit/cmd/conduit/internal/list"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/loggers"
	"github.com/algorand/conduit/conduit/pipeline"
	_ "github.com/algorand/conduit/conduit/plugins/exporters/all"
	_ "github.com/algorand/conduit/conduit/plugins/importers/all"
	_ "github.com/algorand/conduit/conduit/plugins/processors/all"
	"github.com/algorand/conduit/version"
)

var (
	logger     *log.Logger
	conduitCmd = makeConduitCmd()
	//go:embed banner.txt
	banner string
)

const (
	conduitEnvVar = "CONDUIT_DATA_DIR"
)

// init() function for main package
func init() {
	conduitCmd.AddCommand(initialize.InitCommand)
	conduitCmd.AddCommand(list.Command)
}

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
		fmt.Print(banner)
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

// makeConduitCmd creates the main cobra command, initializes flags
func makeConduitCmd() *cobra.Command {
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

func main() {
	// Hidden command to generate docs in a given directory
	// conduit generate-docs [path]
	if len(os.Args) == 3 && os.Args[1] == "generate-docs" {
		err := doc.GenMarkdownTree(conduitCmd, os.Args[2])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := conduitCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
