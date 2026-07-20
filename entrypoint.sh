#!/bin/sh
set -e

: "${SOCKETS_DIR:=/var/run/cortex-governance/providers}"

# Infer PROVIDER_NAME from /etc/cortex-provider/provider.json if not explicitly provided
if [ -z "$PROVIDER_NAME" ]; then
  if [ -f "/etc/cortex-provider/provider.json" ]; then
    PROVIDER_NAME=$(grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' /etc/cortex-provider/provider.json | head -n1 | cut -d'"' -f4)
  fi
fi

if [ -n "$PROVIDER_NAME" ]; then
  mkdir -p "$SOCKETS_DIR/$PROVIDER_NAME"

  if [ -f "/etc/cortex-provider/provider.json" ]; then
    cp /etc/cortex-provider/provider.json "$SOCKETS_DIR/$PROVIDER_NAME/provider.json"
  elif [ -f "/etc/$PROVIDER_NAME-provider/provider.json" ]; then
    cp "/etc/$PROVIDER_NAME-provider/provider.json" "$SOCKETS_DIR/$PROVIDER_NAME/provider.json"
  fi

  if [ -z "$GOVERNANCE_PROVIDER_SOCKET" ]; then
    export GOVERNANCE_PROVIDER_SOCKET="$SOCKETS_DIR/$PROVIDER_NAME/provider.sock"
  fi
fi

exec "$@"
