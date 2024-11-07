import os
import secrets
import shutil
import textwrap
import pulumi
import pulumi_random
import pulumi_threefold as threefold

import util

# Check if vars.py exists
if not os.path.exists('vars.py'):
    try:
        print("vars.py not found. Running create_vars.py...")
        import create_vars
        # After running create_vars.py, try to import vars again
        import vars
    except Exception as e:
        exit(f"Failed to create vars.py: {str(e)}")
else:
    try:
        import vars
    except ModuleNotFoundError:
        exit("Error importing vars.py even though file exists. Check file contents.")

# Same for the base zstor config. Exit if the user didn't provide this
ZSTOR_CONFIG_BASE = "zstor_config.base.toml"
ZSTOR_CONFIG = "zstor_config.toml"
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
MNEMONIC = vars.MNEMONIC
NETWORK = vars.NETWORK

with open(os.path.expanduser("~/.ssh/id_rsa.pub")) as file:
    SSH_KEY = file.read()

VM_NODE = vars.VM_NODE
FLIST = "https://hub.grid.tf/tf-official-apps/threefoldtech-ubuntu-22.04.flist"
CPU = 1
RAM = 2048  # MB
ROOTFS = 1024 * 15  # MB
NET_NAME = "net"

META_NODES = vars.META_NODES
DATA_NODES = vars.DATA_NODES
DATA_SIZE = vars.DATA_SIZE
META_SIZE = 1

# Generate separate secrets for Zstor key and Zdb namespaces passwords
ZSTOR_KEY = secrets.token_hex(32)
ZDB_PW = secrets.token_urlsafe(32)
zstor_key = pulumi_random.RandomBytes("zstor_key", length=32)
zdb_pw = pulumi_random.RandomPassword("zdb_pw", length=20)

if vars.ZDB_CONNECTION == "ipv6":
    ZDB_IP_INDEX = ZDB_IP6_INDEX
elif vars.ZDB_CONNECTION == "mycelium":
    ZDB_IP_INDEX = ZDB_MYC_INDEX

provider = threefold.Provider(
    "provider",
    mnemonic=MNEMONIC,
    network=NETWORK,
)

network = threefold.Network(
    "network",
    name=NET_NAME,
    description="A network",
    nodes=[VM_NODE],
    ip_range="10.1.0.0/16",
    mycelium=True,
    opts=pulumi.ResourceOptions(provider=provider),
)

nodes = set([VM_NODE] + META_NODES + DATA_NODES)

deployments = {}

for node in nodes:
    net_name = ""
    vms = []
    depends = []
    if node == VM_NODE:
        net_name = NET_NAME
        depends.append(network)
        vms.append(
            threefold.VMInputArgs(
                name="vm",
                flist=FLIST,
                entrypoint="/sbin/zinit init",
                network_name=net_name,
                cpu=CPU,
                memory=RAM,
                rootfs_size=ROOTFS,
                mycelium=True,
                planetary=True,
                public_ip6=True,
                env_vars={
                    "SSH_KEY": SSH_KEY,
                },
            )
        )
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

    deployments[node] = threefold.Deployment(
        "deployment" + str(node),
        node_id=node,
        name="node" + str(node),
        network_name=net_name,
        vms=vms,
        zdbs=zdbs,
        opts=pulumi.ResourceOptions(provider=provider, depends_on=depends),
    )


def post_deploy(args):
    # TODO: Don't overwrite existing file if it's there
    # Actually, maybe it's okay as long as we have the secrets persisted
    shutil.copy(ZSTOR_CONFIG_BASE, ZSTOR_CONFIG)

    meta_zdbs = []
    data_zdbs = []
    for vm_list, zdb_list in args["deployments"]:
        if vm_list:
            vm = vm_list[0]

        for zdb in zdb_list:
            if "meta" in zdb["namespace"]:
                meta_zdbs.append(zdb)
            else:
                data_zdbs.append(zdb)
    meta_zdbs = sorted(meta_zdbs, key=lambda z: z["namespace"].split("-")[-1])
    data_zdbs = sorted(data_zdbs, key=lambda z: z["namespace"].split("-")[-1])

    with open(ZSTOR_CONFIG, "a") as file:
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

    # ssh_ip = vm["mycelium_ip"]
    ssh_ip = vm["computed_ip6"].split("/")[0]
    util.scp(ssh_ip, "zinit/", "/etc/")
    util.scp(ssh_ip, ZSTOR_CONFIG, ZSTOR_CONFIG_REMOTE)
    util.run_script_ssh(ssh_ip, POST_DEPLOY_SCRIPT)


pulumi.Output.all(
    deployments=[(d.vms_computed, d.zdbs_computed) for d in deployments.values()],
    zstor_key=zstor_key.hex,
    zdb_pw=zdb_pw.result,
).apply(post_deploy)

vm = deployments[VM_NODE].vms_computed[0]
pulumi.export("mycelium_ip", vm.mycelium_ip)
pulumi.export("pub_ipv6", vm.computed_ip6)
