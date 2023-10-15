#!/bin/bash

count=0
while [ $count -lt 1000 ]; do
  dd if=/dev/zero of="grow_storage$(( RANDOM*1000^999)).txt" bs=1KB count=1
  count=$((count + 1))
done

ls grow_storage* | xargs -I % sh -c '{ rm %; sleep 1; }'

echo "Finished Shrinking file. Restart Pod"

sleep infinity