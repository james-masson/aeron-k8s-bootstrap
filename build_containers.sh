#!/bin/bash

set -eo pipefail

docker build -t jmips/aeron-k8s-bootstrap .
docker build -t jmips/aeronmd -f Dockerfile-aeronmd .

# If running in github actions, also save the containers to disk for later caching
if test -z "${GITHUB_JOB}"
then
    mkdir /tmp/container-cache
    docker save jmips/aeron-k8s-bootstrap -o /tmp/container-cache/aeron-k8s-bootstrap
    docker save jmips/aeronmd -o /tmp/container-cache/aeronmd
fi