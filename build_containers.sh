#!/bin/bash

set -eo pipefail

docker build -t jmips/aeron-k8s-bootstrap .
docker build -t jmips/aeronmd -f Dockerfile-aeronmd .