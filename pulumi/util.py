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
