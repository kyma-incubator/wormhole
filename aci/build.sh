#!/usr/bin/env bash
set -e

if [ "$EUID" -ne 0 ]; then
    echo "This script uses functionality which requires root privileges"
    exit 1
fi

# Start the build with an empty ACI
acbuild --debug begin

# In the event of the script exiting, end the build
trap "{ export EXT=$?; acbuild --debug end && exit $EXT; }" EXIT

# Name the ACI
acbuild --debug set-name kinvolk.io/wormhole-connector

# Based on alpine
acbuild --debug dep add quay.io/coreos/alpine-sh

# copy wormhole-connector
acbuild --debug copy ../wormhole-connector /wormhole-connector

acbuild --debug copy entrypoint.sh /entrypoint.sh
# Run wormhole connector
acbuild --debug set-exec /entrypoint.sh

# Add keys mountoint
acbuild --debug mount add key /connector-key.pem
acbuild --debug mount add cert /connector.pem

# Set user and group
acbuild --debug set-user 1000
acbuild --debug set-group 1000

# Add server port
acbuild --debug port add server tcp 8080

# Add serf port
acbuild --debug port add serf tcp 1111

# Add raft port
acbuild --debug port add raft tcp 1112

# Write the result
acbuild --debug write --overwrite wormhole-connector-linux-amd64.aci
