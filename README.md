[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Leaf-Hub-Status-Sync

[![Go Report Card](https://goreportcard.com/badge/github.com/open-cluster-management/leaf-hub-status-sync)](https://goreportcard.com/report/github.com/open-cluster-management/leaf-hub-status-sync)

The leaf hub status sync component of [Hub-of-Hubs](https://github.com/open-cluster-management/hub-of-hubs).

## How it works

## Build and push the image to docker registry

1.  Set the `REGISTRY` environment variable to hold the name of your docker registry:
    ```
    $ export REGISTRY=...
    ```
    
1.  Set the `IMAGE_TAG` environment variable to hold the required version of the image.  
    default value is `latest`, so in that case no need to specify this variable:
    ```
    $ export IMAGE_TAG=latest
    ```
    
1.  Run make to build and push the image:
    ```
    $ make push-images
    ```

## Deploy on a leaf hub

1.  Set the `REGISTRY` environment variable to hold the name of your docker registry:
    ```
    $ export REGISTRY=...
    ```
    
1.  Set the `IMAGE` environment variable to hold the name of the image.

    ```
    $ export IMAGE=$REGISTRY/$(basename $(pwd)):latest
    ```

1.  Set the `SYNC_SERVICE_PORT` environment variable to hold the ESS port as was setup in the leaf hub.
    ```
    $ export SYNC_SERVICE_PORT=...
    ```
    
1.  Set the `LH_ID` environment variable to hold the leaf hub unique id.
    ```
    $ export LH_ID=...
    ```

1.  Set the `NUMBER_OF_SIMULATED_LEAF_HUBS` environment variable to hold the number of simulated leaf hubs (not including the original leaf hub).
    ```
    $ export NUMBER_OF_SIMULATED_LEAF_HUBS=...
    ```
    
1.  Run the following command to deploy the `leaf-hub-status-sync` to your leaf hub cluster:  
    ```
    envsubst < deploy/leaf-hub-status-sync.yaml.template | kubectl apply -f -
    ```
    
## Cleanup from a leaf hub
    
1.  Run the following command to clean `leaf-hub-status-sync` from your leaf hub cluster:  
    ```
    envsubst < deploy/leaf-hub-status-sync.yaml.template | kubectl delete -f -
    ```
