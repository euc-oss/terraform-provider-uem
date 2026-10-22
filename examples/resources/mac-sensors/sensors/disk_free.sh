#!/bin/bash
# Reports available disk space on the boot volume.
df -h / | awk 'NR==2 { print $4 }'
