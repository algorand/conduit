# Writing Blocks to Files Using Conduit

This guide will take you step by step through a specific application of some
Conduit plugins. We will detail each of the steps necessary to solve our problem, and point out documentation and tools
useful for both building and debugging conduit pipelines.

## Our Problem Statement

For this example, our task is to ingest blocks from an Algorand network (we'll use Testnet for this),
and write blocks to files.

Additionally, we don't care to see data about transactions which aren't sent to us, so we'll filter out all transactions
which are not sending either algos or some other asset to our account.

## Getting Started

First we need to make sure we have Conduit installed. Head over to the installation section of [the README](../../README.md) for more details. For this tutorial we'll assume `conduit` is installed and available on the path.

For our conduit pipeline we're going to use the `algod` importer, a `filter_processor`, and of course the
`file_writer` exporter.
To get more details about each of these individually, and their configuration variables,
use the list command or check the [plugins README](../../conduit/plugins/). For example:

```bash
conduit list exporters file_writer
```

## Setting Up Our Pipeline

Let's start assembling a configuration file which describes our conduit pipeline. For that we'll use the `conduit init` command to create the `conduit.yml` template.

```bash
mkdir data
conduit init --importer algod --processor filter_processor --exporter ... > data/conduit.yml data
```

This will create a configuration directory and write the `conduit.yml` config file
template.

## Setting up our Importer

Now we will fill in the algod importer configuration. I've got a local instance of algod running at `127.0.0.1:8080`,
with an API token of `e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302` and an admin API token of `2d9ceaf47debb8aff77c62475b8337f6ecef75c2aa8aa02170a8d38b9126b75a`. Find these in the `algod.token` and `algod.admin.token` files from your algod data directory. If you need help setting up
algod, you can take a look at the [go-algorand docs](https://github.com/algorand/go-algorand#getting-started) or our
[developer portal](https://developer.algorand.org/).

There are other options for the algod importer which are not being used for this tutorial. Those can be left or deleted as I've done here in the completed importer config:

```yaml
importer:
    name: algod
    config:
      netaddr: "http://127.0.0.1:8080"
      token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"
      catchup-config:
        admin-token: "2d9ceaf47debb8aff77c62475b8337f6ecef75c2aa8aa02170a8d38b9126b75a"
```

## Setting up our Processor

The processor section in our generated config has the filter_processor template, so we'll need to fill that in.

The default configuration looks something like this:

```yaml
name: filter_processor
config:
  # Filters is a list of boolean expressions that can search the payset transactions.
  # See README for more info.
  filters:
    - any:
      - tag: "txn.rcv"
        expression-type: "equal"
        expression: "ADDRESS"
```

The filter processor uses the tag of a property and allows us to specify an exact value to match or a regex.
For our use case we'll grab the address of a wallet I've created on testnet, `NVCAFYNKJL2NGAIZHWLIKI6HGMTLYXL7BXPBO7NXX4A7GMMWKNFKFKDKP4`.

That should give us exactly what we want, a filter that only allows transaction through for which the receiver is my account. However, there is a lot more you can do with the filter processor. To learn more about the possible uses, take a look at the filter plugin documentation.

## Setting up our Exporter

For the exporter the setup is simple. No configuration is necessary because it defaults to a directory inside the
conduit data directory. In this example I've chosen to override the default and set the directory output of my blocks
to a temporary directory, `block-dir: "/tmp/conduit-blocks/"`.

## Running the pipeline

Now we should have a fully valid config, so let's try it out. Here's the full config I ended up with. Comments and optional parameters were removed:

```yaml
log-level: "INFO"
importer:
    name: algod
    config:
      netaddr: "http://127.0.0.1:8080"
      token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"

processors:
  - name: filter_processor
    config:
      filters:
        - any:
          - tag: txn.rcv
            expression-type: equal
            expression: "NVCAFYNKJL2NGAIZHWLIKI6HGMTLYXL7BXPBO7NXX4A7GMMWKNFKFKDKP4"

exporter:
    name: file_writer
    config:
      block-dir: "/tmp/conduit-blocks/"
```

There are two things to address before our example becomes useful.

1. We need to get a payment transaction to our account.

For me, it's easiest to use the testnet dispenser, so I've done that. You can look at my transaction for yourself,
block #26141781 on testnet.

2. Skip rounds

To avoid having to run algod all the way from genesis to the most recent round, we can use catchpoint catchup to
fast-forward to a more recent block. Similarly, we want to be able to run Conduit pipelines from whichever round is
most relevant and useful for us.
To run conduit from a round other than 0, use the `--next-round-override` or `-r` flag. Because we set the admin token for our node, Conduit is able to run fast-catchup for us.

Now let's run the command!

```bash
> conduit -d /tmp/conduit-tmp/ --next-round-override 26141781
```

Once we've processed round 26141781, we should see our transaction show up!

```bash
> cat /tmp/conduit-blocks/* | grep payset -A 14
  "payset": [
    {
      "hgi": true,
      "sig": "DI4oMkUT01LAs5XT55qcZ3VCY8Wn2WrAZpntzFu2bTz9xnzaObmp5TOTUF5/PVVFCn14hXKyF3/LTZTUJylaDw==",
      "txn": {
        "amt": 10000000,
        "fee": 1000,
        "fv": 26141780,
        "lv": 26142780,
        "rcv": "NVCAFYNKJL2NGAIZHWLIKI6HGMTLYXL7BXPBO7NXX4A7GMMWKNFKFKDKP4",
        "snd": "GD64YIY3TWGDMCNPP553DZPPR6LDUSFQOIJVFDPPXWEG3FVOJCCDBBHU5A",
        "type": "pay"
      }
    }
  ]
```

There are many other existing plugins and use cases for Conduit! Take a look through the documentation and don't
hesitate to open an issue if you have a question. If you want to get a deep dive into the different types of filters
you can construct using the filter processor, take a look at our filter plugin.
