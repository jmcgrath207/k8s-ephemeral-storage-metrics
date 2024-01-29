#!/bin/bash

while true; do
  current_size=$(du /cache -s | grep -oP '\d+')
  if [[ "$current_size" -gt 12000 ]]; then
    # Max Size Reach. Kill Container
    exit 1
  fi
  dd if=/dev/zero of="grow_storage$((RANDOM * 1000 ^ 999)).txt" bs=4KB count=1
  dd if=/dev/zero of="/cache/grow_storage$((RANDOM * 1000 ^ 999)).txt" bs=4KB count=1
  dd if=/dev/zero of="/cachez/grow_storage$((RANDOM * 1000 ^ 999)).txt" bs=4KB count=1
  sleep 1
done
