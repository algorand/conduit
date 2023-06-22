# Algod Import Plugin

This plugin imports block data from an algod node. Fetch blocks data from the [algod REST API](https://developer.algorand.org/docs/rest-apis/algod/v2/).

## Features

### Automatic Fast Catchup

If an admin API token is set, the plugin will attempt to use a fast catchup when it would help reach the target round.
A specific catchpoint can be provided, otherwise one will be selected automatically by querying the catchpoint URLs
listed in the sample.

### Follower Node Orchestration

When configured to work with a follower node, this plugin fully automates the management of the follower node. The sync round will be configured according to the pipeline round, and will be advanced as data is importer.

When using a follower node, ledger state delta objects are provided to the processors and exporter. This data contains detailed state transition information which is necessary for some processors and exporters.

## Configuration
```yml @sample.yaml
name: algod
config:
    # The mode of operation, either "archival" or "follower".
    # * follower mode allows you to use a lightweight non-archival node as the
    #   data source. In addition, it will provide ledger state delta objects to
    #   the processors and exporter.
    # * archival mode allows you to start processing on any round but does not
    #   contain the ledger state delta objects required for the postgres writer.
    mode: "follower"
  
    # Algod API address.
    netaddr: "http://url:port"
  
    # Algod API token.
    token: ""
  
    # Algod catchpoint catchup arguments
    catchup-config:
        # Algod Admin API Token. Used for running fast catchup during startup
        # if the node needs to be initialized.
        admin-token: ""
```
