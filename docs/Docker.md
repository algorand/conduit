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
docker run -it -v $(pwd)/conduit.yml:/data/conduit.yml algorand/conduit
```

# Mounting the Data Directory

The data directory is located at `/algod/data`. Mounting a volume at that location will allow you to resume the deployment from another container.

## Volume Permissions

The container executes in the context of the `algorand` user with UID=999 and GID=999 which is handled differently depending on your operating system or deployment platform. During startup the container temporarily runs as root in order to modify the permissions of `/data`. It then changes to the `algorand` user. This can sometimes cause problems, for example if your deployment platform doesn't allow containers to run as the root user.

### Use specific UID and GID

On the host system, ensure the directory being mounted uses UID=999 and GID=999. If the directory already has these permissions you may override the default user with `-u 999:999`.
