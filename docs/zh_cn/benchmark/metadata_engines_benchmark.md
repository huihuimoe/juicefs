---
title: 元数据引擎性能测试
sidebar_position: 6
slug: /metadata_engines_benchmark
description: 本文介绍如何对 JuiceFS 当前支持的元数据引擎性能进行测试和评估。
---

JuiceFS 当前支持 Redis 兼容数据库和 BadgerDB 作为元数据引擎。评估元数据性能时，应保持客户端、对象存储和工作负载一致，只切换元数据引擎 URL。

## 元数据引擎

### Redis 兼容数据库

需要多客户端共享同一个文件系统时使用 Redis。根据部署要求分别测试相关持久化配置：

- `appendfsync always`：可靠性更强，延迟更高
- `appendfsync everysec`：延迟更低，但 Redis 崩溃时可能丢失最近一秒的写入

### BadgerDB

BadgerDB 适合本地或单进程场景。BadgerDB 会把元数据存储在本地目录中，不支持多个 JuiceFS 进程并发访问。

## 测试工具

### Go benchmark

运行源码内置的元数据基准测试：

```shell
go test ./pkg/meta -bench Benchmark -run '^$' -count=1
```

### JuiceFS bench

在已挂载的文件系统上运行内置端到端测试：

```shell
juicefs bench /mnt/jfs -p 4
```

### mdtest

使用 `mdtest` 测试偏元数据操作的多客户端负载。对比元数据引擎时，应保持对象存储和客户端节点不变。

```shell
mpirun --use-hwthread-cpus --allow-run-as-root -np 12 --hostfile myhost --map-by slot \
  /root/mdtest -b 3 -z 1 -I 100 -u -d /mnt/jfs
```

### fio

工作负载包含大块数据 I/O 时使用 `fio`。大 I/O 场景通常会把瓶颈从元数据引擎转移到对象存储。

```shell
fio --name=big-write --directory=/mnt/jfs --rw=write --refill_buffers \
  --bs=4M --size=4G --numjobs=4 --end_fsync=1 --group_reporting
```
