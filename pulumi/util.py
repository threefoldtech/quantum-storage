import os
import subprocess


def run_script_ssh(ip, script):
    counter = 1
    while True:
        log_filename = f"ssh.{counter}.log"
        try:
            with open(log_filename, "x") as logfile:
                with open(script, "r") as scriptfile:
                    script_contents = scriptfile.read()
                    subprocess.run(
                        [
                            "ssh",
                            "-oStrictHostKeyChecking=accept-new",
                            "-oConnectionAttempts=5",
                            "root@" + ip,
                            "bash",
                            " -s",
                        ],
                        input=script_contents,
                        text=True,
                        stdout=logfile,
                        stderr=logfile,
                    )
                break
        except FileExistsError:
            counter += 1


def scp(ip, source, destination):
    # Meant for ipv6
    counter = 1
    while True:
        log_filename = f"scp.{counter}.log"
        try:
            with open(log_filename, "x") as logfile:
                subprocess.run(
                    [
                        "scp",
                        "-r",
                        "-oStrictHostKeyChecking=accept-new",
                        "-oConnectionAttempts=5",
                        source,
                        f"root@[{ip}]:{destination}",
                    ],
                    stdout=logfile,
                    stderr=logfile,
                )
                break
        except FileExistsError:
            counter += 1


def get_ssh_key_from_disk():
    key_paths = [
        os.path.expanduser("~/.ssh/id_rsa.pub"),
        os.path.expanduser("~/.ssh/id_ed25519.pub"),
        os.path.expanduser("~/.ssh/id_ecdsa.pub"),
        os.path.expanduser("~/.ssh/id_dsa.pub"),
    ]

    ssh_key = None

    for path in key_paths:
        try:
            with open(path) as file:
                ssh_key = file.read()
                break
        except (FileNotFoundError, OSError):
            continue

    return ssh_key
