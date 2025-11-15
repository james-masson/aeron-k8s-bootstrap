#!/bin/bash

set -eo pipefail

function f_check (){

    OUTPUT="$(kubectl exec -it example-aeron-k8s-bootstrap-2 -c media-driver --   AeronStat -w false | grep "Resolver neighbors" | awk '{print $3}')"

    # We expect two neighbours
    if [[ ${OUTPUT} != "2" ]]
    then
        return 1
    fi

    return 0
}

# Retry logic: try up to 5 times with 2 second backoff
MAX_ATTEMPTS=5
BACKOFF_SECONDS=2

for attempt in $(seq 1 $MAX_ATTEMPTS); do
    echo "Attempt $attempt of $MAX_ATTEMPTS..."

    if f_check; then
        echo "** Bootstrap successful **"
        exit 0
    fi

    if [[ $attempt -lt $MAX_ATTEMPTS ]]; then
        echo "Check failed, retrying in ${BACKOFF_SECONDS} seconds..."
        sleep $BACKOFF_SECONDS
    fi
done

# All attempts failed
echo "*** Bootstrap failure after $MAX_ATTEMPTS attempts ***"
kubectl logs example-aeron-k8s-bootstrap-2 -c media-driver
kubectl logs example-aeron-k8s-bootstrap-2 -c aeron-k8s-bootstrap
kubectl exec -it example-aeron-k8s-bootstrap-2 -c media-driver --   AeronStat -w false
exit 1
