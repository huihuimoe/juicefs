---
title: 如何设置元数据引擎
sidebar_position: 2
slug: /databases_for_metadata
description: JuiceFS 支持 Redis 兼容数据库和 BadgerDB 作为元数据引擎，本文分别介绍相应的设置和使用方法。
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

:::tip
`META_PASSWORD` 是 JuiceFS v1.0 新增功能，旧版客户端需要[升级](../administration/upgrade.md)后才能使用。
:::

JuiceFS 采用数据和元数据分离的存储架构，元数据可以存储在任意支持的数据库中，称为「元数据存储引擎」。不同元数据引擎的性能、易用性、场景均有区别，具体性能对比可参考[该文档](../benchmark/metadata_engines_benchmark.md)。

## 元数据存储用量 {#storage-usage}

元数据所需的存储空间跟文件名的长度、文件的类型和长度以及扩展属性等相关，无法准确地估计一个文件系统的元数据存空间需求。简单起见，我们可以根据没有扩展属性的单个小文件所需的存储空间来做近似：

- **Redis 兼容数据库**：300 字节／文件
- **BadgerDB**：300 字节／文件

当平均文件更大（超过 64MB），或者文件被频繁修改导致有很多碎片，或者有很多扩展属性，或者平均文件名很长（超过 50 字节），都会导致需要更多的存储空间。

## Redis 兼容数据库

### Redis

JuiceFS 要求使用 4.0 及以上版本的 Redis。JuiceFS 也支持使用 Redis Cluster 作为元数据引擎，但为了避免在 Redis 集群中执行跨节点事务，同一个文件系统的元数据总会坐落于单个 Redis 实例中。

:::tip Redis Cluster 键前缀
使用 Redis Cluster 时，URL 中的数据库编号会被用作**键前缀**，而不是用于实际的数据库选择（因为 Redis Cluster 仅支持数据库 0）。前缀格式为 `{N}`（例如 `{1}`、`{2}`），使用 Redis 哈希标签（hash tag）确保同一个卷的所有键都被路由到同一个槽（slot）。这使得多个 JuiceFS 文件系统可以共享同一个 Redis Cluster：

```shell
# 不同的卷使用不同的数据库编号作为键前缀
juicefs format redis://cluster:6379/1 volume1   # 键前缀为 {1}
juicefs format redis://cluster:6379/2 volume2   # 键前缀为 {2}
```

可以使用以下命令在 Redis Cluster 中验证键：

```shell
redis-cli -c -h <host> -p 6379 keys '{1}*'   # 列出前缀为 {1} 的所有键
```

:::

为了保证元数据安全，JuiceFS 需要 [`maxmemory-policy noeviction`](https://redis.io/docs/reference/eviction/)，否则在启动 JuiceFS 的时候将会尝试将其设置为 `noeviction`，如果设置失败将会打印告警日志。更多可以参考 [Redis 最佳实践](../administration/metadata/redis_best_practices.md)。

#### 创建文件系统

使用 Redis 作为元数据存储引擎时，通常使用以下格式访问数据库：

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

其中，`[]` 括起来的是可选项，其它部分为必选项。

- 如果开启了 Redis 的 [TLS](https://redis.io/docs/manual/security/encryption) 特性，协议头需要使用 `rediss://`，否则使用 `redis://`。
- `<username>` 是 Redis 6.0 之后引入的，如果没有用户名可以忽略，但密码前面的 `:` 冒号需要保留，如 `redis://:<password>@<host>:6379/1`。
- Redis 监听的默认端口号为 `6379`，如果没有改变默认端口号可以不用填写，如 `redis://:<password>@<host>/1`，否则需要显式指定端口号。
- Redis 支持多个[逻辑数据库](https://redis.io/commands/select)，请将 `<db>` 替换为实际使用的数据库编号。
- 如果需要连接 Redis 哨兵（Sentinel），元数据 URL 的格式会稍有不同，具体请参考[「Redis 最佳实践」](../administration/metadata/redis_best_practices.md#数据可用性)。
- 如果 Redis 的用户名或者密码中包含特殊字符，需要使用单引号进行封闭，避免 shell 进行解释。或者使用环境变量 `REDIS_PASSWORD` 进行传递。

:::tip 提示
一个 Redis 实例默认可以创建 16 个逻辑数据库，而一个逻辑数据库可以创建一个 JuiceFS 文件系统。也就是说，在默认情况下，你可以使用一个 Redis 实例创建 16 个 JuiceFS 文件系统。需要注意，用于 JuiceFS 的逻辑数据库不要和其他应用共享，否则可能会造成数据混乱。
:::

例如，创建名为 `pics` 的文件系统，使用 Redis 的 `1` 号数据库存储元数据：

```shell
juicefs format \
    --storage s3 \
    ... \
    "redis://:mypassword@192.168.1.6:6379/1" \
    pics
```

安全起见，建议使用环境变量 `META_PASSWORD` 或 `REDIS_PASSWORD` 传递数据库密码，例如：

```shell
export META_PASSWORD=mypassword
```

然后就无需在元数据 URL 中设置密码了：

```shell
juicefs format \
    --storage s3 \
    ... \
    "redis://192.168.1.6:6379/1" \
    pics
```

#### 挂载文件系统

如果需要在多台服务器上共享同一个文件系统，必须确保每台服务器都能访问到存储元数据的数据库。

```shell
juicefs mount -d "redis://:mypassword@192.168.1.6:6379/1" /mnt/jfs
```

挂载文件系统也支持用 `META_PASSWORD` 或 `REDIS_PASSWORD` 环境变量传递密码：

```shell
export META_PASSWORD=mypassword
juicefs mount -d "redis://192.168.1.6:6379/1" /mnt/jfs
```

#### 设置 TLS

JuiceFS 同时支持 Redis 的 TLS 单向加密认证和 mTLS 双向加密认证连接。通过 TLS 或 mTLS 连接到 Redis 时均使用 `rediss://` 协议头，但是在使用 TLS 单向加密认证时，不需要指定客户端证书和私钥。

:::note 注意
对 Redis mTLS 功能的支持需要使用 1.1.0 及以上版本的 JuiceFS
:::

当通过 mTLS 连接 Redis 时，需要提供客户端证书和私钥，以及签发客户端证书的 CA 证书进行连接。在 JuiceFS 中，可以通过以下方式设置 mTLS 需要的客户端证书：

```shell
juicefs format --storage s3 \
    ... \
    "rediss://192.168.1.6:6379/1?tls-cert-file=/etc/certs/client.crt&tls-key-file=/etc/certs/client.key&tls-ca-cert-file=/etc/certs/ca.crt"
    pics
```

上面的示例代码使用 `rediss://` 协议头来开启 mTLS 功能，然后使用以下选项来指定客户端证书的路径：

- `tls-cert-file=<path>` 指定客户端证书的路径
- `tls-key-file=<path>` 指定客户端密钥的路径
- `tls-ca-cert-file=<path>` 指定签发客户端证书的 CA 证书路径，它是可选的，如果不指定，客户端会使用系统默认的 CA 证书进行验证。
- `insecure-skip-verify=true` 可以用来跳过对服务端证书的验证

在 URL 指定选项时，以 `?` 符号开头，使用 `&` 符号来分隔多个选项，例如：`?tls-cert-file=client.crt&tls-key-file=client.key`。

上例中的 `/etc/certs` 只是一个目录，实际使用时请替换为你的证书目录，可以使用相对路径或绝对路径。

### Valkey

[Valkey](https://valkey.io) 是 Redis 的一个开源分支，旨在保留该项目由社区驱动的治理模式，同时保持与 Redis 生态系统的高度兼容。Valkey 专注于在中立的方式下维护稳定性、提升性能并持续创新，从而为依赖 Redis 兼容工作负载的用户确保长期可用性。

在用于 JuiceFS 的元数据存储引擎时，Valkey 的功能与 Redis 完全相同。请参考 Valkey 的[文档](https://valkey.io/topics/installation)进行安装以及了解其他相关内容，同时参考本文档 [Redis](#redis) 章节了解使用方法。

### KeyDB

[KeyDB](https://keydb.dev) 是 Redis 的一个开源分支，其开发目的是与 Redis 社区保持一致。KeyDB 在 Redis 的基础上实现了多线程支持、更优的内存利用率以及更高的吞吐量，同时还支持 [Active Replication](https://docs.keydb.dev/docs/active-rep)，即 Active-Active（双活）功能。KeyDB 被认为与 Redis 6 版本兼容，但目前[不再被其社区积极维护](https://github.com/Snapchat/KeyDB/issues/895)。

:::note 注意
KeyDB 的 Active Replication 功能是异步复制的，可能会导致一致性问题，请务必充分验证、谨慎使用！
:::

在用于 JuiceFS 的元数据存储引擎时，KeyDB 的功能与 Redis 完全相同。请参考 KeyDB 的[文档](https://docs.keydb.dev/docs)进行安装以及了解其他相关内容，同时参考本文档 [Redis](#redis) 章节了解使用方法。

## 键值数据库

### BadgerDB

[BadgerDB](https://github.com/dgraph-io/badger) 是一个 Go 语言开发的嵌入式、持久化的单机 Key-Value 数据库，它的数据库文件存储在本地你指定的目录中。

使用 BadgerDB 作为 JuiceFS 元数据存储引擎时，使用 `badger://` 协议头指定数据库路径。

#### 创建文件系统

无需提前创建 BadgerDB 数据库，直接创建文件系统即可：

```shell
juicefs format badger://$HOME/badger-data myjfs
```

上述命令在当前用户的 `home` 目录创建 `badger-data` 作为数据库目录，并以此作为 JuiceFS 的元数据存储。

#### 挂载文件系统

挂载文件系统时需要指定数据库路径：

```shell
juicefs mount -d badger://$HOME/badger-data /mnt/jfs
```

对于小规格机器或读多写少负载，可以启用 lean profile，降低 BadgerDB 的固定内存和后台 CPU 开销：

```shell
juicefs mount -d "badger://$HOME/badger-data?profile=lean" /mnt/jfs
```

lean profile 会限制 BadgerDB block/index cache、memtable 和 compaction worker，并且只在累计足够元数据写入后运行 value-log GC。代价是部分元数据吞吐会下降。

:::tip 提示
BadgerDB 只允许单进程访问，如果需要执行 `gc`、`fsck`、`dump`、`load` 等操作，需要先卸载文件系统。
:::
