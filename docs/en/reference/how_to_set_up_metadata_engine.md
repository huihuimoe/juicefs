---
title: How to Set Up Metadata Engine
sidebar_position: 2
slug: /databases_for_metadata
description: JuiceFS supports Redis-compatible databases and BadgerDB as metadata engines, and this article describes how to set up and use them.
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

:::tip
`META_PASSWORD` is supported from JuiceFS v1.0. You should [upgrade](../administration/upgrade.md) if you're still using older versions.
:::

JuiceFS is a decoupled structure that separates data and metadata. Metadata can be stored in any supported database (called Metadata Engine). Supported engines have different performance and intended scenarios, refer to [our docs](../benchmark/metadata_engines_benchmark.md) for comparison.

## The storage usage of metadata {#storage-usage}

The storage space required for metadata is related to the length of the file name, the type and length of the file, and extended attributes. It is difficult to accurately estimate the metadata storage space requirements of a file system. For simplicity, we can approximate based on the storage space required for a single small file without extended attributes.

- **Redis-compatible database**: 300 bytes/file
- **BadgerDB**: 300 bytes/file

When the average file is larger (over 64MB), or the file is frequently modified and has a lot of fragments, or there are many extended attributes, or the average file name is long (over 50 bytes), more storage space is needed.

## Redis-compatible database

### Redis

JuiceFS requires Redis version 4.0 and above. Redis Cluster is also supported, but in order to avoid transactions across different Redis instances, JuiceFS puts all metadata for one file system on a single Redis instance.

:::tip Redis Cluster Key Prefix
When using Redis Cluster, the database number in the URL is used as a **key prefix** rather than for actual database selection (since Redis Cluster only supports database 0). The prefix format is `{N}` (e.g., `{1}`, `{2}`), which uses Redis hash tags to ensure all keys for one volume are routed to the same slot. This allows multiple JuiceFS file systems to share a single Redis Cluster:

```shell
# Different volumes use different DB numbers as key prefixes
juicefs format redis://cluster:6379/1 volume1   # keys prefixed with {1}
juicefs format redis://cluster:6379/2 volume2   # keys prefixed with {2}
```

You can verify the keys in Redis Cluster using:

```shell
redis-cli -c -h <host> -p 6379 keys '{1}*'   # list all keys for volume with prefix {1}
```

:::

To ensure metadata security, JuiceFS requires [`maxmemory-policy noeviction`](https://redis.io/docs/reference/eviction/), otherwise it will try to set it to `noeviction` when starting JuiceFS, and will print a warning log if it fails. Refer to [Redis Best practices](../administration/metadata/redis_best_practices.md) for more.

#### Create a file system

When using Redis as the metadata storage engine, the following format is usually used to access the database:

<Tabs>
  <TabItem value="tcp" label="TCP">

```
redis[s]://[<username>:<password>@]<host>[:<port>]/<db>
```

  </TabItem>
  <TabItem value="unix-socket" label="Unix socket">

```
unix://[<username>:<password>@]<socket-file-path>?db=<db>
```

  </TabItem>
</Tabs>

Where `[]` enclosed are optional and the rest are mandatory.

- If the [TLS](https://redis.io/docs/manual/security/encryption) feature of Redis is enabled, the protocol header needs to use `rediss://`, otherwise use `redis://`.
- `<username>` is introduced after Redis 6.0 and can be ignored if there is no username, but the `:` colon in front of the password needs to be kept, e.g. `redis://:<password>@<host>:6379/1`.
- The default port number on which Redis listens is `6379`, which can be left blank if the default port number is not changed, e.g. `redis://:<password>@<host>/1`.
- Redis supports multiple [logical databases](https://redis.io/commands/select), please replace `<db>` with the actual database number used.
- If you need to connect to Redis Sentinel, the format will be slightly different, refer to [Redis Best Practices](../administration/metadata/redis_best_practices.md#high-availability) for details.
- If username / password contains special characters, use single quote to avoid unexpected shell interpretations, or use the `REDIS_PASSWORD` environment.

:::tip
A Redis instance can, by default, create a total of 16 logical databases, with each of these databases eligible for the creation of a singular JuiceFS file system. Thus, under ordinary circumstances, a single Redis instance may be utilized to form up to 16 JuiceFS file systems. However, it is crucial to note that the logical databases intended for use with JuiceFS must not be shared with other applications, as doing so could lead to data inconsistencies.
:::

For example, the following command will create a JuiceFS file system named `pics`, using the database No. `1` in Redis to store metadata:

```shell
juicefs format \
    --storage s3 \
    ... \
    "redis://:mypassword@192.168.1.6:6379/1" \
    pics
```

For security purposes, it is recommended to pass the password using the environment variable `META_PASSWORD` or `REDIS_PASSWORD`, e.g.

```shell
export META_PASSWORD=mypassword
```

Similarly, the password can be provided from a file using:

```shell
export META_PASSWORD_FILE=/secret/mypassword.txt
```

Then there is no need to set a password in the metadata URL.

```shell
juicefs format \
    --storage s3 \
    ... \
    "redis://192.168.1.6:6379/1" \
    pics
```

#### Mount a file system

If you need to share the same file system across multiple nodes, ensure that all nodes has access to the Metadata Engine.

```shell
juicefs mount -d "redis://:mypassword@192.168.1.6:6379/1" /mnt/jfs
```

Passing passwords with the `META_PASSWORD` or `REDIS_PASSWORD` environment variables is also supported.

```shell
export META_PASSWORD=mypassword
juicefs mount -d "redis://192.168.1.6:6379/1" /mnt/jfs
```

Similarly, the password can be provided from a file using as follows:

```shell
export META_PASSWORD_FILE=/secret/mypassword.txt
juicefs mount -d "redis://192.168.1.6:6379/1" /mnt/jfs
```

#### Set up TLS

JuiceFS supports both TLS server-side encryption authentication and mTLS mutual encryption authentication connections to Redis. When connecting to Redis via TLS or mTLS, use the `rediss://` protocol header. However, when using TLS server-side encryption authentication, it is not necessary to specify the client certificate and private key.

:::note
Using Redis mTLS requires JuiceFS version 1.1.0 and above
:::

If Redis server has enabled mTLS feature, it is necessary to provide client certificate, private key, and CA certificate that issued the client certificate to connect. In JuiceFS, mTLS can be used in the following way:

```shell
juicefs format --storage s3 \
    ... \
    "rediss://192.168.1.6:6379/1?tls-cert-file=/etc/certs/client.crt&tls-key-file=/etc/certs/client.key&tls-ca-cert-file=/etc/certs/ca.crt"
    pics
```

In the code mentioned above, we use the `rediss://` protocol header to enable mTLS functionality, and then use the following options to specify the path of the client certificate:

- `tls-cert-file=<path>`: The path of the client certificate.
- `tls-key-file=<path>`: The path of the private key.
- `tls-ca-cert-file=<path>`: The path of the CA certificate. It is optional. If it is not specified, the system CA certificate will be used.
- `insecure-skip-verify=true` It can skip verifying the server certificate.

When specifying options in a URL, start with the `?` symbol and use the `&` symbol to separate multiple options, for example: `?tls-cert-file=client.crt&tls-key-file=client.key`.

In the above example, `/etc/certs` is just a directory name. Replace it with your actual certificate directory when using it, which can be a relative or absolute path.

### Valkey

[Valkey](https://valkey.io) is an open-source fork of Redis, created to preserve the project's community-driven governance while remaining highly compatible with the Redis ecosystem. Valkey focuses on maintaining stability, improving performance, and continuing innovation under a neutral approach, ensuring long-term availability for users who rely on Redis-compatible workloads.

When being used as the metadata storage engine for JuiceFS, Valkey functions the same way as Redis. Please refer to Valkey's [documentation](https://valkey.io/topics/installation) for installation and other aspects, as well as the [Redis](#redis) section for usage.

### KeyDB

[KeyDB](https://keydb.dev) is an open-source fork of Redis, developed to stay aligned with the Redis community. KeyDB implements multi-threading support, better memory utilization, and greater throughput on top of Redis and also supports [Active Replication](https://docs.keydb.dev/docs/active-rep) (also known as Active-Active). KeyDB is considered compatible with Redis version 6, but it is [not actively maintained by its community](https://github.com/Snapchat/KeyDB/issues/895) at the moment.

:::note
The Active Replication feature is asynchronous and may cause consistency issues, so use with caution!
:::

When being used as the metadata storage engine for JuiceFS, KeyDB functions the same way as Redis. Please refer to KeyDB's [documentation](https://docs.keydb.dev/docs) for installation and other aspects, as well as the [Redis](#redis) section for usage.

## Key-value database

### BadgerDB

[BadgerDB](https://github.com/dgraph-io/badger) is an embedded, persistent, and standalone Key-Value database developed in pure Go. The database files are stored locally in the specified directory.

When using BadgerDB as the JuiceFS metadata storage engine, use `badger://` to specify the database path.

#### Create a file system

You only need to create a file system for use, and there is no need to create a BadgerDB database in advance.

```shell
juicefs format badger://$HOME/badger-data myjfs
```

This command creates `badger-data` as a database directory in the `home` directory of the current user, which is used as metadata storage for JuiceFS.

#### Mount a file system

The database path needs to be specified when mounting the file system.

```shell
juicefs mount -d badger://$HOME/badger-data /mnt/jfs
```

:::tip
BadgerDB only allows single-process access. If you need to perform operations like `gc`, `fsck`, `dump`, and `load`, you need to unmount the file system first.
:::
