# QSFS docker

## Building

```bash
docker build . -t qsfs
```

## Running

```bash
docker run -v <zstor.toml-path-file-on-host>:/data/zstor.toml -ti qsfs
```

Here is a sample of a [./zstor-sample.toml](zstor.toml)
