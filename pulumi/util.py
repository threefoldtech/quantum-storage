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


def wait_for_host(host, max_retries=None):
    """Ping a host until it responds or max retries is reached. Since the default timeout on Linux systems is typically 10 seconds, the total timeout will be that times the number of retries."""
    import subprocess

    retry_count = 0

    while True:
        if max_retries and retry_count >= max_retries:
            return False

        try:
            # Ping the host once
            command = ["ping", "-c", "1", host]
            subprocess.check_output(command)
            return True

        except subprocess.CalledProcessError:
            retry_count += 1
            continue
