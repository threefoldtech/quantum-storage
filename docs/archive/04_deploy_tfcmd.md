## Introduction

This section covers the manual deployment steps for QSFS, including both the backend zdbs and an Ubuntu VM to host the frontend. It is also possible to host the frontend on pretty much any other Linux machine as long as FUSE is available. At this point you should already have node IDs selected for your backend zdbs. If not, please see the planning section above for more details.

While there are several ways to deploy zdbs on the ThreeFold Grid, including using Terraform and custom code using one of the SDKs, this guide shows how to to do this with our simple command line tool, tfcmd. This tool is available both for Linux and MacOS. For best security of the seed phrase used to create the deployments, it is recommended to run tfcmd on your local machine. However, it is also possible to use it inside the VM deployed for the QSFS frontend.

## Deployment Prerequisites

Before proceeding, we assume that you have an activated TFChain account funded with TFT and ready to create deployments. If not, see the [getting started](https://manual.grid.tf/documentation/system_administrators/getstarted/tfgrid3_getstarted.html) section of the manual for more details.

## Deploy Frontend VM

Both full and micro VMs work fine for the QSFS frontend.

There are a few considerations to keep in mind for this VM:

- QSFS will consume more CPU and RAM as load increases.
- 1 vCPU and 2 GB of RAM can work for light loads, but at least an additional vCPU will be helpful for heavier loads.
- The SSD capacity of the VM is the maximum available frontend cache size for QSFS.
- You should enable Mycelium networking, as this will be required to connect to the zdbs.

You can deploy this VM using the [Dashboard](https://dashboard.grid.tf/#/deploy/virtual-machines/) or via the various other methods described in the ThreeFold Manual.

If you plan to use the frontend VM to run tfcmd and deploy the zdbs, then connect to the VM via SSH now and run all the following commands on the VM.

## Deploy Backend Zdbs

Now we will deploy the backend zdbs using tfcmd. To assist with creating multiple deployments efficiently, some short scripts will be shown below. These scripts have been tested in bash and might not work in other shells. At the same time, we will also create our zstor config file, which must contain information about the backends.

### Prerequisites

We will use `wget`, `jq`, and `openssl`. Make sure that they are properly installed on the system you will be using. On Ubuntu, for example, you can use this command:

```sh
apt update && apt install -y wget jq openssl
```

### Set Your EDITOR

In the sections below, we'll be creating a number of config files. Set your preferred editor now to facilitate copying and pasting the commands. For example:

```sh
EDITOR=nano
```

### Install Tfcmd

Here we will fetch the `tfcmd` binary and install it locally. It is released as part of `tfgrid-sdk-go`. You can find the latest releases [here](https://github.com/threefoldtech/tfgrid-sdk-go/releases). Choose the correct download link for your platform.

For example, to install version 15.10 on a x64 Linux machine, you would run the following:

```
wget -O tfgrid-sdk-go.tar.gz https://github.com/threefoldtech/tfgrid-sdk-go/releases/download/v0.15.10/tfgrid-sdk-go_Linux_x86_64.tar.gz
tar -C /usr/local/bin -xf tfgrid-sdk-go.tar.gz tfcmd
chmod +x /usr/local/bin/tfcmd
rm tfgrid-sdk-go.tar.gz
```

To test that it worked, try logging in. You'll need to complete this step before creating any deployments:

```sh
tfcmd login
```


### Create Stub Zstor Config

Next we will create a stub of the zstor config file. This will contain all of the information that needs to be filled manually. The final sections with the backend info will be filled automatically using scripts.

Open a file `zstor-default.toml` in a text editor and paste in the template below. For example:

```sh
$EDITOR zstor-default.toml
```

Initial contents of the file:

```
minimal_shards = 16
expected_shards = 20
redundant_groups = 0
redundant_nodes = 0
zdbfs_mountpoint = "/mnt/qsfs"
socket = "/var/run/zstor.sock"
prometheus_port = 9100
zdb_data_dir_path = "/data/data/zdbfs-data"
max_zdb_data_dir_size = 25600

[encryption]
algorithm = "AES"
key = "Write your key here"

[compression]
algorithm = "snappy"

[meta]
type = "zdb"

[meta.config]
prefix = "qsfs-meta"

[meta.config.encryption]
algorithm = "AES"
key = "write your key here"
```

Make sure to edit the file as needed. You can change the minimal and expected shards according to your own plan. Another important value is `max_zdb_data_dir_size`, which is how large the cache is allowed to grow before data blocks are removed. This value is given in MiB. Therefore, the example shown is 25GiB.

The `zdbfs_mountpoint` can also be adjusted to wherever you want to mount the filesystem, and that could be any location of your choosing. This configuration value does not actually control the placement of the mount, however, it is just used by zstor for monitoring purposes. The actual mounting will happen later. Likewise `zdb_data_dir_path` should be updated if you want to place the zdb data directory somewhere else.

It is also necessary to fill in the encryption keys with your own. You can use the same or different keys for data and metadata, at your own preference. The key must be 32 bytes in hex format. Here's an example of how to generate a key in this format:

```sh
openssl rand -hex 32
```

Once this is done, take the output and insert it into the file in the indicated locations.

### Prepare for Zdb Deployment

Now we will deploy the zdbs and write their details into the config file in the proper format. Before proceeding, we'll set a few shell variables that will be used in the deployment scripts:

```sh
CONFIG=zstor-default.toml
PASSWORD=$(openssl rand -base64 18)
METADATA_NODES="1 2 3 4"
BACKEND_NODES="1 2 3 4 5 6 7 8"
BACKEND_SIZE=1
```

This will generate a strong random password that will be used to secure each zdb. You can replace the code that generates the password with your own password if you wish. For now, don't worry about having to save the randomly generated password. It will get written to the config file and you can take note of it later.


For the metadata and backend nodes, replace the example values with the node IDs you selected before. Set your desired backend size too, which is specified in gigabtyes. For more details on selection of backend nodes, and how many of each type to specify, see the previous section on planning a deployment.


### Deploy Metadata Zdbs

Here is an example script to deploy the metadata zdbs based on the variables set above. This uses a fixed size of 1 GB, which is the minimum when deploying via tfcmd and should be plenty for storing all of the metadata ever needed by a typical QSFS deployment. Metadata zdbs run in `user` mode.

```sh
for node in $METADATA_NODES; do
  name=node${node}meta
  tfcmd deploy zdb --mode user --node $node -n $name --size 1 --password $PASSWORD
done
```

It's possible that some of your chosen nodes don't respond properly at deployment time and need to be replaced with other nodes. In that case, just replace the variable with the new node IDs like this:

```sh
# Example: Node 3 wasn't working, replace it with node 5
METADATA_NODES="1 2 4 5"
```

Then you can just run the original deployment script loop again. Any zdb deployments that would be duplicated will be detected by `tfcmd` and skipped over.

### Cancel Metadata Zdbs

If you need to cancel the metadata zdb deployments, use this script:

```sh
for node in $METADATA_NODES; do
  name=node${node}meta
  tfcmd cancel $name
done
```

### Write Metadata Zdbs Config

Next we will write the configuration data from the deployed zdbs into the config file. The script as shown is for IPv6 connections. If you are using Yggdrasil, replace `.Zdbs[0].ips[0]` with `.Zdbs[0].ips[1]`

```sh
# Wait about five seconds before doing the next step to make sure data is available. Only needed if running immediately after the deployment step in a single script
sleep 5

# Write out the config sections for the metadata zdbs
for node in $METADATA_NODES; do
  name=node${node}meta
  echo Fetching and writing config for $name
  json=$(tfcmd get zdb $name 2>&1 | tail -n +3 | sed $'s/\e\[0m//')
  ip=$(echo $json | jq .Zdbs[0].ips[-1] | tr -d \")
  port=$(echo $json | jq .Zdbs[0].port)
  namespace=$(echo $json | jq .Zdbs[0].namespace)
  password=$(echo $json | jq .Zdbs[0].password)

  echo \# Node $node >> $CONFIG
  echo [[meta.config.backends]] >> $CONFIG
  echo address = \"\[$ip\]:$port\" >> $CONFIG
  echo namespace = $namespace >> $CONFIG
  echo password = $password >> $CONFIG
  echo >> $CONFIG
done
```

Once that has completed, you can check inside your `zstor-default.toml` file to see the result. There should be four sections that look like this:

```
# Node 1
[[meta.config.backends]]
address = "[2a02:1802:5e:0:c11:7dff:fe8e:83bb]:9900"
namespace = "18-532404-node1meta0"
password = "Your password"
```

Sometimes `tfcmd` fails to fetch deployment information from one or more nodes. In that case, you might see one block with blank fields. You can try generating those sections again by changing the script to only target those node IDs.

Here's an example to retry nodes 2 and 3:

```sh
# Example with nodes 2 and 3. Write out the config sections for the metadata zdbs
for node in 2 3; do
  # The rest of the script is the same
  # ...
```

Then check the file again. Make sure that all the metadata nodes you specified have a populated config section and that result of any failed attempts are deleted.

### Deploy Data Zdbs

This process is very similar to the deployment of the metadata backends, with a few small changes to the scripts. We use the variable `BACKEND_SIZE` we defined above and `seq` mode this time.

To deploy:

```sh
for node in $BACKEND_NODES; do
  name=node${node}backend
  tfcmd deploy zdb --mode seq --node $node -n $name --size $BACKEND_SIZE --password $PASSWORD
done
```

As before, you might need to replace some node IDs and try again:

```sh
# Example: Node 3 wasn't working, replace it with node 9
BACKEND_NODES="1 2 4 5 6 7 8 9"
```

Then run the deployment loop again.

### Cancel Metadata Zdbs

Likewise, if you need to cancel the data zdb deployments, use this script:

```sh
for node in $BACKEND_NODES; do
  name=node${node}backend
  tfcmd cancel $name
done
```

### Write Data Zdbs Config

To write the config for the data zdbs, use the following script:

```sh
# Ditto, need to wait
sleep 5

echo [[groups]] >> $CONFIG
for node in $BACKEND_NODES; do
  name=node${node}backend
  echo Fetching and writing config for $name
  json=$(tfcmd get zdb $name 2>&1 | tail -n +3 | sed $'s/\e\[0m//')
  ip=$(echo $json | jq .Zdbs[0].ips[-1] | tr -d \")
  port=$(echo $json | jq .Zdbs[0].port)
  namespace=$(echo $json | jq .Zdbs[0].namespace)
  password=$(echo $json | jq .Zdbs[0].password)

  echo \# Node $node >> $CONFIG
  echo [[groups.backends]] >> $CONFIG
  echo address = \"\[$ip\]:$port\" >> $CONFIG
  echo namespace = $namespace >> $CONFIG
  echo password = $password >> $CONFIG
  echo >> $CONFIG
done
```

Notice this time that the data backends have an extra line `[[groups]]` separating them from the top of the file. This script just creates a single group. If you want to use more groups, add more groups lines to separate the backends in each group.

As before, check the output for any failures to retrieve data. You can retry them in the same way:

```sh
# Note that we skipped the line with [[groups]]
for node in $BACKEND_NODES; do
  # The rest of the script is the same
  # ...
```

Once every data backend has a valid entry in the config file, we are done with this section of the deployment.

### Storing Zstor Config

The `zstor-default.toml` file contains sensitive information that is sufficient to recover and decrypt all of the data stored in your QSFS. Needless to say, you should keep the contents of this file secure.

If your frontend machine is lost for any reason, the zstor config file can be used to recover the data. On the other hand, if the contents of this file are lost, the data in the backends can never be recovered.

*Consider storing the entire contents of your `zstor-default.toml` file in a durable and secure data store like a password manager.*

## Frontend System Prep

At this point, we are ready to proceed the setup of the frontend system. We'll demonstrate all necessary commands to do this in any Linux system that already has `wget` installed, FUSE enabled in the kernel, and a Mycelium connectivity. If you deployed a VM on the ThreeFold Grid according to the instructions above, then these requirements are already met.

### Install Binaries

The three binary executables needed to operate QSFS are provided in statically compiled form with no dependencies. You can download them from GitHub according to the links on each project's release page:

- 0-db-fs: [https://github.com/threefoldtech/0-db-fs/releases](https://github.com/threefoldtech/0-db-fs/releases)
- 0-db: [https://github.com/threefoldtech/0-db/releases](https://github.com/threefoldtech/0-db/releases)
- 0-stor: [https://github.com/threefoldtech/0-stor_v2/releases](https://github.com/threefoldtech/0-stor_v2/releases)


Here is an example with the latest versions of each component at the time of publishing this guide. We will also download a hook script that is the final needed component:

```
wget -O /usr/local/bin/zdbfs https://github.com/threefoldtech/0-db-fs/releases/download/v0.1.11/zdbfs-0.1.11-amd64-linux-static
wget -O /usr/local/bin/zdb https://github.com/threefoldtech/0-db/releases/download/v2.0.8/zdb-2.0.8-linux-amd64-static
wget -O /bin/zstor https://github.com/threefoldtech/0-stor_v2/releases/download/v0.4.0/zstor_v2-x86_64-linux-musl
wget -O /usr/local/bin/zdb-hook.sh https://raw.githubusercontent.com/threefoldtech/quantum-storage/master/lib/zdb-hook.sh

# Make them all executable
chmod +x /usr/local/bin/* /bin/zstor
```

One note here is that the name and location of the `zstor` executable must match what is shown here for the hook script to work properly.

### Directories

Two directories will be needed for QSFS operation. First is the QSFS mount point and second is the data directory for the local zdb.

The locations shown below are examples and you can change them if you wish. However if you choose to use different locations for either the mount point or the data folder, don't forget to update them accordingly in the zstor config file and also substitute them throughout the guide below.

Make sure both directories exist like this:

```sh
mkdir -p /mnt/qsfs
mkdir -p /data
```

### Set EDITOR

Again, make sure your preferred editor is set if you want easy copy and paste:

```sh
EDITOR=nano
```

### Zstor Config

Now copy your `zstor-default.toml` to the `/etc` folder inside the VM. You could use `scp` for this, or copy and paste the contents into a new file on the frontend machine:

```sh
$EDITOR /etc/zstor-default.toml
```

## Frontend Services Setup

Now we will demonstrate how to configure each of the three QSFS components as a system service managed by a process manager. There are two different options here. First is systemd, which comes standard in most Linux distributions including ThreeFold full VMs. The second will be zinit, which is the process manager provided in our official micro VM images where systemd is not in use. Example config files and commands will be shown for each case.

### Note on Systemd Service Restarts

The default behavior of systemd is to restart services fast if they exit unexpectedly but also to quickly give up on restarting a service that continues to exit.

To ensure that systemd always tries to restart our services, no matter how many failures occur, `StartLimitIntervalSec` is set to zero in the examples below to disable the restart limit. However, in the event of a crash loop this can put a fair amount of strain on the CPU.

While crash loops are by no means expected, you may want to increase the time that systemd waits before trying to restart the services. Therefore, the default value of `RestartSec` is also written in case you want to change it. The tradeoff is that in the event of a recoverable failure, a longer timeout means that the service is offline for more time.

### Zstor

The example service config files for zstor are shown below. There is another value you might want to tune in these files, which is the timeout for stopping the service. For systemd, this is called `TimeoutStopSec` and for zinit it is called `shutdown_timeout`.

In both cases, these values determine how long the process manager will wait between asking the process to exit on its own and issuing a kill signal in case the process does not respond.

For zstor, this timeout represents the max time available to write data to the backends when the service is stopped, such as during a graceful system shutdown in the case of systemd. For the examples a fairly generous five minutes is used, but a longer timeout might be safer.

#### Zstor Systemd

```sh
$EDITOR /etc/systemd/system/zstor.service
```

```
[Unit]
Wants=network.target
After=network.target
StartLimitIntervalSec=0

[Service]
ProtectHome=true
ProtectSystem=true
ReadWritePaths=/data /var/log
ExecStart=/bin/zstor \
  --log_file /var/log/zstor.log \
  -c /etc/zstor-default.toml \
  monitor
Restart=always
RestartSec=100ms
TimeoutStopSec=5m

[Install]
WantedBy=multi-user.target
```

#### Zstor Zinit

```sh
$EDITOR /etc/zinit/zstor.yaml
```

```yaml
exec: /bin/zstor \
  --log_file /var/log/zstor.log \
  -c /etc/zstor-default.toml \
  monitor
shutdown_timeout: 300
```

### Zdb

Next is zdb. There are two arguments here that might be of interest for tuning. The first is `--datasize`, which is the maximum size of data blocks, in bytes. Here use 64MiB.

The other argument to consider is `--rotate`, which is the time at which incomplete data blocks are closed and backed up. This value is in seconds, so the example is 15 minutes. Reducing this time can help reduce the chance of data loss if the frontend is lost, but it will also result in more data fragmentation which can impact performance

In this case, we use a shorter `TimeoutStopSec`, to give some time for zdb to flush remaining data to disk, but with the assumption that this happens much more quickly than zstor's network based operations.

#### Zdb Systemd

```sh
$EDITOR /etc/systemd/system/zdb.service
```

```
[Unit]
Wants=network.target zstor.service
After=network.target zstor.service

[Service]
ProtectHome=true
ProtectSystem=true
ReadWritePaths=/data /var/log
ExecStart=/usr/local/bin/zdb \
    --index /data/index \
    --data /data/data \
    --logfile /var/log/zdb.log \
    --datasize 67108864 \
    --hook /usr/local/bin/zdb-hook.sh \
    --rotate 900
Restart=always
RestartSec=5
TimeoutStopSec=60

[Install]
WantedBy=multi-user.target
```

#### Zdb Zinit

```sh
$EDITOR /etc/zinit/zdb.yaml
```

```yaml
exec: /usr/local/bin/zdb \
    --index /data/index \
    --data /data/data \
    --logfile /var/log/zdb.log \
    --datasize 67108864 \
    --hook /usr/local/bin/zdb-hook.sh \
    --rotate 900
shutdown_timeout: 60
after: zstor
```

### Zdbfs

And finally, zdbfs. The only option to configure here is the mount point. For this example we use `/mnt/qsfs`. Remember that if you are using a different mount point, it's best to also update it in the zstor config file.

In the case of zdbfs, the stop timeouts are less relevant. When zdbfs receives `TERM` it will exit regardless of any ongoing writes, and those write operations will encounter an error. There is a final flush to the zdb, but this happens very quickly when the zdb is running on the same machine so five seconds should be more than enough.

Note that upon subsequent starts when using the `autons` option, zdbfs will give some non fatal errors about not being able to create zdb namespaces. These are really just informational messages that can be ignored.

#### Zdbfs Systemd

```
$EDITOR /etc/systemd/system/zdbfs.service
```

```
[Unit]
Wants=network.target zdb.service
After=network.target zdb.service

[Service]
ExecStart=/usr/local/bin/zdbfs /mnt/qsfs -o autons
Restart=always
RestartSec=5
TimeoutStopSec=5

[Install]
WantedBy=multi-user.target
```

#### Zdbfs Zinit

```sh
$EDITOR /etc/zinit/zdbfs.yaml
```

In the case of zdbfs, the shutdown timeout is less relevant. When zdbfs receives `TERM` it will exit regardless of any ongoing writes, and those writes operations will encounter an error. There is a final flush to the zdb, but this happens very quickly when the zdb is running on the same machine so five seconds should be more than enough.

```yaml
exec: /usr/local/bin/zdbfs /mnt/qsfs -o autons
after: zdb
```

### Start the Services

Next we will start up all the services and then check that everything is working properly.

#### Systemd

```sh
systemctl daemon-reload
systemctl enable --now zstor zdb zdbfs
```

#### Zinit

```sh
zinit monitor zdbfs
zinit monitor zdb
zinit monitor zstor
```

### Check Operation

Check that zstor is working well:

```sh
zstor --log_file ~/zstor.log -c /etc/zstor-default.toml status
```

It should show info on each backend and ideally they should all be reachable.

You can also check that zdbfs is mounted:

```sh
df
```

There should be an entry with type `zdbfs` mounted at your specified mount point. Now any files you write to the mount point will be encoded and uploaded to the backends, either when a zdb data block fills or when the rotate time has passed.

## Conclusion

Deployment of QSFS is now complete. In the next section, we'll cover concerns regarding the ongoing operation of a QSFS system, including how to recover from backend failures.
