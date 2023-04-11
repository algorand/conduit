# Getting Started


## Installation

### Download

The latest `conduit` binary can be downloaded from the [GitHub releases page](https://github.com/algorand/conduit/releases).

### Install from Source

1. Checkout the repo, or download the source, `git clone https://github.com/algorand/conduit.git && cd conduit`
2. Run `make conduit`.
3. The binary is created at `cmd/conduit/conduit`.

## Getting Started

Conduit requires a configuration file to set up and run a data pipeline. To generate an initial skeleton for a conduit
config file, you can run `./conduit init -d data`. This will set up a sample data directory with a config located at
`data/conduit.yml`.

You will need to manually edit the data in the config file, filling in a valid configuration for conduit to run.
You can find a valid config file in [Configuration.md](Configuration.md) or via the `conduit init` command.

Once you have a valid config file in a directory, `config_directory`, launch conduit with `./conduit -d config_directory`.

# Configuration and Plugins
Conduit comes with an initial set of plugins available for use in pipelines. For more information on the possible
plugins and how to include these plugins in your pipeline's configuration file see [Configuration.md](Configuration.md).

# Tutorials

Detailed tutorials for specific topics are available.

## Migrate from Indexer to Conduit
[Information about converting from Indexer writer configurations to Conduit.](./tutorials/IndexerMigration.md)

## Setup Conduit for the Indexer API
[How to configure algod, PostgreSQL and Conduit as an Indexer API backend.](./tutorials/IndexerWriter.md)

## Transaction Filtering Plugin

[A deep dive into the transaction filter plugin.](./tutorials/FilterDeepDive.md)

## Writing block data to the filesystem
[Use the file exporter to write data to files.](./tutorials/WritingBlocksToFile.md)
