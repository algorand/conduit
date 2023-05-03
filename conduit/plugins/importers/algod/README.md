# Algod Import Plugin

This plugin imports block data from an algod node. Fetch blocks data from the [algod REST API](https://developer.algorand.org/docs/rest-apis/algod/v2/).

## Features

### Automatic Fast Catchup

If an admin API token and Auto (or a catchpoint label) are set, the plugin will automatically run fast catchup on startup if the node is behind the current pipeline round. If a catchpoint label is set, that takes precedent over auto being set to true.

### Follower Node Orchestration

When configured to work with a follower node, this plugin fully automates the management of the follower node. The sync round will be configured according to the pipeline round, and will be advanced as data is importer.

When using a follower node, ledger state delta objects are provided to the processors and exporter. This data contains detailed state transition information which is necessary for some processors and exporters.

## Configuration
```yml @sample.yaml
  name: algod
  config:
    # The mode of operation, either "archival" or "follower".
    # * archival mode allows you to start processing on any round but does not
    #   contain the ledger state delta objects required for the postgres writer.
    # * follower mode allows you to use a lightweight non-archival node as the
    #   data source. In addition, it will provide ledger state delta objects to
    #   the processors and exporter.
    mode: "follower"

    # Algod API address.
    netaddr: "http://url:port"

    # Algod API token.
    token: ""

    # Algod catchpoint catchup arguments
    catchup-config:
        # Automatically download an appropriate catchpoint label. If false, you
        # must specify a catchpoint to use fast catchup.
        auto: false
        # The catchpoint to use when running fast catchup. If this is set it
        # overrides 'auto: true'. To select an appropriate catchpoint for your
        # deployment, see the list of available catchpoints for each network:
        #   mainnet: https://algorand-catchpoints.s3.us-east-2.amazonaws.com/consolidated/mainnet_catchpoints.txt
        #   betanet: https://algorand-catchpoints.s3.us-east-2.amazonaws.com/consolidated/betanet_catchpoints.txt
        #   testnet: https://algorand-catchpoints.s3.us-east-2.amazonaws.com/consolidated/testnet_catchpoints.txt
        catchpoint: ""
        # Algod Admin API Token
        admin-token: ""
```
