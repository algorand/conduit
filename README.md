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

Conduit is configured with a file named `conduit.yml`. Generate a config file template with `./conduit init > conduit.yml`. Place the `conduit.yml` file in a new directory, conventionally it is named `data` and will be referred to later on as "the data directory".

<!-- TODO: select plugins with init options? -->
You will need to manually edit the config file before running Conduit.

Here is an example configuration:

```yaml
# optional: level to use for logging.
log-level: "INFO, WARN, ERROR"

# optional: path to log file
log-file: "<path>"

# optional: maintain a pidfile for the life of the conduit process.
pid-filepath: "path to pid file."

# Define one importer.
importer:
    name:
    config:

# Define one or more processors.
processors:
  - name:
    config:
  - name:
    config:

# Define one exporter.
exporter:
    name:
    config:
```






















A Conduit pipeline is composed of 3 components, [Importers](./conduit/plugins/importers/), [Processors](./conduit/plugins/processors/), and [Exporters](./conduit/plugins/exporters/).
Every pipeline must define exactly 1 Importer, exactly 1 Exporter, and can optionally define a series of 0 or more Processors.





Once you have a valid config file in a directory, `config_directory`, launch conduit with `./conduit -d config_directory`.


### Plugins


Conduit comes with an initial set of plugins available for use in pipelines. For more information on the possible
plugins and how to include them in your pipeline's configuration file see [Configuration.md](Configuration.md).

There are also third party plugins available on the [Third Party Plugins](./docs/Third_Party_Plugins.md) page.


# Develoment

See the [Development](./docs/Development.md) page for building a plugin.









<!-- from here down is good -->

# Contributing

Contributions are welcome! Please refer to our [CONTRIBUTING](https://github.com/algorand/go-algorand/blob/master/CONTRIBUTING.md) document for general contribution guidelines, and individual plugin documentation for contributing to new and existing Conduit plugins.

# Migrating from Indexer 2.x

Conduit can be used to populate data from an existing Indexer 2.x deployment as part of upgrading to Indexer 3.x.

We will continue to maintain Indexer 2.x for the time being, but encourage users to move to Conduit. It provides cost benefits for most deployments in addition to greater flexibility.

[See the migration guide for details.](./docs/tutorials/IndexerMigration.md).
