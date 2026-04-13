#!/bin/sh
set -eu

CONFIG_PATH="${CLI_PROXY_RUNTIME_CONFIG:-/CLIProxyAPI/data/config.yaml}"
CONFIG_DIR="$(dirname "$CONFIG_PATH")"

mkdir -p "$CONFIG_DIR" /CLIProxyAPI/logs /root/.cli-proxy-api

if [ ! -f "$CONFIG_PATH" ]; then
  cp /CLIProxyAPI/config.example.yaml "$CONFIG_PATH"
fi

exec /CLIProxyAPI/CLIProxyAPI -config "$CONFIG_PATH" "$@"
