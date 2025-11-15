#!/bin/bash

set -eo pipefail

OUTPUT="$(kubectl exec -it example-aeron-k8s-bootstrap-2 -c media-driver --   AeronStat -w false | grep "Resolver neighbors" | awk '{print $3}')"

# We expect two neighbours
if [[ ${OUTPUT} != "2" ]]
then
    echo "*** Bootstrap failure ***"
    kubectl logs example-aeron-k8s-bootstrap-2 -c media-driver
    kubectl logs example-aeron-k8s-bootstrap-2 -c aeron-k8s-bootstrap
    kubectl exec -it example-aeron-k8s-bootstrap-2 -c media-driver --   AeronStat -w false
    exit 1
fi

echo "** Bootstrap successful **"