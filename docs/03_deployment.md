As of 2025 there is a new way to deploy and manage QSFS instances, using the `quantumd` utility. As the name implies, `quantumd` is an additional daemon that runs alongside the other components. It is also a tool for handling the entire lifecycle of QSFS deployments, including backend provisioning on the ThreeFold Grid and preparing machines as frontend hosts.

The features of `quantumd` as a daemon include:

1. Retry functionality to recover from any failures to store data in the backends
2. Additional monitoring around system status and health, revealing the progress of any repair operations for example
3. Automatic replacement of failed backends (work in progress)

Given that these features are not present in other deployment methods not using `quantumd`, such as the primitive provided by Zos, it's not recommended to use other deployment methods aside from what's covered here.

## Install quantumd

Before using `quantumd`, a frontend machine must be provisioned. Linux machines only are supported for now, and a VM deployed on the ThreeFold Grid is the best option. You can find information about how to do that in the [ThreeFold Manual](https://manual.grid.tf/users/intro). Both full and micro VMs are supported by `quantumd`. Mycelium connectivity is recommended for backend connections.

After connecting to the frontend machine by SSH, you can install `quantumd` with this command:

```bash
wget https://github.com/threefoldtech/quantum-storage/releases/latest/download/quantumd_linux_amd64 -O /usr/local/bin/quantumd
chmod +x /usr/local/bin/quantumd
```
That will download the latest precompiled release binary from this repository and make it executable. Check for success with:

```bash
quantumd --version
```

The version number should be printed to the console.

> Please note that `quantumd` will make various changes to the system, including installing additional binaries and creating system services, without additional user confirmation. It's designed to provision a bare VM into a QSFS frontend, although coexistence with other software and services should generally not be a problem.

## Using quantumd

All use of `quantumd` requires a config file. There is an example in this repository under `quantumd/config.example.yaml`. Copy the contents and paste them into a new file on your frontend machine. For example, using the default config path:

```bash
nano /etc/quantumd.yaml
```

Many config options are not required and those lines are commented out. The mnemonic and network options can also be specified as environment variables, MNEMONIC and NETWORK. Your mnemonic will be used both for generating an encryption key and also for creating the backend deployments.

> Both the mnemonic seed phrase and password used to deploy will be required to recover the data later. I recommend making a copy of your entire config file and storing it somewhere safe like a password manager.

### Deploy

With the config file written, a QSFS instance can be brought up with a single command:

```
quantumd init
```

This will perform the following steps:

1. Download all required binaries for QSFS components
2. Deploy backend zdbs
3. Create system services for all components and start them

When the process is finished, you should see your QSFS mountpoint:

```bash
df -h
```

### Restore

In case there's a need to move to a new frontend VM for any reason, `quantumd` provides a convenient restore method. This performs many of the same steps as `init`, but it looks for existing data on existing backends.

First, repeat the steps to install the `quantumd` binary and restore the same config file as originally used. Then run the restore command:

```bash
quantumd restore
```

Once the process is complete, all files that were successfully stored in the backends should be available once more under your QSFS mount.
