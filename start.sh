#!/bin/bash

# Typical machine setup: 4G/virtual core, 16 * 4 = 64 containers / 64G RAM
# Memory limit per process: 2G
# Memory overcommit ratio: 2
cpus=$(cat /proc/cpuinfo |grep processor|wc -l)
cpus=$(( cpus * 16 ))

# Always fetch latest image
docker pull tb0hdan/idun

# Start 
docker-compose up -d --scale worker=${cpus}
