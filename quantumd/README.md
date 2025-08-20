# Quantumd

The idea here is to create a program that can manage all aspects of QSFS that don't fit into the roles of the main components that make up the system. Planned aspects are:

1. Deployment of backend zdbs (both initial deployment and replacement of failed backends during operation)
2. Installation of all needed binaries and creation of system services to keep them alive (supporting both systemd and zinit)
3. Handling hook events from zdb (rather than previously used shell script)
4. Data integrity checks and retries for failed uploads
5. A single central and simple config file for end users (config files and cli args for QSFS components are generated from this automatically)
6. Automated recovery of the QSFS into a new machine, in case of frontend machine failure
7. Overall simplified operation (single binary and single config file brings up the whole system)

## Usage

So far a subset of functionality has been implemented and heavy changes are expected. It's already possible to deploy a QSFS using the tool though, both with local and remote backends.

First get yourself a binary. You can grab a release artifact from this repo:

```bash
wget https://github.com/threefoldtech/quantum-storage/releases/download/v0.4.0-rc13/quantumd_0.4.0-rc13_linux_amd64 -O /usr/local/bin/quantumd
chmod +x /usr/local/bin/quantumd
```

Note the following:

- Linux only for now
- Installs files to `/usr/local/bin` and `/etc` without confirmation
- Recommended to use a VM or container
- Mycelium is required for remote backends

### Local backends

For testing, a local backend option is available. This starts up a number of "backend" zdbs on the local system. No config is required, just run:

```
quantumd deploy --local
```

### Remote backends

To make a real deployment with remote backends, some config information must be supplied. This includes a TFChain account that is funded and able to make deployments.

Copy the `config.example.yaml` file and fill in your own info. The mnemonic and network options can also be specified as environment variables, MNEMONIC and NETWORK.

Then deploy the backend zdbs:

```
quantumd deploy --config config.yaml
```

This will automatically write a zstor config file under `/etc`.

Next we can start up QSFS with this command:

```
quantumd setup
```

That's it. Once the command finishes, your QSFS should be ready.

### Init command

There's also an `init` command that does the deploy and setup operations in one step:

```
quantumd init
```

### Restore

If you need to move your QSFS deployment to a new frontend machine, you can do so using the `restore` command. This requires that the config file uses a matching deployment name and password:

```
quantumd restore
```

This one command will fully bring up the QSFS on the new machine. Note that operating multiple QSFS instances using the same config file or backends is not supported and may lead to data loss. Be sure that the old machine is permanently offline before restoring.

### Daemon Mode

The daemon mode runs continuously to handle hook events from zdb and periodically retry failed uploads. When using the built-in setup feature, a system service running the daemon mode will be added automatically. There's normally no need to run the command manually:

```
quantumd daemon
```

This will:
- Listen for hook events from zdb (close, ready, namespace updates, etc.)
- Periodically scan for failed uploads and retry them
- Track uploaded files in SQLite database as a local cache to make retry handling efficient
