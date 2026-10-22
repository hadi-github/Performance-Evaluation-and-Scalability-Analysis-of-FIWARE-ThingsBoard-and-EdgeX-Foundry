#!/bin/sh
# sidecar (valkey image): INFO server (server_time_usec) + commandstats every 10 s
while :; do echo "##INFO"; valkey-cli -h "${VALKEY_HOST:-valkey}" INFO server | grep server_time_usec
  valkey-cli -h "${VALKEY_HOST:-valkey}" INFO commandstats; sleep 10; done
