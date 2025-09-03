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

See the deployment section of the [docs](https://github.com/threefoldtech/quantum-storage/tree/master/docs) for more info on using `quantumd`.
