#!/bin/sh

WORMHOLE_BIND_ADDRESS='127.0.0.1'
if [ -n "$WORMHOLE_BIND_INTERFACE" ]; then
  WORMHOLE_BIND_ADDRESS=$(ip -o -4 addr list $WORMHOLE_BIND_INTERFACE | head -n1 | awk '{print $4}' | cut -d/ -f1)
  if [ -z "$WORMHOLE_BIND_ADDRESS" ]; then
    echo "Could not find IP for interface '$WORMHOLE_BIND_INTERFACE', exiting"
    exit 1
  fi

  WORMHOLE_BIND_ADDRESS="$WORMHOLE_BIND_ADDRESS"

  echo "==> Found address '$WORMHOLE_BIND_ADDRESS' for interface '$WORMHOLE_BIND_INTERFACE', setting bind file..."
fi

exec /wormhole-connector --local-addr=${WORMHOLE_BIND_ADDRESS} "$@"
