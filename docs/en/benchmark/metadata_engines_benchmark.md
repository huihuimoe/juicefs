---
title: Metadata Engines Benchmark
sidebar_position: 6
slug: /metadata_engines_benchmark
description: This article describes how to test and evaluate the performance of supported metadata engines for JuiceFS.
---

JuiceFS currently supports Redis-compatible databases and BadgerDB as metadata engines. Benchmark metadata performance with the same client, object storage, and workload, changing only the metadata engine URL.

## Metadata engines

### Redis-compatible databases

Use Redis when the file system needs to be shared by multiple clients. Test both durability configurations if they are relevant to your deployment:

- `appendfsync always`: stronger durability, higher latency
- `appendfsync everysec`: lower latency, possible loss of the latest second of writes if Redis crashes

### BadgerDB

Use BadgerDB for local or single-process scenarios. BadgerDB stores metadata in a local directory and does not support concurrent access from multiple JuiceFS processes.

## Tools

### Go benchmark

Run the metadata benchmarks included in the source tree:

```shell
go test ./pkg/meta -bench Benchmark -run '^$' -count=1
```

### JuiceFS bench

Run the built-in end-to-end benchmark on a mounted file system:

```shell
juicefs bench /mnt/jfs -p 4
```

### mdtest

Use `mdtest` for metadata-heavy multi-client workloads. Keep the object storage and client hosts unchanged when comparing metadata engines.

```shell
mpirun --use-hwthread-cpus --allow-run-as-root -np 12 --hostfile myhost --map-by slot \
  /root/mdtest -b 3 -z 1 -I 100 -u -d /mnt/jfs
```

### fio

Use `fio` when the workload includes large data I/O. Large I/O usually shifts the bottleneck from metadata to object storage.

```shell
fio --name=big-write --directory=/mnt/jfs --rw=write --refill_buffers \
  --bs=4M --size=4G --numjobs=4 --end_fsync=1 --group_reporting
```
