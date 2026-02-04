#!/bin/bash

set -eo pipefail

function f_check (){

    ID=$1
    NEIGHBOURS="$(kubectl -n ${NAMESPACE} exec -i example-aeron-k8s-bootstrap-${ID} -c media-driver --   AeronStat -w false | grep "Resolver neighbors")"
    OUTPUT="$(echo ${NEIGHBOURS} | awk '{print $3}')"
    echo "Node ${ID}: ${NEIGHBOURS}"

    # We expect two neighbours
    if [[ ${OUTPUT} != "2" ]]
    then
        return 1
    fi

    return 0
}

function f_checkall (){
    for X in 0 1 2
    do
        if f_check $X
        then true
        else return 1
        fi
    done
}

function f_debug() {
    ID="$1"
    echo "***************** POD ${ID} DEBUG ***************************"
    kubectl -n ${NAMESPACE} logs example-aeron-k8s-bootstrap-${ID} -c media-driver
    kubectl -n ${NAMESPACE} logs example-aeron-k8s-bootstrap-${ID} -c aeron-k8s-bootstrap
    kubectl -n ${NAMESPACE} exec -it example-aeron-k8s-bootstrap-${ID} -c media-driver --   AeronStat -w false
    kubectl -n ${NAMESPACE} describe pod example-aeron-k8s-bootstrap-${ID}
}

# Retry logic: try up to 5 times with 2 second backoff
MAX_ATTEMPTS=5
BACKOFF_SECONDS=5
NAMESPACE="${NAMESPACE:-default}"

for attempt in $(seq 1 $MAX_ATTEMPTS); do
    echo "Attempt $attempt of $MAX_ATTEMPTS..."

    if f_checkall; then
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
f_debug 0
f_debug 1
f_debug 2
exit 1
