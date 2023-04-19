#!/usr/bin/env bash
set -e

# To allow mounting the data directory we need to change permissions
# to our algorand user. The script is initially run as the root user
# in order to change permissions, afterwards the script is re-launched
# as the algorand user.
if [ "$(id -u)" = '0' ]; then
  chown -R conduit:conduit $CONDUIT_DATA_DIR
  exec gosu conduit conduit "$@"
fi

exec "$@"
