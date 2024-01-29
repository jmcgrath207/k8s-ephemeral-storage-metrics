#!/bin/bash

# Grow a storage path to 12MB each with 4Kb files to match ext4 the smallest possible block size.
# https://www.kernel.org/doc/html/latest/filesystems/ext4/overview.html#blocks
count=0
while [ $count -lt 1000 ]; do
  dd if=/dev/zero of="shrink_storage$((RANDOM * 1000 ^ 999)).txt" bs=4KB count=1
  dd if=/dev/zero of="/cache/shrink_storage$((RANDOM * 1000 ^ 999)).txt" bs=4KB count=1
  dd if=/dev/zero of="/cachez/shrink_storage$((RANDOM * 1000 ^ 999)).txt" bs=4KB count=1
  count=$((count + 1))
done

# Non Blocking
(
  ls /cache/shrink_storage* | xargs -I % sh -c '{ rm %; sleep 1; }' || true
) &
(
  ls /cachez/shrink_storage* | xargs -I % sh -c '{ rm %; sleep 1; }' || true
) &

# Blocking code
ls shrink_storage* | xargs -I % sh -c '{ rm %; sleep 1; }'

echo "Finished Shrinking file. Restart Pod"
