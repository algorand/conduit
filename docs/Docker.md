**This container is a work in progress and not yet deployed to docker hub.**

# Docker Image

Algorand's Conduit data pipeline packaged for docker.

This document provides some basic guidance and commands tailored
for the docker container. For additional information refer to
the [full documentation](https://developer.algorand.org/docs/get-details/conduit/GettingStarted/).

# Usage

Conduit needs a configuration file to define the data pipeline.
There are built-in utilities to help create the configuration.
Once a configuration is made launch conduit and pass in the configuration
to use.

## Creating a conduit.yml configuration

The init subcommand can be used to create a configuration template.
See the options here:
```
docker run algorand/conduit init -h
```

For a simple default configuration, run with no arguments:
```
docker run algorand/conduit init > conduit.yml
```

Plugins can also be named directly.

See a list of available plugins:
```
docker run algorand/conduit list
```

Provide plugins to the init command:
```
docker run algorand/conduit init --importer algod --processors filter_processor --exporter postgresql > conduit.yml
```

## Run with conduit.yml

With `conduit.yml` in your current working directory,
launch the container:
```
docker run -it -v $(pwd)/conduit.yml:/conduit/data/conduit.yml algorand/conduit
```
