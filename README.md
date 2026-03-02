# Aeron Kubernetes Bootstrap

This utility is designed to act as a startup shim between Aeron and K8s, to enable Aeron's native internal name resolution support, and use this in conjunction with K8s native service discovery.

https://github.com/aeron-io/aeron/wiki/Name-Resolution

The utility generates an Aeron configuration file, which must be added to configuration Aeron loads.

## Aeron name resolution

While static Aeron installations tend to use DNS to identify media-drivers, modern systems tend to be more dynamic than this, and services can change IP address frequently.
This leads to DNS being updated frequently, and DNS TTL/caching issues can get in the way.

Service Discovery mechanisms like [Zookeeper](https://cloud.spring.io/spring-cloud-zookeeper/1.2.x/multi/multi_spring-cloud-zookeeper-discovery.html), [Consul](https://developer.hashicorp.com/consul/docs/use-case/service-discovery) and [Kubernetes](https://kubernetes.io/docs/concepts/overview/kubernetes-api/) are now the standard way to discover services in most corporate environments.

Aeron has the concept of distributing it's own name/IP lookup information via gossip protocol. It only needs a few things to work.

1. To be enabled
2. To be given a list of initial bootstrap peers to gossip with.
3. To be given an interface to bind to.

## What does this code do?

It's designed to be run as an initContainer before your Aeron Media-Driver starts.
By default, it...

- connects to the K8s cluster API
- looks up it's own namespace
- finds every pod with an IP address that has the K8s label `aeron.io/media-driver=true`
- returns Pods, in order of oldest to youngest
- selects the IP from the Pod's `network-status` annotation, if available. Otherwise, selects the Pod IP.
- generates a bootstrap hosts list for Aeron media driver gossip of these IPs
- generates a local media driver name in the format `<pod-name>.<namespace>.aeron`

All this configuration is written to a java properties file (default `/etc/aeron/bootstrap.properties`), _which your media-driver process needs to load_.

## Assumptions made

Your media driver code expects to load `/etc/aeron/bootstrap.properties` ( path configurable ) as part of it's startup, to use the generated configuration.

## Configuration

**Environment Variables**:

- `AERON_MD_LABEL_SELECTOR`: Label selector for finding media driver pods (default: "aeron.io/media-driver=true")
- `AERON_MD_DISCOVERY_PORT`: Discovery port for Aeron (default: 8050)
- `AERON_MD_BOOTSTRAP_PATH`: Full path to create bootstrap properties file (default: "/etc/aeron/bootstrap.properties")
- `AERON_MD_MAX_BOOTSTRAP_PODS`: Maximum number of pods to include in bootstrap (default: 0 = unlimited)
- `AERON_MD_NAMESPACE`: Kubernetes namespace to scan (default: auto-discover from service account)
- `AERON_MD_HOSTNAME_SUFFIX`: Suffix for Aeron resolver hostname (default: ".aeron")
- `AERON_MD_SECONDARY_INTERFACE_NAME`: Name of secondary network interface to bind to (default: "net1"). Takes precedence over `AERON_MD_SECONDARY_INTERFACE_NETWORK_NAME`.
- `AERON_MD_SECONDARY_INTERFACE_NETWORK_NAME`: Name of secondary network to bind to (default: "aeron-network")
- `HOSTNAME`: Pod hostname (used as the interface to bind to)

## Building the containers

```
./build_containers.sh
```

## Build & Test

```
go test -v
go build
```

## Using the container in K8s

```
kubectl apply -f examples/simple.yml
```

## Things to be aware of.

IPs in frequently updated Kubernetes environments may get re-used. The IP that hosts a Aeron cluster node in a CI environment one moment may be re-used in an unrelated Aeron install the next minute.
This code generates Aeron hostnames in the format `<pod-name>.<namespace>.aeron` by default, to make sure you're talking to the media-driver you think you are.

## Aeron project implementation, low latency performance tuning and consulting

Also available, Aeron Operator for K8s.

Contact me via https://www.jmips.co.uk/#contact

Copyright © JMIPS Ltd 2025
