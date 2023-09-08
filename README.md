<div style="text-align:center" align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/algorand_logo_mark_white.png">
    <source media="(prefers-color-scheme: light)" srcset="docs/assets/algorand_logo_mark_black.png">
    <img alt="Shows a black Algorand logo light mode and white in dark mode." src="docs/assets/algorand_logo_mark_black.png" width="200">
  </picture>

[![CircleCI](https://img.shields.io/circleci/build/github/algorand/conduit/master?label=master)](https://circleci.com/gh/algorand/conduit/tree/master)
![Github](https://img.shields.io/github/license/algorand/conduit)
[![Contribute](https://img.shields.io/badge/contributor-guide-blue?logo=github)](https://github.com/algorand/go-algorand/blob/master/CONTRIBUTING.md)
</div>

# Algorand Conduit

Conduit is a framework for ingesting blocks from the Algorand blockchain into external applications. It is designed as modular plugin system that allows users to configure their own data pipelines for filtering, aggregation, and storage of blockchain data.

<!-- TODO: a cool diagram here that clearly demonstrates data moving through the system -->

For example, use conduit to:
* Build a notification system for on chain events.
* Power a next generation block explorer.
* Select app specific data and write it to a custom database.
* Build a custom Indexer for a new [ARC](https://github.com/algorandfoundation/ARCs).
* Send blockchain data to another streaming data platform for additional processing (e.g. RabbitMQ, Kafka, ZeroMQ).
* Build an NFT catalog based on different standards.

# Getting Started

## Installation

### Download

The latest `conduit` binary can be downloaded from the [GitHub releases page](https://github.com/algorand/conduit/releases).

### Docker

[The latest docker image is on docker hub.](https://hub.docker.com/r/algorand/conduit)

### Install from Source

1. Checkout the repo, or download the source, `git clone https://github.com/algorand/conduit.git && cd conduit`
2. Run `make conduit`.
3. The binary is created at `cmd/conduit/conduit`.

## Usage

Conduit is configured with a YAML file named `conduit.yml`. This file defines the pipeline behavior by enabling and configuring different plugins.

### Create `conduit.yml` configuration file

Use the `conduit init` subcommand to create a configuration template. Place the configuration template in a new data directory. By convention the directory is named `data` and is referred to as the data directory.

```sh
mkdir data
./conduit init > data/conduit.yml
```

A Conduit pipeline is composed of 3 components, [Importers](./conduit/plugins/importers/), [Processors](./conduit/plugins/processors/), and [Exporters](./conduit/plugins/exporters/).
Every pipeline must define exactly 1 Importer, exactly 1 Exporter, and can optionally define a series of 0 or more Processors. See a full list of available plugins with `conduit list` or the [plugin documentation page](./conduit/plugins).

Here is an example `conduit.yml` that configures two plugins:

```yaml
importer:
    name: algod
    config:
        mode: "follower"
        netaddr: "http://your-follower-node:1234"
        token: "your API token"

# no processors defined for this configuration
processors:

exporter:
    name: file_writer
    config:
        # the default config writes block data to the data directory.
```

The `conduit init` command can also be used to select which plugins to include in the template. The example below uses the standard algod importer and sends the data to PostgreSQL. This example does not use any processor plugins.
```sh
./conduit init --importer algod --exporter postgresql > data/conduit.yml
```

Before running Conduit you need to review and modify `conduit.yml` according to your environment.

### Run Conduit

Once configured, start Conduit with your data directory as an argument:
```sh
./conduit -d data
```

### Full Tutorials

* [Writing Blocks to Files Using Conduit](./docs/tutorials/WritingBlocksToFile.md)
* [Using Conduit to Populate an Indexer Database](./docs/tutorials/IndexerWriter.md)

# External Plugins

Conduit supports external plugins which can be developed by anyone.

For a list of available plugins and instructions on how to use them, see the [External Plugins](./docs/ExternalPlugins.md) page.

## External Plugin Development

See the [Plugin Development](./docs/PluginDevelopment.md) page for building a plugin.

# Contributing

Contributions are welcome! Please refer to our [CONTRIBUTING](https://github.com/algorand/go-algorand/blob/master/CONTRIBUTING.md) document for general contribution guidelines.

# Migrating from Indexer 2.x

Conduit can be used to populate data from an existing [Indexer 2.x](https://github.com/algorand/indexer/) deployment as part of upgrading to Indexer 3.x. The v3 API is 100% backwards compatible with the v2 API.

We will continue to maintain Indexer 2.x up through November 1. From that point onward, subsequent consensus upgrades will only be compatible with Indexer 3.x when paired with Conduit.

To migrate, follow the [Using Conduit to Populate an Indexer Database](./docs/tutorials/IndexerWriter.md) tutorial. When you get to the step about setting up postgres, substitute your existing database connection string. Conduit will read the database to initialize the next round.
