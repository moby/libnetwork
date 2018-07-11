Design
======

The vision and goals of libnetwork are highlighted in [roadmap](../ROADMAP.md).
This document describes how libnetwork has been designed in order to achieve this.
Requirements for individual releases can be found on the [Project Page](https://github.com/docker/libnetwork/wiki).

Many of the design decisions are inspired by the learnings from the Docker networking design as of Docker v1.6.
Please refer to this [Docker v1.6 Design](legacy.md) document for more information on networking design as of Docker v1.6.

XXX: Docker v1.6 is very old. Perhaps this is no longer useful and can be removed now?

## Goal

The libnetwork project follows the Docker and Linux philosophy of developing small, highly modular and composable tools that work well independently.
libnetwork aims to satisfy the need for composable networking in containers.

## The Container Network Model

libnetwork implements the Container Network Model (CNM) which formalizes the steps required to provide networking for containers while providing an abstraction that can be used to support multiple network drivers. The CNM is built on 3 main components (shown below)

![](/docs/cnm-model.jpg?raw=true)

XXX: This diagram is really confusing when considering overlay networks (which are the default).
There, as I understand it, you have both a "container sandbox" (which the diagram calls the "Network Sandbox") and a separate network sandbox (which connects the network to all of that network's containers on a single host).

**Sandbox**

A Sandbox contains the configuration of a container's network stack.
This includes management of the container's interfaces, routing table and DNS settings.
An implementation of a Sandbox could be a Linux Network Namespace, a FreeBSD Jail or other similar concept.
A Sandbox may contain *many* endpoints from *multiple* networks.
The sandbox is managed by libnetwork itself, not by any individual driver.

**Endpoint**

An Endpoint joins a Sandbox to a Network.
An implementation of an Endpoint could be a `veth` pair, an Open vSwitch internal port or similar.
An Endpoint can belong to *only one* network and to *only one* Sandbox.

**Network**

A Network is a group of Endpoints that are able to communicate with each-other directly.
An implementation of a Network could be a Linux bridge, a VLAN, etc.
Networks consist of *many* endpoints.

## Services

A Service (e.g. a SwarmKit service) is an abstract concept used for load balancing and discovery.
Each service has a number of replicas, each with its own Endpoint.
Each endpoint has its own IP address, and the service itself has another IP address, called its Virtual IP (VIP).
When an application connects to the service's VIP, libnetwork will load-balance this across the available endpoints.

## Go API

**NetworkController**

The `NetworkController` object provides the entry-point into libnetwork. It exposes simple APIs for the users (such as Docker Engine) to allocate and manage Networks. libnetwork supports multiple active drivers (both inbuilt and remote).
Each network has an assigned driver, and `NetworkController` will forward most operations directly to the network's driver.

**Driver**

`Driver` is not a user visible object, but drivers provide the actual implementation that makes networking work. 
`NetworkController` however provides an API to configure any specific driver with driver-specific options/labels that is transparent to libnetwork, but can be handled by the drivers directly.

Drivers can be both inbuilt (such as Bridge, Host, None & overlay) and remote (from plugin providers) to satisfy various usecases & deployment scenarios. A network driver owns its network and is responsible for managing it.

Note: libnetwork (not the driver) is responsible for the container sandbox, including DNS, service discovery and load balancing across service replicas.
Some drivers (such as the [overlay](overlay.md) driver) create additional network namespaces for their own use.

**Ipam**

An IPAM driver manages IP address spaces.
It can use used to reserve an IP pool (subnet) and to allocate IP addresses within that pool.
In both cases, the caller can specify the addresses manually or ask the driver to choose.
Note that, confusingly, an `Ipam` driver is not a sub-type of `Driver` in the Go API.

XXX: Multiple IPAM drivers can be registered. Presumably they must allocate from non-overlapping pools.

**Network**

`Network` object is an implementation of the `CNM : Network` as defined above.

`NetworkController` provides APIs to create and manage `Network` objects. Whenever a `Network` is created or updated, the corresponding `Driver` will be notified of the event.
libnetwork treats `Network` objects at an abstract level to provide connectivity between a group of end-points that belong to the same network
and isolate from the rest.

XXX: I don't understand what `and isolate from the rest` means here. What is being isolated from what? Networks? Endpoints?

The Driver performs the actual work of providing the required connectivity and isolation. The connectivity can be within the same host or across multiple-hosts. Hence `Network` has a global scope within a cluster.

XXX: I thought networks could be local or global?

**Endpoint**

`Endpoint` represents a Service Endpoint. It provides the connectivity for services exposed by a container in a network with other services provided by other containers in the network. `Network` object provides APIs to create and manage endpoints. An endpoint can be attached to only one network. `Endpoint` creation calls are made to the corresponding `Driver` which is responsible for allocating resources for the corresponding `Sandbox`. Since Endpoint represents a Service and not necessarily a particular container, `Endpoint` has a global scope within a cluster as well.

XXX: does an endpoint really represent a service? It seems that a service may have many endpoints (one per replica?).

**Sandbox**
`Sandbox` object represents container's network configuration such as ip-address, mac-address, routes, DNS entries.
A `Sandbox` object is created when the user requests to create an endpoint on a network.

XXX: In the code sample, the sandbox is created by `controller.NewSandbox`, which the user performs after `network.CreateEndpoint`.

The `Driver` that handles the `Network` is responsible to allocate the required network resources (such as ip-address) and pass the info called `SandboxInfo` back to libnetwork.

XXX: Is this true? This seems like the job of the IPAM driver rather than the network driver.

XXX: `SandboxInfo` doesn't appear anywhere else in this repository.

libnetwork will make use of OS specific constructs (example: netns for Linux) to populate the network configuration into the containers that is represented by the `Sandbox`. A `Sandbox` can have multiple endpoints attached to different networks. Since `Sandbox` is associated with a particular container in a given host, it has a local scope that represents the Host that the Container belong to.

**CNM Attributes**

***Options***
`Options` provides a generic and flexible mechanism to pass `Driver` specific configuration option from the user to the `Driver` directly. `Options` are just key-value pairs of data with `key` represented by a string and `value` represented by a generic object (such as golang `interface{}`). libnetwork will operate on the `Options` ONLY if the  `key` matches any of the well-known `Label` defined in the `net-labels` package. `Options` also encompasses `Labels` as explained below. `Options` are generally NOT end-user visible (in UI), while `Labels` are.

***Labels***
`Labels` are very similar to `Options` & in fact they are just a subset of `Options`. `Labels` are typically end-user visible and are represented in the UI explicitly using the `--labels` option. They are passed from the UI to the `Driver` so that `Driver` can make use of it and perform any `Driver` specific operation (such as a subnet to allocate IP-Addresses from in a Network).

XXX: How does the system know when an option is also a label?

## CNM Lifecycle

Consumers of the CNM, like Docker for example, interact through the CNM Objects and its APIs to network the containers that they manage.

XXX: Where do these calls happen? Manager vs worker?

1. `Drivers` register with `NetworkController`. Built-in drivers register inside of libnetwork, while remote Drivers register with libnetwork via the plugin mechanism. (*plugin-mechanism is WIP*). Each `driver` handles a particular `networkType`.

XXX: Are remote plugins still WIP?

2. `NetworkController` object is created using `libnetwork.New()` API to manage the allocation of Networks and optionally configure a `Driver` with driver specific `Options`.

3. `Network` is created using the controller's `NewNetwork()` API by providing a `name` and `networkType`. `networkType` parameter helps to choose a corresponding `Driver` and binds the created `Network` to that `Driver`. From this point, any operation on `Network` will be handled by that `Driver`.

4. `controller.NewNetwork()` also takes in an optional `options` parameter which carries Driver-specific options and `Labels`, which the driver can make use of for its purpose.

5. `network.CreateEndpoint()` can be called to create a new Endpoint in a given network. This API also accepts an optional `options` parameter which drivers can make use of. These options carry both well-known labels and driver-specific labels. The driver will in turn be called with `driver.CreateEndpoint` and it can choose to reserve IPv4/IPv6 addresses when an `Endpoint` is created in a `Network`.

XXX: Doesn't the IPAM driver handle address reservations?

The `Driver` will assign these addresses using the `InterfaceInfo` interface defined in the `driverapi` package. The IP/IPv6 are needed to complete the endpoint as service definition along with the ports the endpoint exposes since essentially a service endpoint is nothing but a network address and the port number that the application container is listening on.

6. `endpoint.Join()` can be used to attach a container to an `Endpoint`. The Join operation will create a `Sandbox` if it doesn't exist already for that container.

XXX: endpoint.go: `Join` returns an error if no sandbox is passed.

The Drivers can make use of the Sandbox Key to identify multiple endpoints attached to a same container. This API also accepts optional `options` parameter which drivers can make use of.
  * Though it is not a direct design issue of libnetwork, it is highly encouraged to have users like `Docker` to call the endpoint.Join() during Container's `Start()` lifecycle that is invoked *before* the container is made operational.
  * One of a FAQ on endpoint join() API is that, why do we need an API to create an Endpoint and another to join the endpoint.
    - The answer is based on the fact that Endpoint represents a Service which may or may not be backed by a Container. When an Endpoint is created, it will have its resources reserved so that any container can get attached to the endpoint later and get a consistent networking behaviour.

7. `endpoint.Leave()` can be invoked when a container is stopped. The `Driver` can clean up the states that it allocated during the `Join()` call. libnetwork will delete the `Sandbox` when the last referencing endpoint leaves the network. But libnetwork keeps hold of the IP addresses as long as the endpoint is still present and will be reused when the container(or any container) joins again. This ensures that the container's resources are reused when they are Stopped and Started again.

8. `endpoint.Delete()` is used to delete an endpoint from a network. This results in deleting an endpoint and cleaning up the cached `sandbox.Info`.

XXX: Isn't this handled by SwarmKit? What does it actually do in libnetwork?

9. `network.Delete()` is used to delete a network. libnetwork will not allow the delete to proceed if there are any existing endpoints attached to the Network.

XXX: It seems we have two different things here called `Network`:
- The specification for a network, as created by a user with e.g. `docker network create`, and
- An engine network, that exists on a worker node while that node has containers participating in the network.
Which are we talking about where?

## Example: using an overlay network

When some application container tries to connect to a backend database service named `db`:

1. The application gets the DNS server from `/etc/resolv.conf`. Docker sets this to `127.0.0.11`.
2. The application makes a query for the service name `db`. This request is forwarded to the Docker daemon.
3. The daemon returns the service's Virtual IP (VIP).
4. The application tries to connect to the VIP.
5. iptables and ipvs rules inside the container sandbox choose a task IP for a particular replica of the service and NAT the request to that address.
6. The request leaves the container sandbox by a virtual ethernet device, the other end of which is owned by the network driver.

In the case of an overlay network, the frame will arrive in another network namespace (created by the overlay driver).
iptables rules in this namespace will route the frame either to the target container's namespace (if on the same host), or
to the target's physical host using VXLAN, where another overlay namespace will route it to the correct container.

The mapping of service name (`db`) to Service VIP and from VIP to endpoint IP are handled by libnetwork (not the driver).
This information is discovered using a gossip protocol, where nodes in the cluster spread updates around in a peer-to-peer fashion.

The mapping of target IPs to physical host IPs is handled by the overlay driver, also using gossip.

XXX: Not sure about this.

## Implementation Details

### Networks & Endpoints

libnetwork's Network and Endpoint APIs are primarily for managing the corresponding Objects and book-keeping them to provide a level of abstraction as required by the CNM. It delegates the actual implementation to the drivers which realize the functionality as promised in the CNM. For more information on these details, please see [the drivers section](#drivers)

### Sandbox

libnetwork provides a framework to implement a Sandbox in multiple operating systems. Currently we have implemented Sandbox for Linux using `namespace_linux.go` and `configure_linux.go` in `sandbox` package.
This creates a Network Namespace for each sandbox which is uniquely identified by a path on the host filesystem.
Netlink calls are used to move interfaces from the global namespace to the Sandbox namespace.
Netlink is also used to manage the routing table in the namespace.

## Drivers

## API

Drivers are essentially an extension of libnetwork and provide the actual implementation for all of the libnetwork APIs defined above. Hence there is an 1-1 correspondence for all the `Network` and `Endpoint` APIs, which includes :
* `driver.Config`
* `driver.CreateNetwork`
* `driver.DeleteNetwork`
* `driver.CreateEndpoint`
* `driver.DeleteEndpoint`
* `driver.Join`
* `driver.Leave`

These Driver facing APIs make use of unique identifiers (`networkid`,`endpointid`,...) instead of names (as seen in user-facing APIs).

### Driver semantics

 * `Driver.CreateEndpoint`

This method is passed an interface `EndpointInfo`, with methods `Interface` and `AddInterface`.

XXX: It appears to be `CreateEndpoint(nid, eid string, ifInfo InterfaceInfo, options map[string]interface{}) error`

If the value returned by `Interface` is non-nil, the driver is expected to make use of the interface information therein (e.g., treating the address or addresses as statically supplied), and must return an error if it cannot. If the value is `nil`, the driver should allocate exactly one _fresh_ interface, and use `AddInterface` to record them; or return an error if it cannot.

It is forbidden to use `AddInterface` if `Interface` is non-nil.

## Implementations

libnetwork includes the following driver packages:

- null
- bridge
- overlay
- remote

### Null

The null driver is a `noop` implementation of the driver API, used only in cases where no networking is desired. This is to provide backwards compatibility with Docker's `--net=none` option.

### Bridge

The `bridge` driver provides a Linux-specific bridging implementation based on the Linux Bridge.
For more details, please [see the Bridge Driver documentation](bridge.md).

### Overlay

The `overlay` driver implements networking that can span multiple hosts using overlay network encapsulations such as VXLAN.
For more details on its design, please see the [Overlay Driver Design](overlay.md).

### Remote

The `remote` package does not provide a driver, but provides a means of supporting drivers over a remote transport.
This allows a driver to be written in a language of your choice.
For further details, please see the [Remote Driver Design](remote.md).
