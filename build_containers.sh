#!/bin/bash

set -eo pipefail

docker build -t ghcr.io/james-masson/aeron-k8s-bootstrap/aeron-k8s-bootstrap .
docker build -t ghcr.io/james-masson/aeron-k8s-bootstrap/aeronmd -f Dockerfile-aeronmd .
