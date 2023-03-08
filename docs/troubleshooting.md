# Troubleshooting

## zstor status

the `zstor -c <zstor_config.toml> status` command gives an overview of the backend 0-db's, if they are reachable or not and the storage space they consume.

## Error: ZstorError

```log
Error: ZstorError { kind: Storage, internal: Zdb(ZdbError { kind: Connect, remote: ZdbConnectionInfo { address: [300:cc55:8958:c1ff:229e:63d2:6fda:878d]:9900, namespace: Some("741-18336-meta1"), password: Some("supersecretpass") }, internal: Redis(Address not available (os error 99)) }) }
```

The ip address of the 0-db is not reachable.

## ERROR error during storage

```log
ERROR error during storage: ZDB at [300:cc55:8958:c1ff:229e:63d2:6fda:878d]:9900 741-18336-meta1, error operation CONNECT caused by timeout
```

Check that the zdb ip adress is reachable by performing a normal `ping` command.
If it is, try a [redis-cli](https://redis.io/docs/ui/cli/) `PING` to check if the zdb is alive.

## fuse device not found

```log
fusermount3: fuse device not found, try 'modprobe fuse' first
```

Make sure fuse3 is installed and the kernel module is loaded.

## failed to process store command error during accessing local storage for attempting to store file which is not in the file tree rooted at

```log
ERROR failed to process store command error during accessing local storage for attempting to store file which is not in the file tree rooted at /mnt: invalid data, queueing a retry
```

The zstor config has an invalid `root` property set. Set it to where the local 0-db stores its data or drop it completely.

## could not find any viable backend distribution to statisfy redundancy requirement

```log
2023-03-03 13:25:56 +00:00: DEBUG Finding backend config
2023-03-03 13:25:56 +00:00: ERROR failed to process store command error during configuration: could not find any viable backend distribution to statisfy redundancy requirement, queueing a retry
2023-03-03 13:25:56 +00:00: DEBUG the scheduler forwarding command Store(Store { file: "/data/index/zdbfs-meta/zdb-namespace", key_path: None, save_failure: true, delete: false, blocking: false })
2023-03-03 13:25:56 +00:00: DEBUG encoding file "/data/index/zdbfs-meta/zdb-namespace" with key path "/data/index/zdbfs-meta/zdb-namespace"
2023-03-03 13:25:56 +00:00: DEBUG file checksum: 4ab1d2c1bb4be398cd9726251442fc78 ("/data/index/zdbfs-meta/zdb-namespace")
2023-03-03 13:25:56 +00:00: DEBUG Loading metadata for key /testdocker/meta/f46911200c6be4cc49cf7c1df2a37b6a
2023-03-03 13:25:56 +00:00: DEBUG Reading data from zdb metastore for key /testdocker/meta/f46911200c6be4cc49cf7c1df2a37b6a
2023-03-03 13:25:56 +00:00: DEBUG Metadata for file "/data/index/zdbfs-meta/zdb-namespace" not found.
```

The zstor redundancy configuration (probably redundant groups or redundant nodes) is not configured correctly ( maybe it's set too high).
