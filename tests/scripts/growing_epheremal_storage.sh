#!/bin/bash

while true; do
  dd if=/dev/zero of="grow_storage$(( RANDOM*1000^999)).txt" bs=1KB count=1
  sleep 1
done
