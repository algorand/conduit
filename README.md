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

<!-- TODO: a section here that explains that you select plugins to configure behavior and how data goes through the system -->

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

### Create the configuration

The `conduit init` subcommand can be used to create a configuration template. It should be place in a new data directory. By convention the directory is named `data` and is referred to as the data directory.
```
mkdir data
./conduit init > data/conduit.yml
```

A Conduit pipeline is composed of 3 components, [Importers](./conduit/plugins/importers/), [Processors](./conduit/plugins/processors/), and [Exporters](./conduit/plugins/exporters/).
Every pipeline must define exactly 1 Importer, exactly 1 Exporter, and can optionally define a series of 0 or more Processors. A full list of available plugins with `conduit list` and the [plugin documentation page](TODO: plugin docs).

Here is an example that configures two plugins:
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
    name: "file_writer"
    config:
        # the default config writes block data to the data directory.
```

The `conduit init` command can also be used to select which plugins to include in the template. For example the following command creates a configuration template to populate an Indexer database.
```sh
docker run algorand/conduit init --importer algod --processors filter_processor --exporter postgresql > conduit.yml
```

The config file must be edited before running Conduit.

### Run Conduit

Once configured, start Conduit with your data directory:
```sh
./conduit -d data
```

# Third Party Plugins

<!-- TODO: "Third Party Plugins" or "External Plugins"? -->
Conduit supports Third Party Plugins, but not in the way you may be used to in other pluggable systems. In order to limit adding dependencies, third party plugins are enabled with a custom build that imports exactly the plugins you would like to deploy.

Over time this process can be automated, but for now it is manual and requires a go development environment and a little bit of code.

For a list of available plugins and instructions on how to use them, see the [Third Party Plugins](./docs/Third_Party_Plugins.md) page.

# Develoment

<!-- TODO: take the first section from docs/Development.md and put it here, similar to what was done with the above section -->
See the [Development](./docs/Development.md) page for building a plugin.

# Contributing

Contributions are welcome! Please refer to our [CONTRIBUTING](https://github.com/algorand/go-algorand/blob/master/CONTRIBUTING.md) document for general contribution guidelines, and individual plugin documentation for contributing to new and existing Conduit plugins.

# Migrating from Indexer 2.x

Conduit can be used to populate data from an existing Indexer 2.x deployment as part of upgrading to Indexer 3.x.

We will continue to maintain Indexer 2.x for the time being, but encourage users to move to Conduit. It provides cost benefits for most deployments in addition to greater flexibility.

[See the migration guide for details.](./docs/tutorials/IndexerMigration.md).
