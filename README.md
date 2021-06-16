[comment]: #( Copyright Contributors to the Open Cluster Management project )

# Leaf-Hub-Status-Sync
Red Hat Advanced Cluster Management Leaf Hub Status Sync

## How it works

## Build to run locally

```
make build
```

## Run Locally

Disable the currently running controller in the cluster (if previously deployed):

```
kubectl scale deployment leaf-hub-status-sync -n open-cluster-management --replicas 0
```

Set the following environment variables:

* PERIODIC_SYNC_INTERVAL
* WATCH_NAMESPACE

1.  `PERIODIC_SYNC_INTERVAL` - represents the time intervals that the controller will update the transport layer. 
    in between intervals, it will aggregate changes.
 
1.  `WATCH_NAMESPACE` - can be defined empty so the controller will watch all the namespaces.

```
./build/_output/bin/leaf-hub-status-sync
```

## Build image

Define the `REGISTRY` environment variable.

```
make build-images
```

## Deploy to a cluster

1.  Deploy the operator:

    ```
    IMAGE_TAG=latest envsubst < deploy/operator.yaml.template | kubectl apply -n open-cluster-management -f -
    ```
