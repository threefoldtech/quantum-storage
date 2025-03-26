## Introduction

In this section, we cover the planning and selection process of QSFS backends.

The backend storage we will consider here is an HDD-based zdb capacity provisioned on the ThreeFold Grid. While backends must always use zdb, it is possible to host these on other systems or storage media. That being said, such configurations will not be covered here.

## Values Overview

When data is pushed into the backends by zstor, it is done so according to a few configuration values. In a general sense, these values determine the amount of overhead that is used to provide redundancy and fault tolerance, but they can also affect performance.

Here's an overview of the most important values to consider when planning the backends for your QSFS deployment:

* **Minimal shards** - The minimum number of data shards needed to recreated a given data block.
* **Expected shards** - The total amount of shards which are generated when the data is encoded.
* **Disposable shards** - How many shards can be lost without losing any data. While this is not configured directly, it is derived from the previous two values:
  * `disposable shards = expected shards - minimal shards`
* **Groups** - backends can be grouped together into logical groups. Typically, each group would represent a physical site.
* **Redundant groups** - How many groups can be lost while keeping the data available.
* **Redundant nodes** - How many nodes can be lost in each group while keeping the data available in that group.
* **Backend size** - The size of each backend. In this guide, we will use gigabytes.


For planning purposes, it can be helpful to know how much data can be stored in a given configuration. Here are a few example calculations for a basic single group setup with minimal shards of 16 and expected shards of 20:

```
disposable shards = expected shards - minimal shards = 20 - 16 = 4

overhead = disposable shards / minimal shards = 4 / 16 = 25% = 0.25
```

Now let's say that we provision 20 backends with 1 GB of storage each:

```
total backend storage = 20 * 1 GB = 20 GB

overhead = 20 GB * 0.25 = 5 GB

usable capacity = 20 GB - 5 GB = 15 GB
```

When using redundant groups, there is additional overhead because each group must store at least the minimal shards. The exact figures depend on the configuration. In the case that each group holds the expected shards, then the total usable capacity is that of a single group (the smallest if the groups have different total storage sizes).

## Zstor Metadata

In addition to the data backends, zstor also requires exactly four metadata backends. This value is hard-coded for now, as is the quantity of disposable metadata shards, which is two. That means that two of four metadata backends can be lost while the operation of the system can continue normally. If three or all four of the metadata backends are unreachable, then no data can be retrieved by zstor. In that case, only any data cached in the frontend would remain available.

Therefore, to tolerate the failure of a site, the metadata nodes would need to be split among at least two sites. Metadata and data shards can be stored on the same nodes, but the zdbs should be provisioned separately in any case.

## Finding backend Nodes

Since we require HDD storage for backends, this will be the primary constraint when searching for host nodes. One way to do this is to use the [node finder](https://dashboard.grid.tf/#/deploy/node-finder) in the ThreeFold Dashboard and use a minimum HDD capacity filter.

Grouping nodes according to farm can be a strategy for ensuring the resulting system can tolerate site failures. We should note however that a given farm can contain nodes in different physical locations, and different farms can have their nodes in the same location. While the longitude and latitude data in the Dashboard is not precise, it's the best indication of whether nodes are in the same place or at least if they are connected to the same ISP regional hub.

## Backend Networking

With the release of ThreeFold Grid 3.14, zdbs provisioned through Zero OS have three networking options: IPv6, Yggdrasil (Planetary Network), and Mycelium. For this guide, we will stick to Mycelium because it is available on every node and thus simplifies the deployment process.

For more information about the tradeoffs involved in using IPv6 for zdb networking, see the [section below](#ipv6-as-a-zdb-networking-option).

## Additional backend Planning Considerations

To take advantage of the redundancy properties of QSFS, you will need to find at least as many backend nodes as your planned expected shards count. Placing multiple backends on the same node invalidates the calculations and reasoning about fault tolerance. Therefore, it is not recommended. Optionally, you can also find four additional nodes for metadata storage, if for some reason you don't want to mix data and metadata storage nodes.

It is also possible to provision more data backends than the expected shards amount. In this case, some nodes will not store any shards for each given data block. Data is still only guaranteed to be durable for the failure of nodes equal to the number of disposable shards, though in practice the availability of a given data block will depend only on the availability of the minimal shard count of nodes where data for that block is actually stored.

We will cover replacing backends in the final section of this guide. The reason for this could be to replace backends that have failed or to increase the storage amount. For now, it might be useful to note a few potential replacement nodes that can be swapped in later for any failed backends.

## IPv6 as a Zdb Networking Option

Using IPv6 for zdb connectivity could have some advantages. Because the data sent to backends is already quantum safe, there's no need to encrypt it again for transport as Mycelium does for all traffic. It's also possible that IPv6 provides a much more direct route, especially in the case that the frontend is in the same LAN as some or all of the backends. All Mycelium traffic must pass over a publicly reachable Mycelium node, and that could mean routing traffic over another physical location even when the machines communicating are side by side.

Using IPv6 has a few downsides for this use case. It is not every node that supports IPv6 at all, and among those that do, not all of them are publicly reachable. There are no simple ways to find which nodes or farms provide publicly reachable IPv6 addresses. It's also possible that IPv6 addresses are reassigned by the farmer's ISP. While some farmers certainly have stable IPv6 blocks, it is again not possible to find this information in any simple way.

Due to these difficulties, the details of how to provision and use backend zdbs with IPv6 isn't covered in this guide. It can be accomplished, however, with some small changes to the process shown.
