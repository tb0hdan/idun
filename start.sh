#!/bin/bash

cpus=$(cat /proc/cpuinfo |grep processor|wc -l)
cpus=$(( cpus * 2 ))

docker-compose up -d --scale worker=${cpus}
