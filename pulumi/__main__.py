import os
import shutil
import textwrap
from pathlib import Path

import pulumi
import pulumi_random
import pulumi_command
import pulumi_threefold as threefold

# It's up to the user to create their own vars.py before trying to deploy
try:
    from vars import (
        MNEMONIC,
        NETWORK,
        SSH_KEY_PATH,
        VM_NODE,
        META_NODES,
        DATA_NODES,
        DATA_SIZE,
        ZDB_CONNECTION,
        SSH_CONNECTION,
    )
except ModuleNotFoundError:
    exit("vars.py not found. Exiting.")

# Same for the base zstor config. Exit if the user didn't provide this
ZSTOR_CONFIG_BASE = "zstor_config.base.toml"
ZSTOR_CONFIG_PATH = "zstor_config.toml"
# This path is hard coded in the Zdb hook script
ZSTOR_CONFIG_REMOTE = "/etc/zstor-default.toml"

if not os.path.exists(ZSTOR_CONFIG_BASE):
    exit("zstor_config.base.toml not found. Exiting.")


# Path of the script that will run on the deployed VM after deployment
# Installs needed binaries and starts up all the services
POST_DEPLOY_SCRIPT = "post_deploy.sh"

# If a node has IPv6, then it will be the first IP in the zdb IP list
# Mycelium will always be last, but this could be index 1 or 2
ZDB_IP6_INDEX = 0
ZDB_MYC_INDEX = -1

# From here are all the parameters for the deployment
MNEMONIC = MNEMONIC if MNEMONIC else os.environ.get("MNEMONIC")
NETWORK = NETWORK if NETWORK else os.environ.get("NETWORK")

ssh_key_path = os.path.expanduser(SSH_KEY_PATH)

with open(ssh_key_path, "r") as file:
    ssh_private_key = file.read()

with open(ssh_key_path + ".pub", "r") as file:
    ssh_public_key = file.read()

FLIST = "https://hub.grid.tf/tf-official-apps/threefoldtech-ubuntu-22.04.flist"
CPU = 1
RAM = 2048  # MB
ROOTFS = 1024 * 15  # MB
NET_NAME = "net"

META_SIZE = 1

# Generate separate secrets for Zstor key and Zdb namespaces passwords
zstor_key = pulumi_random.RandomBytes("zstor_key", length=32)
zdb_pw = pulumi_random.RandomPassword("zdb_pw", length=20)

if ZDB_CONNECTION == "ipv6":
    ZDB_IP_INDEX = ZDB_IP6_INDEX
elif ZDB_CONNECTION == "mycelium":
    ZDB_IP_INDEX = ZDB_MYC_INDEX

# CopyToRemote requires that the path used contains some file from the start, so
# we just put an empty one there if needed
Path(ZSTOR_CONFIG_PATH).touch()

provider = threefold.Provider(
    "provider",
    mnemonic=MNEMONIC,
    network=NETWORK,
)

# Deploying a VM is optional. Some users might want to use an existing VM or
# another system for their QFS frontend
if VM_NODE is not None:
    network = threefold.Network(
        "network",
        name=NET_NAME,
        description="A network",
        nodes=[VM_NODE],
        ip_range="10.1.0.0/16",
        # With mycelium enabled, we can't redeploy the vm
        # https://github.com/threefoldtech/pulumi-threefold/issues/552
        # Maybe it's okay though if we use separate deployements for vm and zdbs?
        # mycelium=True,
        opts=pulumi.ResourceOptions(provider=provider),
    )

    vm_deployment = threefold.Deployment(
        "vm_deployment",
        node_id=VM_NODE,
        name="vm",
        network_name=NET_NAME,
        vms=[
            threefold.VMInputArgs(
                name="vm",
                node_id=VM_NODE,
                flist=FLIST,
                entrypoint="/sbin/zinit init",
                network_name=NET_NAME,
                cpu=CPU,
                memory=RAM,
                rootfs_size=ROOTFS,
                # mycelium=True,
                planetary=True,
                public_ip6=True,
                env_vars={
                    "SSH_KEY": ssh_public_key,
                },
            )
        ],
        opts=pulumi.ResourceOptions(provider=provider, depends_on=[network]),
    )
else:
    vm_deployment = None

zdb_nodes = set(META_NODES + DATA_NODES)
zdb_deployments = []

for node in zdb_nodes:
    zdbs = []
    if node in DATA_NODES:
        zdbs.append(
            threefold.ZDBInputArgs(
                name="data" + str(node),
                size=DATA_SIZE,
                mode="seq",
                password=zdb_pw.result,
            )
        )
    if node in META_NODES:
        zdbs.append(
            threefold.ZDBInputArgs(
                name="meta" + str(node),
                size=META_SIZE,
                mode="user",
                password=zdb_pw.result,
            )
        )

    zdb_deployments.append(
        threefold.Deployment(
            "zdb_deployment" + str(node),
            node_id=node,
            name="node" + str(node),
            zdbs=zdbs,
            opts=pulumi.ResourceOptions(provider=provider),
        )
    )


def make_ssh_connection(vm):
    if SSH_CONNECTION == "mycelium":
        ssh_ip = vm["mycelium_ip"]
    elif SSH_CONNECTION == "ipv6":
        ssh_ip = vm["computed_ip6"].apply(lambda ip6: ip6.split("/")[0])

    return pulumi_command.remote.ConnectionArgs(
        host=ssh_ip,
        user="root",
        private_key=ssh_private_key,
    )


def make_zstor_config(args):
    # Changes to the zdb backends mean that we need to regenerate the config
    # file. Here we always choose a new local path and leave the old files
    # around just in case
    i = 1
    while os.path.exists(path := ZSTOR_CONFIG_PATH + "." + str(i)):
        i += 1

    shutil.copy(ZSTOR_CONFIG_BASE, path)

    vm = args["vm"]
    meta_zdbs = []
    data_zdbs = []
    for zdbs in args["zdbs"]:
        for zdb in zdbs:
            if "meta" in zdb["namespace"]:
                meta_zdbs.append(zdb)
            else:
                data_zdbs.append(zdb)
    meta_zdbs = sorted(meta_zdbs, key=lambda z: z["namespace"].split("-")[-1])
    data_zdbs = sorted(data_zdbs, key=lambda z: z["namespace"].split("-")[-1])

    with open(path, "a") as file:
        encryption_config = f"""
        [encryption]
        algorithm = "AES"
        key = "{args['zstor_key']}"

        [meta.config.encryption]
        algorithm = "AES"
        key = "{args['zstor_key']}"
        """
        file.write(textwrap.dedent(encryption_config))
        for zdb in meta_zdbs:
            ip = zdb["ips"][ZDB_IP_INDEX]
            ns = zdb["namespace"]
            file.write("[[meta.config.backends]]\n")
            file.write(f'address = "[{ip}]:9900"\n')
            file.write(f'namespace = "{ns}"\n')
            file.write(f'password = "{args['zdb_pw']}"\n\n')

        file.write("[[groups]]\n")
        for zdb in data_zdbs:
            ip = zdb["ips"][ZDB_IP_INDEX]
            ns = zdb["namespace"]
            file.write("[[groups.backends]]\n")
            file.write(f'address = "[{ip}]:9900"\n')
            file.write(f'namespace = "{ns}"\n')
            file.write(f'password = "{args['zdb_pw']}"\n\n')

    # This way the current file is always in the same place and we get around
    # the fact that it's not possible to return a path from this function and
    # use it as a FileAsset, because you can't pass an Output to FileAsset
    if not open(path).read() == open(ZSTOR_CONFIG_PATH).read():
        shutil.copy(path, ZSTOR_CONFIG_PATH)
    else:
        # We end up regenerating the same file from time to time for reasons
        # that may or may not be unavoidable. For now, just delete duplicates
        os.remove(path)


zstor_config_output = pulumi.Output.all(
    vm=vm_deployment.vms_computed[0],
    zdbs=[d.zdbs_computed for d in zdb_deployments],
    zstor_key=zstor_key.hex,
    zdb_pw=zdb_pw.result,
).apply(make_zstor_config)

if vm_deployment:
    vm = vm_deployment.vms_computed[0]
    conn = make_ssh_connection(vm)
    depends = []

    copy_zstor_config = pulumi_command.remote.CopyToRemote(
        "copy_zstor_config",
        connection=conn,
        source=pulumi.FileAsset(ZSTOR_CONFIG_PATH),
        remote_path=ZSTOR_CONFIG_REMOTE,
        # Without this trigger, a new upload isn't triggered when the VM is
        # replaced. However, the file on an existing VM gets updated just with
        # zstor_config_output in the depends_on list
        triggers=[conn.host],
        opts=pulumi.ResourceOptions(depends_on=[zstor_config_output, vm_deployment]),
    )

    if os.path.isfile("prometheus.yaml"):
        depends.append(
            pulumi_command.remote.CopyToRemote(
                "copy_prometheus",
                connection=conn,
                source=pulumi.FileAsset("prometheus.yaml"),
                remote_path="/etc/prometheus.yaml",
                triggers=[conn.host],
            )
        )

    # In case we want to test our own zstor binary, such as a prebuild
    if os.path.isfile("zstor"):
        depends.append(
            pulumi_command.remote.CopyToRemote(
                "copy_zstor_binary",
                connection=conn,
                source=pulumi.FileAsset("zstor"),
                remote_path="/usr/bin/zstor",
                triggers=[conn.host],
            )
        )

    # We put the zinit files under /root to start, so that the services don't get
    # started accidentally on reboot. In the case of recovering on a new VM, we
    # need to ensure some other steps are completed first
    copy_zinit = pulumi_command.remote.CopyToRemote(
        "copy_zinit",
        connection=conn,
        source=pulumi.FileArchive("zinit/"),
        remote_path="/root/zinit/",
        triggers=[conn.host],
    )

    copy_scripts = pulumi_command.remote.CopyToRemote(
        "copy_scripts",
        connection=conn,
        source=pulumi.FileArchive("scripts/"),
        remote_path="/root/scripts/",
        triggers=[conn.host],
    )

    depends.append(copy_scripts)

    prep_vm = pulumi_command.remote.Command(
        "prep_vm",
        connection=conn,
        create="bash /root/scripts/prep_vm.sh 2>&1 | tee > /var/log/prep_vm.log",
        triggers=[conn.host],
        opts=pulumi.ResourceOptions(depends_on=depends),
    )

    depends.extend([prep_vm, copy_zinit, copy_zstor_config])
    pulumi_command.remote.Command(
        "activate_qsfs",
        connection=conn,
        create="bash /root/scripts/activate_qsfs.sh 2>&1 | tee > /var/log/activate_qsfs.log",
        update="",
        opts=pulumi.ResourceOptions(depends_on=depends),
    )

pulumi.export("mycelium_ip", vm.mycelium_ip)
pulumi.export("pub_ipv6", vm.computed_ip6)
