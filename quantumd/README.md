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

First get yourself a binary:

```
# Go compiler is required
make build
```

Then copy the binary to the machine where QSFS will run. Note the following:

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
