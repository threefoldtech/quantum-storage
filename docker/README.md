# QSFS docker

## Important paths

The container mounts qsfs on `/mnt` and the 0-stor config is read from `/data/zstor.toml`.

Make sure to mount a valid 0-stor config in the container.

## Building

```bash
docker build . -t qsfs
```

## Running

```bash
docker run -v <zstor.toml-path-file-on-host>:/data/zstor.toml -ti qsfs
```

Here is a sample of a [zstor.toml](./zstor-sample.toml)
