# QSFS docker

## Important paths

The container mounts qsfs on `/mnt` and the 0-stor config is read from `/data/zstor.toml`.

Make sure to mount a valid 0-stor config in the container.

### 0-stor config

In the 0-stor config, set the following properties to these fixed paths:

```toml
zdbfs_mountpoint = "/mnt"
socket = "/var/run/zstor.sock"
zdb_data_dir_path = "/data/data/zdbfs-data"
```

There's no need to set the `root` property.

## IPv6

When the 0-db's are deployed on the grid, they have IPv6 addresses so the [Docker daemon needs to be configured for this](https://docs.docker.com/config/daemon/ipv6/).

You can also use host networking  by passing `--network host`to make sure the processes in the container can access the zdb's

Note:
> IPv6 networking is only supported on Docker daemons running on Linux hosts.

## Fuse

Make sure the container host has the fuse kernel module loaded ( `modprobe fuse`).
Inside the container the fuse device needs to be available ( pass `--device /dev/fuse`) and the process inside the container needs to be authorised to use it ( use ``--privileged` or `--cap-add SYS_ADMIN`).

## Running

```bash
docker run  --network host --device /dev/fuse --privileged -v <zstor.toml-path-file-on-host>:/data/zstor.toml  ghcr.io/threefoldtech/qsfs
```

Here is a sample of a [zstor.toml](./zstor-sample.toml)

## Extra Feature

You can use a special option with docker to mount-share the container mountpoint:

```bash
mkdir /mnt/qsfs
docker run [...] --mount type=bind,source=/mnt/qsfs,target=/mnt,bind-propagation=rshared ghcr.io/threefoldtech/qsfs
```

Using this feature, you will get the `/mnt/qsfs` on your host, being the same mount as `/mnt` inside
the container.

So anything going to `/mnt/zdbfs` on your host, is sent to qsfs.

## Docker image and workings

By inspecting the Dockerfile we can immediately see that the `qsfs` image simply downloads all the needed components and puts them on the image (0-db-fs, 0-db, and 0-stor). It also downloads the extra `zinit`component that is not explained above.

`zinit` is a light weight process manager that is ideal to use in containers. It allow you to run and maintain multiple services (processes) running inside the same container. This is accomplished by simply making zinit your container entrypoint, and then start other services by means of configuration files. The reason why `zinit` is used is that it can easily configure services dependencies on each other which is important for `qsfs`. `zinit` also provide a `test` command for each service that can be executed to make sure the service is running before it starts other services that depends on it.

So what is the order that the services has to start with:

- (1) zstor: This need to start first to make sure it's there to handle zdb events from the start
- (2) zdb: The local db
- (3) ns: create the needed zdb namespaces
- (4) zdb-fs: The actual file system

> the `3rd` process is a oneshot service (runs only one time when the docker is started) that ensure that the zdb has the needed namespaces for the zdb-fs operations

Okay, this sounds straight forward. right? so there should be at least `4` configuration files they are listed here

- [zstor.yaml](rootfs/etc/zinit/zstor.yaml)
- [zdb.yaml](rootfs/etc/zinit/zdb.yaml)
- [ns.yaml](rootfs/etc/zinit/ns.yaml)
- [fs.yaml](rootfs/etc/zinit/fs.yaml)

In each config file you can see already that it lists what services it has to wait for before they can start using the `after` directive in the config file. Also please not the following

- `zstor` is started in daemon mode `zstor monitor` hence any other zstor command called later will basically queue the operation for the daemon to execute
- `zdb` is stated with `hooks` that points to [hooks.sh](rootfs/bin/hook.sh)
- all services `log` is sent to stdout (of zinit) this means all logs from all the services will aggregate to zinit stdout which will end up in the container logs so you can use `docker logs` to see the logs.

### hooks.sh

We are not going into details about this file, but the hooks file is called by zdb each time according to documentation [here](https://github.com/threefoldtech/0-db#hook-system).

The hooks.sh file then handles the events (hooks) triggered by zdb to call the proper `zstor` commands to offload, or fetch missing data files.

### That's it?

Well, with the above all running and well configured `qsfs` should work smoothly. The only issue will arise when you trying to do `unmount`. You see when you do `unmount` you also need to make sure last bits of the zdb data files that has been written locally are uploaded to remote server.

In other words on container shutdown we need to make sure that not only that services are shutting down in order but also that they actually close (cleanly close) after all binding hooks (to upload) are handled. Otherwise data loss might occur.

Luckily `zinit` also maintains the service order on a shutdown. So the system shutdown the service in the reverse order of how they started. It also wait for a service to shutdown before it stops it's dependencies.

This mean that the system starts by stopping `zdb-fs` followed by `zdb` then `zstor` in that order, in a away that the zdb is only terminated after the fs has gracefully shutdown, and so on.

zinit also allows to configure a shutdown_timeout which can control how long zinit waits on a service shutdown before it kills it and move on.

### wait_hooks

This brings us directly to the `wait_hooks` wait hooks is a pseudo service. It actually does nothing for the startup, but it's in the dependency chain for zdb. because of what is explained above things goes as follows:

#### On Start

- wait hooks script is started, the script is very simple it register a `trap` on script exit (code to be executed when the script is terminated) then go to sleep for infinity.
- zdb starts after wait hooks

#### On Shutdown

- zdb is stopped, this can trigger some hooks for zdb
- wait_hooks script is then terminated. (SIGTERM) the script wakes up from sleep and then tries to run the TERM trap. The close code lists an `hook` process running if there is any, waits for a little longer and try again
- `wait_hooks` keep doing this forever until there are no more hooks running. This means all hooks has been registered with `zstor` and it's up to zstor to make sure all write operations are handled
- if `wait_hooks` takes longer than the `shutdown_timeout` which is set to a very large value, it's forcefully terminated (may be a smaller timeout should be better)
- Once `wait_hooks` is done, zinit moves to the next service which is `zstor` in this case
- `zstor` once it receive a TERM will also wait until all queued operations are handled, preventing the system from fully shutting down until all write operations are handled.

## It does not work

There is a seperate [troubleshooting guide](./troubleshooting/md).
