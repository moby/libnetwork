Namespace Driver
=============

The namespace driver allows containers to use pre-defined network namespaces.
A namespace may be created with ip netns add namespace-name-here

## Configuration

The namespace driver can be configured with Docker's --net flag.

## Usage

The driver can invoked by running a docker container with --net=namespace:path/to/namespace
    e.g. docker run -it --net=namespace:/var/run/netns/alec docker.io/fedora /bin/bash
